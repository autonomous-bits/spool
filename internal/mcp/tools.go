package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolHandler handles execution of a Spool tool.
type ToolHandler func(ctx context.Context, args json.RawMessage) (any, error)

// Tool represents a Spool tool specification and its execution handler.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Handler     ToolHandler    `json:"-"`
}

var repoMu sync.Mutex

type budgetInput struct {
	MaxRows          *int `json:"max_rows,omitempty"`
	MaxResponseBytes *int `json:"max_response_bytes,omitempty"`
	MaxDepth         *int `json:"max_depth,omitempty"`
	MaxVisited       *int `json:"max_visited,omitempty"`
	TimeoutMs        *int `json:"timeout_ms,omitempty"`
}

func (b *budgetInput) toBudget() resolve.QueryBudgetRequest {
	if b == nil {
		return resolve.QueryBudgetRequest{}
	}
	req := resolve.QueryBudgetRequest{
		MaxRows:          b.MaxRows,
		MaxResponseBytes: b.MaxResponseBytes,
		MaxDepth:         b.MaxDepth,
		MaxVisited:       b.MaxVisited,
	}
	if b.TimeoutMs != nil {
		d := time.Duration(*b.TimeoutMs) * time.Millisecond
		req.Timeout = &d
	}
	return req
}

var budgetSchema = map[string]any{
	"type":        "object",
	"description": "Optional query budget constraints",
	"properties": map[string]any{
		"max_rows": map[string]any{
			"type":        "integer",
			"description": "Maximum rows to return",
		},
		"max_response_bytes": map[string]any{
			"type":        "integer",
			"description": "Maximum response size in bytes",
		},
		"max_depth": map[string]any{
			"type":        "integer",
			"description": "Maximum traversal depth",
		},
		"max_visited": map[string]any{
			"type":        "integer",
			"description": "Maximum visited nodes",
		},
		"timeout_ms": map[string]any{
			"type":        "integer",
			"description": "Maximum query duration in milliseconds",
		},
	},
}

func withRepo[T any](stateDirProvider func() (string, error), fn func(repo *repository.Repository) (T, error)) (res T, err error) {
	repoMu.Lock()
	defer repoMu.Unlock()

	stateDir, err := stateDirProvider()
	if err != nil {
		return res, fmt.Errorf("resolve repository state directory: %w", err)
	}
	repo, err := repository.OpenRepository(stateDir)
	if err != nil {
		return res, fmt.Errorf("open repository: %w", err)
	}
	defer func() {
		if closeErr := repo.Close(); closeErr != nil {
			if err == nil {
				err = fmt.Errorf("close repository: %w", closeErr)
			} else {
				err = errors.Join(err, fmt.Errorf("close repository: %w", closeErr))
			}
		}
	}()
	return fn(repo)
}

func withTool[T any](stateDirProvider func() (string, error), fn func(tool *resolve.ResolveTool) (T, error)) (res T, err error) {
	repoMu.Lock()
	defer repoMu.Unlock()

	stateDir, err := stateDirProvider()
	if err != nil {
		return res, fmt.Errorf("resolve repository state directory: %w", err)
	}
	repo, err := repository.OpenRepository(stateDir)
	if err != nil {
		return res, fmt.Errorf("open repository: %w", err)
	}
	defer func() {
		if closeErr := repo.Close(); closeErr != nil {
			if err == nil {
				err = fmt.Errorf("close repository: %w", closeErr)
			} else {
				err = errors.Join(err, fmt.Errorf("close repository: %w", closeErr))
			}
		}
	}()
	return fn(resolve.NewResolveTool(repo))
}

func wrapHandler(fn ToolHandler) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := json.RawMessage("{}")
		if req.Params != nil && len(req.Params.Arguments) > 0 {
			args = req.Params.Arguments
		}
		res, err := fn(ctx, args)
		if err != nil {
			if res != nil {
				data, marshalErr := json.Marshal(res)
				if marshalErr == nil {
					var mapped map[string]any
					if json.Unmarshal(data, &mapped) == nil {
						mapped["warning"] = err.Error()
						if enhancedData, err := json.Marshal(mapped); err == nil {
							data = enhancedData
						}
					}
					return &mcp.CallToolResult{
						Content:           []mcp.Content{&mcp.TextContent{Text: string(data)}},
						StructuredContent: json.RawMessage(data),
						IsError:           true,
					}, nil
				}
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
				IsError: true,
			}, nil
		}
		data, err := json.Marshal(res)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
				IsError: true,
			}, nil
		}
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: string(data)}},
			StructuredContent: json.RawMessage(data),
		}, nil
	}
}

// RegisterAllTools registers all 42 Spool tools onto the given official MCP server.
func RegisterAllTools(s *mcp.Server, stateDirProvider func() (string, error)) {
	tools := []Tool{
		toolStatus(stateDirProvider),
		toolAdd(stateDirProvider),
		toolCommit(stateDirProvider),
		toolResolve(stateDirProvider),
		toolSearch(stateDirProvider),
		toolSearchExpand(stateDirProvider),
		toolFilter(stateDirProvider),
		toolContext(stateDirProvider),
		toolDiff(stateDirProvider),
		toolHistory(stateDirProvider),
		toolBranchesContaining(stateDirProvider),
		toolGraph(stateDirProvider),
		toolBranchList(stateDirProvider),
		toolBranchCreate(stateDirProvider),
		toolBranchDelete(stateDirProvider),
		toolSwitch(stateDirProvider),
		toolMergePreview(stateDirProvider),
		toolMergeApply(stateDirProvider),
		toolMergeConflicts(stateDirProvider),
		toolMergeResolve(stateDirProvider),
		toolMergeAbort(stateDirProvider),
		toolMergeFinalize(stateDirProvider),
		toolCherryPick(stateDirProvider),
		toolSchemaMigrate(stateDirProvider),
		toolValidate(stateDirProvider),
		toolInit(stateDirProvider),
		toolPrune(stateDirProvider),
		toolFsck(stateDirProvider),
		toolGC(stateDirProvider),
		toolMigrate(stateDirProvider),
		toolAssetAdd(stateDirProvider),
		toolAssetRead(stateDirProvider),
		toolWorkspaceInit(),
		toolWorkspaceAttach(),
		toolRemoteSet(stateDirProvider),
		toolRemoteShow(stateDirProvider),
		toolRemoteRemove(stateDirProvider),
		toolRemoteBranch(stateDirProvider),
		toolPush(stateDirProvider),
		toolPull(stateDirProvider),
		toolClone(),
		toolVersion(),
	}

	for _, t := range tools {
		s.AddTool(&mcp.Tool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}, wrapHandler(t.Handler))
	}
}
