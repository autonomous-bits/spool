package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/autonomous-bits/spool/internal/contextual"
	"github.com/autonomous-bits/spool/internal/ctxgit"
	"github.com/autonomous-bits/spool/internal/repository"
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

func toolResolve(rt *runtime) Tool {
	return Tool{
		Name:        "spl_resolve",
		Description: "Resolve a node by stable ID from the bound context-git checkout.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"node": map[string]any{"type": "string", "description": "Stable node entity ID"},
			},
			"required": []string{"node"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Node string `json:"node"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Node == "" {
				return nil, errors.New("node is required")
			}
			session, err := rt.requireSession(ctx)
			if err != nil {
				return nil, err
			}
			return session.ResolveResult(in.Node)
		},
	}
}

func toolSearch(rt *runtime) Tool {
	return Tool{
		Name:        "spl_search",
		Description: "Lexical full-text search across the bound context-git projection.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "Search query terms or keywords"},
			},
			"required": []string{"query"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Query string `json:"query"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Query == "" {
				return nil, errors.New("query is required")
			}
			session, err := rt.requireSession(ctx)
			if err != nil {
				return nil, err
			}
			return session.SearchResult(ctx, in.Query, 20)
		},
	}
}

func toolSearchExpand(rt *runtime) Tool {
	return Tool{
		Name:        "spl_search_expand",
		Description: "Select lexical or typed-filter seed evidence, then expand bounded graph context from the bound checkout.",
		InputSchema: queryContextSchema(),
		Handler:     queryContextHandler(rt),
	}
}

func toolFilter(rt *runtime) Tool {
	return Tool{
		Name:        "spl_filter",
		Description: "Filter bound context-git nodes by label and property predicates.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"labels":     map[string]any{"type": "array", "description": "Node labels to match", "items": map[string]any{"type": "string"}},
				"predicates": predicatesSchema,
			},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Labels     []string                       `json:"labels,omitempty"`
				Predicates []repository.MetadataPredicate `json:"predicates,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			session, err := rt.requireSession(ctx)
			if err != nil {
				return nil, err
			}
			return session.FilterResult(in.Labels, in.Predicates, 50)
		},
	}
}

func toolQueryContext(rt *runtime) Tool {
	return Tool{
		Name:        "spl_query_context",
		Description: "Assemble evidence-focused bounded graph context from the bound checkout. Replaces the former spl_context query tool.",
		InputSchema: queryContextSchema(),
		Handler:     queryContextHandler(rt),
	}
}

func queryContextSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":      map[string]any{"type": "string", "description": "Lexical search query (mutually exclusive with label/predicates)"},
			"label":      map[string]any{"type": "string", "description": "Label filter (mutually exclusive with query)"},
			"labels":     map[string]any{"type": "array", "description": "Node labels (mutually exclusive with query)", "items": map[string]any{"type": "string"}},
			"predicates": predicatesSchema,
			"direction":  map[string]any{"type": "string", "description": "Edge traversal direction", "enum": []string{"out", "in", "both"}},
			"edge_types": map[string]any{"type": "array", "description": "Optional edge types to traverse", "items": map[string]any{"type": "string"}},
			"seed_limit": map[string]any{"type": "integer", "description": "Maximum number of seed nodes to expand from"},
		},
	}
}

func queryContextHandler(rt *runtime) ToolHandler {
	return func(ctx context.Context, args json.RawMessage) (any, error) {
		var in struct {
			Query      string                         `json:"query,omitempty"`
			Label      string                         `json:"label,omitempty"`
			Labels     []string                       `json:"labels,omitempty"`
			Predicates []repository.MetadataPredicate `json:"predicates,omitempty"`
			Direction  string                         `json:"direction,omitempty"`
			EdgeTypes  []string                       `json:"edge_types,omitempty"`
			SeedLimit  int                            `json:"seed_limit,omitempty"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return nil, err
		}
		if in.Query != "" && (in.Label != "" || len(in.Labels) > 0 || len(in.Predicates) > 0) {
			return nil, errors.New("query is mutually exclusive with label, labels, and predicates")
		}
		if in.Query == "" && in.Label == "" && len(in.Labels) == 0 && len(in.Predicates) == 0 {
			return nil, errors.New("either query, label, labels, or predicates must be specified")
		}
		var dir contextual.Direction
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
		labels := in.Labels
		if in.Label != "" && len(labels) == 0 {
			labels = []string{in.Label}
		}
		session, err := rt.requireSession(ctx)
		if err != nil {
			return nil, err
		}
		return session.QueryContext(ctx, ctxgit.QueryContextRequest{
			Query: in.Query, Labels: labels, Predicates: in.Predicates, Direction: dir, EdgeTypes: in.EdgeTypes, SeedLimit: in.SeedLimit,
		})
	}
}

func toolGraph(rt *runtime) Tool {
	return Tool{
		Name:        "spl_graph",
		Description: "Export every node and edge from the bound context-git checkout as JSON.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			session, err := rt.requireSession(ctx)
			if err != nil {
				return nil, err
			}
			return session.GraphResult()
		},
	}
}
