package ctxgit

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/autonomous-bits/spool/internal/repository"
)

// Overlap is a path/ID that already exists on the protected checkout. The new
// content is written on the short-lived branch so humans resolve it in the PR.
type Overlap struct {
	Entity string `json:"entity"`
	ID     string `json:"id"`
	Path   string `json:"path"`
}

// ApplyResult is the in-memory outcome of applying one mutation batch.
type ApplyResult struct {
	Graph    *Graph
	Overlaps []Overlap
	Written  []string
	Deleted  []string
	Warnings []string
}

func namespaceOperations(repositoryID string, operations []repository.MutationOperation) []repository.MutationOperation {
	out := make([]repository.MutationOperation, len(operations))
	for i, op := range operations {
		op.ID = NamespaceID(repositoryID, op.ID)
		if op.Entity == "edge" {
			op.Source = NamespaceID(repositoryID, op.Source)
			op.Target = NamespaceID(repositoryID, op.Target)
		}
		out[i] = op
	}
	return out
}

func normalizeOperations(operations []repository.MutationOperation) ([]repository.MutationOperation, error) {
	normalized := make([]repository.MutationOperation, len(operations))
	for i, op := range operations {
		next, err := op.Normalize()
		if err != nil {
			return nil, fmt.Errorf("%w: normalize operation %d: %w", repository.ErrInvalidMutationBatch, i, err)
		}
		normalized[i] = next
	}
	return normalized, nil
}

func validateBatch(base *Graph, operations []repository.MutationOperation) error {
	if len(operations) == 0 {
		return repository.ErrInvalidMutationBatch
	}
	addedNodes := map[string]struct{}{}
	deletedNodes := map[string]struct{}{}
	for _, op := range operations {
		if op.Action == "add" && op.Entity == "node" && op.ID != "" {
			addedNodes[op.ID] = struct{}{}
		}
		if op.Action == "delete" && op.Entity == "node" && op.ID != "" {
			deletedNodes[op.ID] = struct{}{}
		}
	}
	seen := map[string]struct{}{}
	var genericInvalid, missingEndpoint bool
	for _, op := range operations {
		if (op.Action != "add" && op.Action != "update" && op.Action != "delete") ||
			(op.Entity != "node" && op.Entity != "edge") || op.ID == "" {
			genericInvalid = true
			continue
		}
		key := op.Entity + ":" + op.ID
		if _, dup := seen[key]; dup {
			genericInvalid = true
			continue
		}
		seen[key] = struct{}{}
		switch op.Entity {
		case "node":
			_, exists := base.Nodes[op.ID]
			switch op.Action {
			case "add":
				if op.Title == "" {
					genericInvalid = true
				}
				// Existing IDs are overlaps, not silent rejects — they surface in the PR.
			case "update":
				if !exists || op.Title == "" {
					genericInvalid = true
				}
			case "delete":
				if !exists {
					genericInvalid = true
				}
			}
		case "edge":
			_, exists := base.Edges[op.ID]
			switch op.Action {
			case "add":
				if op.Source == "" || op.Target == "" {
					genericInvalid = true
					continue
				}
				if !nodeExists(op.Source, base.Nodes, addedNodes, deletedNodes) ||
					!nodeExists(op.Target, base.Nodes, addedNodes, deletedNodes) {
					missingEndpoint = true
				}
			case "update":
				if !exists || op.Source == "" || op.Target == "" {
					genericInvalid = true
					continue
				}
				if !nodeExists(op.Source, base.Nodes, addedNodes, deletedNodes) ||
					!nodeExists(op.Target, base.Nodes, addedNodes, deletedNodes) {
					missingEndpoint = true
				}
			case "delete":
				if !exists {
					genericInvalid = true
				}
			}
		}
	}
	if missingEndpoint {
		return repository.ErrMissingEdgeEndpoint
	}
	if genericInvalid {
		return repository.ErrInvalidMutationBatch
	}
	return nil
}

func nodeExists(id string, existing map[string]repository.Node, added, deleted map[string]struct{}) bool {
	if _, gone := deleted[id]; gone {
		return false
	}
	if _, ok := existing[id]; ok {
		return true
	}
	_, ok := added[id]
	return ok
}

func applyOperations(base *Graph, operations []repository.MutationOperation) (*Graph, []Overlap, error) {
	next := cloneGraph(base)
	var overlaps []Overlap
	for _, op := range operations {
		switch op.Entity {
		case "node":
			rel, err := nodeRelPath(op.ID)
			if err != nil {
				return nil, nil, err
			}
			switch op.Action {
			case "delete":
				delete(next.Nodes, op.ID)
				delete(next.NodeFiles, op.ID)
			case "update":
				node := next.Nodes[op.ID]
				node.Title = op.Title
				if op.Labels != nil {
					node.Labels = op.Labels
				}
				if op.Properties != nil {
					node.Properties = op.Properties
				}
				normalized, err := node.Normalize()
				if err != nil {
					return nil, nil, err
				}
				next.Nodes[op.ID] = normalized
				next.NodeFiles[op.ID] = rel
				overlaps = append(overlaps, Overlap{Entity: "node", ID: op.ID, Path: rel})
			default:
				if _, exists := base.Nodes[op.ID]; exists {
					overlaps = append(overlaps, Overlap{Entity: "node", ID: op.ID, Path: rel})
				}
				node := repository.Node{ID: op.ID, Title: op.Title, Labels: op.Labels, Properties: op.Properties}
				normalized, err := node.Normalize()
				if err != nil {
					return nil, nil, err
				}
				next.Nodes[op.ID] = normalized
				next.NodeFiles[op.ID] = rel
			}
		case "edge":
			rel, err := edgeRelPath(op.ID)
			if err != nil {
				return nil, nil, err
			}
			switch op.Action {
			case "delete":
				delete(next.Edges, op.ID)
				delete(next.EdgeFiles, op.ID)
			case "update":
				edge := next.Edges[op.ID]
				edge.Source, edge.Target = op.Source, op.Target
				if op.Type != "" {
					edge.Type = op.Type
				}
				if op.Properties != nil {
					edge.Properties = op.Properties
				}
				normalized, err := edge.Normalize()
				if err != nil {
					return nil, nil, err
				}
				next.Edges[op.ID] = normalized
				next.EdgeFiles[op.ID] = rel
				overlaps = append(overlaps, Overlap{Entity: "edge", ID: op.ID, Path: rel})
			default:
				if _, exists := base.Edges[op.ID]; exists {
					overlaps = append(overlaps, Overlap{Entity: "edge", ID: op.ID, Path: rel})
				}
				edge := repository.Edge{ID: op.ID, Source: op.Source, Target: op.Target, Type: op.Type, Properties: op.Properties}
				normalized, err := edge.Normalize()
				if err != nil {
					return nil, nil, err
				}
				next.Edges[op.ID] = normalized
				next.EdgeFiles[op.ID] = rel
			}
		}
	}
	schema, err := schemaSnapshot(next)
	if err != nil {
		return nil, nil, err
	}
	if err := repository.ValidateSchemaSnapshot(schema, next.Nodes, next.Edges); err != nil {
		return nil, nil, err
	}
	return next, overlaps, nil
}

func cloneGraph(g *Graph) *Graph {
	next := NewGraph()
	next.SchemaTOML = append([]byte(nil), g.SchemaTOML...)
	for id, node := range g.Nodes {
		next.Nodes[id] = node.Clone()
		next.NodeFiles[id] = g.NodeFiles[id]
	}
	for id, edge := range g.Edges {
		next.Edges[id] = edge.Clone()
		next.EdgeFiles[id] = g.EdgeFiles[id]
	}
	return next
}

func persistSchema(root string, schemaTOML []byte) error {
	if len(schemaTOML) == 0 {
		return nil
	}
	return writeSchemaFile(root, schemaTOML)
}

func persistGraphDiff(root string, before, after *Graph) (written, deleted []string, err error) {
	for id := range before.Nodes {
		if _, ok := after.Nodes[id]; ok {
			continue
		}
		rel := before.NodeFiles[id]
		if rel == "" {
			rel, err = nodeRelPath(id)
			if err != nil {
				return nil, nil, err
			}
		}
		if rmErr := os.Remove(filepath.Join(root, filepath.FromSlash(rel))); rmErr != nil && !os.IsNotExist(rmErr) {
			return nil, nil, rmErr
		}
		deleted = append(deleted, rel)
	}
	for id := range before.Edges {
		if _, ok := after.Edges[id]; ok {
			continue
		}
		rel := before.EdgeFiles[id]
		if rel == "" {
			rel, err = edgeRelPath(id)
			if err != nil {
				return nil, nil, err
			}
		}
		if rmErr := os.Remove(filepath.Join(root, filepath.FromSlash(rel))); rmErr != nil && !os.IsNotExist(rmErr) {
			return nil, nil, rmErr
		}
		deleted = append(deleted, rel)
	}
	for id, node := range after.Nodes {
		rel := after.NodeFiles[id]
		if rel == "" {
			rel, err = nodeRelPath(id)
			if err != nil {
				return nil, nil, err
			}
			after.NodeFiles[id] = rel
		}
		if err := writeJSONFile(root, rel, node); err != nil {
			return nil, nil, err
		}
		written = append(written, rel)
	}
	for id, edge := range after.Edges {
		rel := after.EdgeFiles[id]
		if rel == "" {
			rel, err = edgeRelPath(id)
			if err != nil {
				return nil, nil, err
			}
			after.EdgeFiles[id] = rel
		}
		if err := writeJSONFile(root, rel, edge); err != nil {
			return nil, nil, err
		}
		written = append(written, rel)
	}
	sort.Strings(written)
	sort.Strings(deleted)
	return written, deleted, nil
}
