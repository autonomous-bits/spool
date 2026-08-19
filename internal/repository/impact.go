package repository

import (
	"errors"
	"sort"
)

var (
	// ErrMissingImpactDelta reports an impact request without hypothetical mutations.
	ErrMissingImpactDelta = errors.New("impact delta is required")
	// ErrInvalidImpactBudget reports a negative depth or non-positive visited-node limit.
	ErrInvalidImpactBudget = errors.New("impact traversal budget is invalid")
)

// ImpactRequest describes a hypothetical, non-persistent graph change and the
// snapshot against which it is analyzed.
type ImpactRequest struct {
	// Selector identifies the snapshot to analyze.
	Selector DiffSelector `json:"selector"`
	// Delta contains validated hypothetical mutations that are never persisted.
	Delta []MutationOperation `json:"delta"`
	// MaxDepth limits outgoing dependency traversal distance.
	MaxDepth int `json:"maxDepth"`
	// MaxVisited limits nodes traversed and returned.
	MaxVisited int `json:"maxVisited"`
}

// ImpactEntry identifies an impacted node and its canonical supporting path.
type ImpactEntry struct {
	// Node is the impacted node.
	Node Node `json:"node"`
	// Path is a canonical path from a changed seed node to Node.
	Path []string `json:"path"`
	// Distance is the number of edges in Path.
	Distance int `json:"distance"`
}

// ImpactResult describes the snapshot analyzed and its bounded impacted nodes.
type ImpactResult struct {
	// Commit identifies the selected commit.
	Commit ObjectID `json:"commit"`
	// Snapshot identifies the selected graph snapshot.
	Snapshot ObjectID `json:"snapshot"`
	// Impacts contains canonically ordered impacted nodes.
	Impacts []ImpactEntry `json:"impacts"`
}

// Impact applies Delta in memory and analyzes its outgoing dependency impact.
// Until the schema models dependency types, weights, criticality, and
// validators, every edge is an outgoing unit-weight dependency, all nodes have
// zero criticality, and there are no validators to evaluate.
func (r *Repository) Impact(request ImpactRequest) (ImpactResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.ensureOpenLocked(); err != nil {
		return ImpactResult{}, err
	}
	if len(request.Delta) == 0 {
		return ImpactResult{}, ErrMissingImpactDelta
	}
	if request.MaxDepth < 0 || request.MaxVisited <= 0 {
		return ImpactResult{}, ErrInvalidImpactBudget
	}
	commitID, err := r.resolveDiffSelectorLocked(request.Selector)
	if err != nil {
		return ImpactResult{}, err
	}
	baseSnapshot := r.commits[commitID].Snapshot
	snapshot := r.snapshots[baseSnapshot]
	nodes, edges := cloneNodes(r.projections[snapshot.NodeRoot]), cloneEdges(r.edgeProjections[baseSnapshot])
	delta, err := normalizeMutationOperations(request.Delta)
	if err != nil {
		return ImpactResult{}, err
	}
	if err := r.validateMutationBatchLocked(commitID, delta); err != nil {
		return ImpactResult{}, err
	}
	seeds := impactSeeds(delta, edges)
	applyMutationOperations(nodes, edges, delta)

	return ImpactResult{
		Commit:   commitID,
		Snapshot: baseSnapshot,
		Impacts:  traverseImpact(nodes, edges, seeds, request.MaxDepth, request.MaxVisited),
	}, nil
}

func applyMutationOperations(nodes map[string]Node, edges map[string]Edge, operations []MutationOperation) {
	for _, operation := range operations {
		if operation.Entity == "node" {
			switch operation.Action {
			case "delete":
				delete(nodes, operation.ID)
			case "update":
				node := nodes[operation.ID]
				node.Title = operation.Title
				if operation.Labels != nil {
					node.Labels = operation.Labels
				}
				if operation.Properties != nil {
					node.Properties = operation.Properties
				}
				nodes[operation.ID] = node
			default:
				nodes[operation.ID] = Node{
					ID:         operation.ID,
					Title:      operation.Title,
					Labels:     operation.Labels,
					Properties: operation.Properties,
				}
			}
		} else {
			switch operation.Action {
			case "delete":
				delete(edges, operation.ID)
			case "update":
				edge := edges[operation.ID]
				edge.Source, edge.Target = operation.Source, operation.Target
				if operation.Type != "" {
					edge.Type = operation.Type
				}
				if operation.Properties != nil {
					edge.Properties = operation.Properties
				}
				edges[operation.ID] = edge
			default:
				edges[operation.ID] = Edge{
					ID:         operation.ID,
					Source:     operation.Source,
					Target:     operation.Target,
					Type:       operation.Type,
					Properties: operation.Properties,
				}
			}
		}
	}
}

func impactSeeds(operations []MutationOperation, edges map[string]Edge) []string {
	seeds := make(map[string]struct{})
	for _, operation := range operations {
		if operation.Entity == "node" {
			seeds[operation.ID] = struct{}{}
			continue
		}
		if operation.Action == "delete" || operation.Action == "update" {
			if edge, exists := edges[operation.ID]; exists {
				seeds[edge.Source], seeds[edge.Target] = struct{}{}, struct{}{}
			}
		}
		if operation.Action == "delete" {
			continue
		}
		seeds[operation.Source], seeds[operation.Target] = struct{}{}, struct{}{}
	}
	result := make([]string, 0, len(seeds))
	for id := range seeds {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

func traverseImpact(nodes map[string]Node, edges map[string]Edge, seeds []string, maxDepth, maxVisited int) []ImpactEntry {
	adjacency := make(map[string][]string)
	for _, edge := range edges {
		if _, sourceExists := nodes[edge.Source]; !sourceExists {
			continue
		}
		if _, targetExists := nodes[edge.Target]; !targetExists {
			continue
		}
		adjacency[edge.Source] = append(adjacency[edge.Source], edge.Target)
	}
	for source := range adjacency {
		sort.Strings(adjacency[source])
	}

	type visit struct {
		id   string
		path []string
	}
	queue := make([]visit, 0, len(seeds))
	seen := make(map[string]struct{})
	for _, id := range seeds {
		if _, exists := nodes[id]; exists && len(seen) < maxVisited {
			seen[id] = struct{}{}
			queue = append(queue, visit{id: id, path: []string{id}})
		}
	}
	impacts := make([]ImpactEntry, 0, len(queue))
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		impacts = append(impacts, ImpactEntry{
			Node: nodes[current.id].clone(), Path: current.path, Distance: len(current.path) - 1,
		})
		if len(current.path)-1 == maxDepth {
			continue
		}
		for _, target := range adjacency[current.id] {
			if _, visited := seen[target]; visited || len(seen) >= maxVisited {
				continue
			}
			seen[target] = struct{}{}
			path := append(append([]string(nil), current.path...), target)
			queue = append(queue, visit{id: target, path: path})
		}
	}
	sort.Slice(impacts, func(i, j int) bool {
		if impacts[i].Distance != impacts[j].Distance {
			return impacts[i].Distance < impacts[j].Distance
		}
		return impacts[i].Node.ID < impacts[j].Node.ID
	})
	return impacts
}
