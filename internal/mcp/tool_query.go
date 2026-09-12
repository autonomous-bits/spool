package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/autonomous-bits/spool/internal/contextual"
	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
)

var predicatesSchema = map[string]any{
	"type":        "array",
	"description": "Optional indexed property predicates",
	"items": map[string]any{
		"type": "object",
		"properties": map[string]any{
			"key":          map[string]any{"type": "string", "description": "Indexed property name"},
			"textEquals":   map[string]any{"type": "string", "description": "String property equality"},
			"numberEquals": map[string]any{"type": "number", "description": "Numeric property equality"},
			"numberMin":    map[string]any{"type": "number", "description": "Inclusive numeric lower bound"},
			"numberMax":    map[string]any{"type": "number", "description": "Inclusive numeric upper bound"},
		},
		"required": []string{"key"},
	},
}

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
				"budget": budgetSchema,
			},
			"required": []string{"branch", "node"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch string       `json:"branch"`
				Node   string       `json:"node"`
				Commit *string      `json:"commit,omitempty"`
				Budget *budgetInput `json:"budget,omitempty"`
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
					Budget: in.Budget.toBudget(),
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
				"commit": map[string]any{
					"type":        "string",
					"description": "Optional reachable commit ID to search against",
				},
				"budget": budgetSchema,
			},
			"required": []string{"branch", "query"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch       string       `json:"branch"`
				Query        string       `json:"query"`
				Commit       *string      `json:"commit,omitempty"`
				Continuation string       `json:"continuation,omitempty"`
				Budget       *budgetInput `json:"budget,omitempty"`
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
					Selector: resolve.SnapshotSelector{
						Branch: in.Branch,
						Commit: in.Commit,
					},
					Query:             in.Query,
					ContinuationToken: in.Continuation,
					Budget:            in.Budget.toBudget(),
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
				"commit": map[string]any{
					"type":        "string",
					"description": "Optional reachable commit ID to query against",
				},
				"labels": map[string]any{
					"type":        "array",
					"description": "Optional node labels for seed matching (mutually exclusive with query)",
					"items": map[string]any{
						"type": "string",
					},
				},
				"predicates": predicatesSchema,
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
				"budget": budgetSchema,
			},
			"required": []string{"branch"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch     string                         `json:"branch"`
				Commit     *string                        `json:"commit,omitempty"`
				Query      string                         `json:"query,omitempty"`
				Label      string                         `json:"label,omitempty"`
				Labels     []string                       `json:"labels,omitempty"`
				Predicates []repository.MetadataPredicate `json:"predicates,omitempty"`
				Direction  string                         `json:"direction,omitempty"`
				EdgeTypes  []string                       `json:"edge_types,omitempty"`
				SeedLimit  int                            `json:"seed_limit,omitempty"`
				Budget     *budgetInput                   `json:"budget,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Branch == "" {
				return nil, errors.New("branch is required")
			}
			if in.Query != "" && (in.Label != "" || len(in.Labels) > 0 || len(in.Predicates) > 0) {
				return nil, errors.New("query is mutually exclusive with label, labels, and predicates")
			}
			if in.Query == "" && in.Label == "" && len(in.Labels) == 0 && len(in.Predicates) == 0 {
				return nil, errors.New("either query, label, labels, or predicates must be specified")
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
				labels := in.Labels
				if in.Label != "" && len(labels) == 0 {
					labels = []string{in.Label}
				}
				seeds.Labels = labels
				seeds.Predicates = in.Predicates
			}

			return withTool(stateDirProvider, func(tool *resolve.ResolveTool) (any, error) {
				return tool.SPLSearchExpand(ctx, resolve.SearchExpandRequest{
					Selector: resolve.SnapshotSelector{
						Branch: in.Branch,
						Commit: in.Commit,
					},
					Seeds:     seeds,
					Direction: dir,
					EdgeTypes: in.EdgeTypes,
					SeedLimit: in.SeedLimit,
					Budget:    in.Budget.toBudget(),
				})
			})
		},
	}
}

func toolFilter(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_filter",
		Description: "Filter nodes in a branch head snapshot by label and indexed property predicates.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch to filter",
				},
				"commit": map[string]any{
					"type":        "string",
					"description": "Optional reachable commit ID to filter against",
				},
				"labels": map[string]any{
					"type":        "array",
					"description": "Node labels to match (e.g. ['Requirement', 'Decision'])",
					"items": map[string]any{
						"type": "string",
					},
				},
				"predicates": predicatesSchema,
				"continuation": map[string]any{
					"type":        "string",
					"description": "Optional pagination continuation token",
				},
				"budget": budgetSchema,
			},
			"required": []string{"branch"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch       string                         `json:"branch"`
				Commit       *string                        `json:"commit,omitempty"`
				Labels       []string                       `json:"labels,omitempty"`
				Predicates   []repository.MetadataPredicate `json:"predicates,omitempty"`
				Continuation string                         `json:"continuation,omitempty"`
				Budget       *budgetInput                   `json:"budget,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Branch == "" {
				return nil, errors.New("branch is required")
			}
			return withTool(stateDirProvider, func(tool *resolve.ResolveTool) (any, error) {
				return tool.SPLFilter(ctx, resolve.FilterRequest{
					Selector: resolve.SnapshotSelector{
						Branch: in.Branch,
						Commit: in.Commit,
					},
					Labels:            in.Labels,
					Predicates:        in.Predicates,
					ContinuationToken: in.Continuation,
					Budget:            in.Budget.toBudget(),
				})
			})
		},
	}
}

func toolContext(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_context",
		Description: "Assemble evidence-focused bounded graph context (nodes, incident edges, supporting paths) from either a lexical query OR label/predicate filter.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch to query",
				},
				"commit": map[string]any{
					"type":        "string",
					"description": "Optional reachable commit ID to query against",
				},
				"query": map[string]any{
					"type":        "string",
					"description": "Lexical search query for seed matching (mutually exclusive with label/predicates)",
				},
				"label": map[string]any{
					"type":        "string",
					"description": "Label filter for seed matching (mutually exclusive with query)",
				},
				"labels": map[string]any{
					"type":        "array",
					"description": "Optional node labels for seed matching (mutually exclusive with query)",
					"items": map[string]any{
						"type": "string",
					},
				},
				"predicates": predicatesSchema,
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
				"budget": budgetSchema,
			},
			"required": []string{"branch"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch     string                         `json:"branch"`
				Commit     *string                        `json:"commit,omitempty"`
				Query      string                         `json:"query,omitempty"`
				Label      string                         `json:"label,omitempty"`
				Labels     []string                       `json:"labels,omitempty"`
				Predicates []repository.MetadataPredicate `json:"predicates,omitempty"`
				Direction  string                         `json:"direction,omitempty"`
				EdgeTypes  []string                       `json:"edge_types,omitempty"`
				SeedLimit  int                            `json:"seed_limit,omitempty"`
				Budget     *budgetInput                   `json:"budget,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Branch == "" {
				return nil, errors.New("branch is required")
			}
			if in.Query != "" && (in.Label != "" || len(in.Labels) > 0 || len(in.Predicates) > 0) {
				return nil, errors.New("query is mutually exclusive with label, labels, and predicates")
			}
			if in.Query == "" && in.Label == "" && len(in.Labels) == 0 && len(in.Predicates) == 0 {
				return nil, errors.New("either query, label, labels, or predicates must be specified")
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
				labels := in.Labels
				if in.Label != "" && len(labels) == 0 {
					labels = []string{in.Label}
				}
				seeds.Labels = labels
				seeds.Predicates = in.Predicates
			}

			return withTool(stateDirProvider, func(tool *resolve.ResolveTool) (any, error) {
				return tool.SPLContext(ctx, resolve.ContextRequest{
					Selector: resolve.SnapshotSelector{
						Branch: in.Branch,
						Commit: in.Commit,
					},
					Seeds:     seeds,
					Direction: dir,
					EdgeTypes: in.EdgeTypes,
					SeedLimit: in.SeedLimit,
					Budget:    in.Budget.toBudget(),
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
				"node_ids": map[string]any{
					"type":        "array",
					"description": "Optional filter for specific node IDs",
					"items": map[string]any{
						"type": "string",
					},
				},
				"edge_ids": map[string]any{
					"type":        "array",
					"description": "Optional filter for specific edge IDs",
					"items": map[string]any{
						"type": "string",
					},
				},
				"node_title_contains": map[string]any{
					"type":        "string",
					"description": "Optional node title substring filter",
				},
				"one_hop": map[string]any{
					"type":        "boolean",
					"description": "Include one-hop context around diff matches",
				},
				"continuation": map[string]any{
					"type":        "string",
					"description": "Optional pagination continuation token",
				},
				"budget": budgetSchema,
			},
			"required": []string{"base_branch", "target_branch"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				BaseBranch        string       `json:"base_branch"`
				BaseCommit        *string      `json:"base_commit,omitempty"`
				TargetBranch      string       `json:"target_branch"`
				TargetCommit      *string      `json:"target_commit,omitempty"`
				NodeIDs           []string     `json:"node_ids,omitempty"`
				EdgeIDs           []string     `json:"edge_ids,omitempty"`
				NodeTitleContains string       `json:"node_title_contains,omitempty"`
				OneHop            bool         `json:"one_hop,omitempty"`
				Continuation      string       `json:"continuation,omitempty"`
				Budget            *budgetInput `json:"budget,omitempty"`
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
					Filter: repository.DiffFilter{
						NodeIDs:         in.NodeIDs,
						EdgeIDs:         in.EdgeIDs,
						NodeTitleSubstr: in.NodeTitleContains,
					},
					IncludeOneHop:     in.OneHop,
					ContinuationToken: in.Continuation,
					Budget:            in.Budget.toBudget(),
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
				"entity_id": map[string]any{
					"type":        "string",
					"description": "Alias for node: stable entity ID",
				},
				"commit": map[string]any{
					"type":        "string",
					"description": "Optional starting commit ID for history traversal",
				},
				"all_parents": map[string]any{
					"type":        "boolean",
					"description": "Traverse all merge parents rather than first-parent only",
				},
				"continuation": map[string]any{
					"type":        "string",
					"description": "Optional pagination continuation token",
				},
				"budget": budgetSchema,
			},
			"required": []string{"branch"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch       string       `json:"branch"`
				Node         string       `json:"node,omitempty"`
				EntityID     string       `json:"entity_id,omitempty"`
				Commit       *string      `json:"commit,omitempty"`
				AllParents   bool         `json:"all_parents,omitempty"`
				Continuation string       `json:"continuation,omitempty"`
				Budget       *budgetInput `json:"budget,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			entityID := in.Node
			if entityID == "" {
				entityID = in.EntityID
			}
			if in.Branch == "" || entityID == "" {
				return nil, errors.New("branch and node (or entity_id) are required")
			}
			return withTool(stateDirProvider, func(tool *resolve.ResolveTool) (any, error) {
				return tool.SPLHistory(ctx, resolve.HistoryRequest{
					Selector: resolve.SnapshotSelector{
						Branch: in.Branch,
						Commit: in.Commit,
					},
					EntityID:          entityID,
					AllParents:        in.AllParents,
					ContinuationToken: in.Continuation,
					Budget:            in.Budget.toBudget(),
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
