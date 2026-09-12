package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/autonomous-bits/spool/internal/contextual"
	"github.com/autonomous-bits/spool/internal/resolve"
)

func toolResolve(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_resolve",
		Description: "Resolve an immutable node entity by stable ID from a branch snapshot or explicit commit.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch to resolve the node from",
				},
				"node": map[string]any{
					"type":        "string",
					"description": "Stable node entity ID",
				},
				"commit": map[string]any{
					"type":        "string",
					"description": "Optional reachable commit ID to resolve against",
				},
			},
			"required": []string{"branch", "node"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch string  `json:"branch"`
				Node   string  `json:"node"`
				Commit *string `json:"commit,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Branch == "" {
				return nil, errors.New("branch is required")
			}
			if in.Node == "" {
				return nil, errors.New("node is required")
			}
			return withTool(stateDirProvider, func(tool *resolve.ResolveTool) (any, error) {
				return tool.SPLResolve(ctx, resolve.ResolveRequest{
					Selector: resolve.SnapshotSelector{
						Branch: in.Branch,
						Commit: in.Commit,
					},
					NodeID: in.Node,
				})
			})
		},
	}
}

func toolSearch(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_search",
		Description: "Perform lexical full-text search (FTS5) across nodes in a branch head snapshot.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch to search within",
				},
				"query": map[string]any{
					"type":        "string",
					"description": "Search query terms or keywords",
				},
				"continuation": map[string]any{
					"type":        "string",
					"description": "Optional pagination continuation token",
				},
			},
			"required": []string{"branch", "query"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch       string `json:"branch"`
				Query        string `json:"query"`
				Continuation string `json:"continuation,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Branch == "" {
				return nil, errors.New("branch is required")
			}
			if in.Query == "" {
				return nil, errors.New("query is required")
			}
			return withTool(stateDirProvider, func(tool *resolve.ResolveTool) (any, error) {
				return tool.SPLSearch(ctx, resolve.SearchRequest{
					Selector:          resolve.SnapshotSelector{Branch: in.Branch},
					Query:             in.Query,
					ContinuationToken: in.Continuation,
				})
			})
		},
	}
}

func toolSearchExpand(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_search_expand",
		Description: "Select lexical or typed-filter seed evidence, then expand bounded graph context from those seeds.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch to query",
				},
				"query": map[string]any{
					"type":        "string",
					"description": "Lexical search query for seed matching (mutually exclusive with label)",
				},
				"label": map[string]any{
					"type":        "string",
					"description": "Label filter for seed matching (mutually exclusive with query)",
				},
				"direction": map[string]any{
					"type":        "string",
					"description": "Edge traversal direction: 'out', 'in', or 'both' (default 'out')",
					"enum":        []string{"out", "in", "both"},
				},
				"edge_types": map[string]any{
					"type":        "array",
					"description": "Optional edge types to traverse",
					"items": map[string]any{
						"type": "string",
					},
				},
				"seed_limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of seed nodes to expand from",
				},
			},
			"required": []string{"branch"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch    string   `json:"branch"`
				Query     string   `json:"query,omitempty"`
				Label     string   `json:"label,omitempty"`
				Direction string   `json:"direction,omitempty"`
				EdgeTypes []string `json:"edge_types,omitempty"`
				SeedLimit int      `json:"seed_limit,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Branch == "" {
				return nil, errors.New("branch is required")
			}
			if in.Query != "" && in.Label != "" {
				return nil, errors.New("query and label are mutually exclusive")
			}
			if in.Query == "" && in.Label == "" {
				return nil, errors.New("either query or label must be specified")
			}

			dir := contextual.DirectionOut
			switch in.Direction {
			case "", "out":
				dir = contextual.DirectionOut
			case "in":
				dir = contextual.DirectionIn
			case "both":
				dir = contextual.DirectionBoth
			default:
				return nil, fmt.Errorf("invalid direction %q: must be 'out', 'in', or 'both'", in.Direction)
			}

			seeds := contextual.SeedSelector{}
			if in.Query != "" {
				seeds.Query = in.Query
			} else {
				seeds.Labels = []string{in.Label}
			}

			return withTool(stateDirProvider, func(tool *resolve.ResolveTool) (any, error) {
				return tool.SPLSearchExpand(ctx, resolve.SearchExpandRequest{
					Selector:  resolve.SnapshotSelector{Branch: in.Branch},
					Seeds:     seeds,
					Direction: dir,
					EdgeTypes: in.EdgeTypes,
					SeedLimit: in.SeedLimit,
				})
			})
		},
	}
}

func toolFilter(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_filter",
		Description: "Filter nodes in a branch head snapshot by label.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch to filter",
				},
				"labels": map[string]any{
					"type":        "array",
					"description": "Node labels to match (e.g. ['Requirement', 'Decision'])",
					"items": map[string]any{
						"type": "string",
					},
				},
				"continuation": map[string]any{
					"type":        "string",
					"description": "Optional pagination continuation token",
				},
			},
			"required": []string{"branch"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch       string   `json:"branch"`
				Labels       []string `json:"labels,omitempty"`
				Continuation string   `json:"continuation,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Branch == "" {
				return nil, errors.New("branch is required")
			}
			return withTool(stateDirProvider, func(tool *resolve.ResolveTool) (any, error) {
				return tool.SPLFilter(ctx, resolve.FilterRequest{
					Selector:          resolve.SnapshotSelector{Branch: in.Branch},
					Labels:            in.Labels,
					ContinuationToken: in.Continuation,
				})
			})
		},
	}
}

func toolContext(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_context",
		Description: "Assemble evidence-focused bounded graph context (nodes, incident edges, supporting paths) from either a lexical query OR label filter.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch to query",
				},
				"query": map[string]any{
					"type":        "string",
					"description": "Lexical search query for seed matching (mutually exclusive with label)",
				},
				"label": map[string]any{
					"type":        "string",
					"description": "Label filter for seed matching (mutually exclusive with query)",
				},
				"direction": map[string]any{
					"type":        "string",
					"description": "Edge traversal direction: 'out', 'in', or 'both' (default 'out')",
					"enum":        []string{"out", "in", "both"},
				},
				"edge_types": map[string]any{
					"type":        "array",
					"description": "Optional edge types to traverse",
					"items": map[string]any{
						"type": "string",
					},
				},
				"seed_limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of seed nodes to expand from",
				},
			},
			"required": []string{"branch"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch    string   `json:"branch"`
				Query     string   `json:"query,omitempty"`
				Label     string   `json:"label,omitempty"`
				Direction string   `json:"direction,omitempty"`
				EdgeTypes []string `json:"edge_types,omitempty"`
				SeedLimit int      `json:"seed_limit,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Branch == "" {
				return nil, errors.New("branch is required")
			}
			if in.Query != "" && in.Label != "" {
				return nil, errors.New("query and label are mutually exclusive")
			}
			if in.Query == "" && in.Label == "" {
				return nil, errors.New("either query or label must be specified")
			}

			dir := contextual.DirectionOut
			switch in.Direction {
			case "", "out":
				dir = contextual.DirectionOut
			case "in":
				dir = contextual.DirectionIn
			case "both":
				dir = contextual.DirectionBoth
			default:
				return nil, fmt.Errorf("invalid direction %q: must be 'out', 'in', or 'both'", in.Direction)
			}

			seeds := contextual.SeedSelector{}
			if in.Query != "" {
				seeds.Query = in.Query
			} else {
				seeds.Labels = []string{in.Label}
			}

			return withTool(stateDirProvider, func(tool *resolve.ResolveTool) (any, error) {
				return tool.SPLContext(ctx, resolve.ContextRequest{
					Selector:  resolve.SnapshotSelector{Branch: in.Branch},
					Seeds:     seeds,
					Direction: dir,
					EdgeTypes: in.EdgeTypes,
					SeedLimit: in.SeedLimit,
				})
			})
		},
	}
}

func toolDiff(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_diff",
		Description: "Compute structural graph diff (added, removed, modified nodes and edges) between base and target branches or explicit commits.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"base_branch": map[string]any{
					"type":        "string",
					"description": "Base branch name",
				},
				"base_commit": map[string]any{
					"type":        "string",
					"description": "Optional explicit reachable base commit",
				},
				"target_branch": map[string]any{
					"type":        "string",
					"description": "Target branch name",
				},
				"target_commit": map[string]any{
					"type":        "string",
					"description": "Optional explicit reachable target commit",
				},
			},
			"required": []string{"base_branch", "target_branch"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				BaseBranch   string  `json:"base_branch"`
				BaseCommit   *string `json:"base_commit,omitempty"`
				TargetBranch string  `json:"target_branch"`
				TargetCommit *string `json:"target_commit,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.BaseBranch == "" || in.TargetBranch == "" {
				return nil, errors.New("base_branch and target_branch are required")
			}
			return withTool(stateDirProvider, func(tool *resolve.ResolveTool) (any, error) {
				return tool.SPLDiff(ctx, resolve.DiffRequest{
					Base: resolve.SnapshotSelector{
						Branch: in.BaseBranch,
						Commit: in.BaseCommit,
					},
					Target: resolve.SnapshotSelector{
						Branch: in.TargetBranch,
						Commit: in.TargetCommit,
					},
				})
			})
		},
	}
}

func toolHistory(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_history",
		Description: "Retrieve commit history and changes for a specific node or entity ID within a branch.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch to query",
				},
				"node": map[string]any{
					"type":        "string",
					"description": "Entity or node ID",
				},
			},
			"required": []string{"branch", "node"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch string `json:"branch"`
				Node   string `json:"node"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Branch == "" || in.Node == "" {
				return nil, errors.New("branch and node are required")
			}
			return withTool(stateDirProvider, func(tool *resolve.ResolveTool) (any, error) {
				return tool.SPLHistory(ctx, resolve.HistoryRequest{
					Selector: resolve.SnapshotSelector{Branch: in.Branch},
					EntityID: in.Node,
				})
			})
		},
	}
}

func toolGraph(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_graph",
		Description: "Export every node and edge in an immutable branch snapshot as JSON.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch to export",
				},
				"commit": map[string]any{
					"type":        "string",
					"description": "Optional reachable commit ID",
				},
			},
			"required": []string{"branch"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch string  `json:"branch"`
				Commit *string `json:"commit,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Branch == "" {
				return nil, errors.New("branch is required")
			}
			return withTool(stateDirProvider, func(tool *resolve.ResolveTool) (any, error) {
				return tool.SPLGraph(ctx, resolve.SnapshotSelector{
					Branch: in.Branch,
					Commit: in.Commit,
				})
			})
		},
	}
}
