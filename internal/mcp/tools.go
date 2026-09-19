package mcp

import (
	"context"
	"encoding/json"

	"github.com/autonomous-bits/spool/internal/surface"
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

// KeepToolNames is the MCP tool surface after the git-SoT command cut.
var KeepToolNames = surface.KeepMCPTools

// RegisterAllTools registers KEEP-only Spool tools onto the official MCP server.
func RegisterAllTools(s *mcp.Server, rt *runtime) {
	if rt == nil {
		rt = newRuntime(ServerOptions{})
	}
	tools := []Tool{
		toolResolve(rt),
		toolSearch(rt),
		toolSearchExpand(rt),
		toolFilter(rt),
		toolQueryContext(rt),
		toolGraph(rt),
		toolContextExport(rt),
		toolMergePreview(rt),
		toolMergeApply(rt),
		toolMergeConflicts(rt),
		toolMergeResolve(rt),
		toolMergeAbort(rt),
		toolMergeFinalize(rt),
		toolSchemaMigrate(rt),
		toolValidate(rt),
		toolPrune(rt),
		toolAssetAdd(rt),
		toolAssetRead(rt),
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
