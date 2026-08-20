package repository

import (
	"context"
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
	// Commit identifies the already pinned snapshot to analyze.
	Commit ObjectID `json:"commit"`
	// Delta contains validated hypothetical mutations that are never persisted.
	Delta []MutationOperation `json:"delta"`
	// MaxDepth limits outgoing dependency traversal distance.
	MaxDepth int `json:"maxDepth"`
	// MaxVisited limits nodes traversed and returned.
	MaxVisited int `json:"maxVisited"`
	// MaxRows limits impacts in one page. Zero returns up to MaxVisited, preserving
	// legacy behavior.
	MaxRows int `json:"maxRows,omitempty"`
	// MaxResponseBytes limits the JSON-encoded ImpactResult payload. Zero preserves
	// legacy unbounded behavior; adapters must reserve envelope overhead.
	MaxResponseBytes int `json:"maxResponseBytes,omitempty"`
	// ContinuationToken resumes a matching impact query.
	ContinuationToken string `json:"continuationToken,omitempty"`
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
	// ContinuationToken resumes remaining impacts with the same request.
	ContinuationToken string `json:"continuationToken,omitempty"`
	// CapacityExhausted reports that MaxVisited prevented further traversal.
	CapacityExhausted bool `json:"capacityExhausted,omitempty"`
}

// Impact applies Delta in memory and analyzes its outgoing dependency impact.
// Until the schema models dependency types, weights, criticality, and
// validators, every edge is an outgoing unit-weight dependency, all nodes have
// zero criticality, and there are no validators to evaluate.
func (r *Repository) Impact(request ImpactRequest) (ImpactResult, error) {
	return r.ImpactContext(context.Background(), request)
}

// ImpactContext applies Delta in memory, honors ctx while traversing, and returns
// a deterministic page. MaxResponseBytes applies only to ImpactResult.
func (r *Repository) ImpactContext(ctx context.Context, request ImpactRequest) (ImpactResult, error) {
	if err := ctx.Err(); err != nil {
		return ImpactResult{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.ensureOpenLocked(); err != nil {
		return ImpactResult{}, err
	}
	if len(request.Delta) == 0 {
		return ImpactResult{}, ErrMissingImpactDelta
	}
	if request.MaxDepth < 0 || request.MaxVisited <= 0 || request.MaxRows < 0 || request.MaxResponseBytes < 0 {
		return ImpactResult{}, ErrInvalidImpactBudget
	}
	commitID, err := r.requirePinnedCommitLocked(request.Commit)
	if err != nil {
		return ImpactResult{}, err
	}
	baseSnapshot := r.commits[commitID].Snapshot
	snapshot := r.snapshots[baseSnapshot]
	nodes, err := cloneImpactNodes(ctx, r.projections[snapshot.NodeRoot])
	if err != nil {
		return ImpactResult{}, err
	}
	edges, err := cloneImpactEdges(ctx, r.edgeProjections[baseSnapshot])
	if err != nil {
		return ImpactResult{}, err
	}
	delta, err := normalizeMutationOperations(request.Delta)
	if err != nil {
		return ImpactResult{}, err
	}
	if err := r.validateMutationBatchLocked(commitID, delta); err != nil {
		return ImpactResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return ImpactResult{}, err
	}
	seeds := impactSeeds(delta, edges)
	applyMutationOperations(nodes, edges, delta)
	impacts, exhausted, traversalErr := traverseImpactContext(ctx, nodes, edges, seeds, request.MaxDepth, request.MaxVisited)
	if traversalErr != nil && !errors.Is(traversalErr, context.DeadlineExceeded) {
		return ImpactResult{}, traversalErr
	}
	fingerprint := queryFingerprint(struct {
		Commit           ObjectID
		Delta            []MutationOperation
		MaxDepth         int
		MaxVisited       int
		MaxRows          int
		MaxResponseBytes int
	}{commitID, delta, request.MaxDepth, request.MaxVisited, request.MaxRows, request.MaxResponseBytes})
	offset, err := decodeContinuation(request.ContinuationToken, fingerprint)
	if err != nil {
		return ImpactResult{}, err
	}
	if offset > len(impacts) {
		return ImpactResult{}, ErrInvalidContinuation
	}
	timedOut := errors.Is(traversalErr, context.DeadlineExceeded) && len(impacts) > offset
	if traversalErr != nil && !timedOut {
		return ImpactResult{}, traversalErr
	}
	result := ImpactResult{
		Commit:            commitID,
		Snapshot:          baseSnapshot,
		Impacts:           make([]ImpactEntry, 0),
		CapacityExhausted: exhausted,
	}
	if !resultFits(result, request.MaxResponseBytes) {
		return ImpactResult{}, ErrResponseBudgetTooSmall
	}
	limit := request.MaxVisited
	if request.MaxRows > 0 && request.MaxRows < limit {
		limit = request.MaxRows
	}
	next := offset
	pageCtx := ctx
	if timedOut {
		pageCtx = context.Background()
	}
	for next < len(impacts) && len(result.Impacts) < limit {
		if err := pageCtx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) && next > offset {
				result.ContinuationToken = encodeContinuation(fingerprint, next)
				return result, err
			}
			return ImpactResult{}, err
		}
		candidate := result
		candidate.Impacts = append(append([]ImpactEntry(nil), result.Impacts...), impacts[next])
		candidate.ContinuationToken = encodeContinuation(fingerprint, next+1)
		if !resultFits(candidate, request.MaxResponseBytes) {
			break
		}
		if next+1 == len(impacts) {
			candidate.ContinuationToken = ""
		}
		result = candidate
		next++
	}
	if next < len(impacts) {
		if next == offset {
			return ImpactResult{}, ErrResponseBudgetTooSmall
		}
		result.ContinuationToken = encodeContinuation(fingerprint, next)
		if !resultFits(result, request.MaxResponseBytes) {
			return ImpactResult{}, ErrResponseBudgetTooSmall
		}
	}
	if timedOut {
		result.ContinuationToken = encodeContinuation(fingerprint, next)
		if !resultFits(result, request.MaxResponseBytes) {
			return ImpactResult{}, ErrResponseBudgetTooSmall
		}
		return result, traversalErr
	}
	return result, nil
}

func cloneImpactNodes(ctx context.Context, source map[string]Node) (map[string]Node, error) {
	result := make(map[string]Node, len(source))
	for id, node := range source {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result[id] = node.clone()
	}
	return result, nil
}

func cloneImpactEdges(ctx context.Context, source map[string]Edge) (map[string]Edge, error) {
	result := make(map[string]Edge, len(source))
	for id, edge := range source {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result[id] = edge.clone()
	}
	return result, nil
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

func traverseImpactContext(ctx context.Context, nodes map[string]Node, edges map[string]Edge, seeds []string, maxDepth, maxVisited int) ([]ImpactEntry, bool, error) {
	adjacency := make(map[string][]string)
	for _, edge := range edges {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		if _, sourceExists := nodes[edge.Source]; !sourceExists {
			continue
		}
		if _, targetExists := nodes[edge.Target]; !targetExists {
			continue
		}
		adjacency[edge.Source] = append(adjacency[edge.Source], edge.Target)
	}
	for source := range adjacency {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		sort.Strings(adjacency[source])
	}

	type visit struct {
		id   string
		path []string
	}
	queue := make([]visit, 0, len(seeds))
	seen := make(map[string]struct{})
	exhausted := false
	for _, id := range seeds {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		if _, exists := nodes[id]; exists && len(seen) < maxVisited {
			seen[id] = struct{}{}
			queue = append(queue, visit{id: id, path: []string{id}})
		} else if _, exists := nodes[id]; exists {
			exhausted = true
		}
	}
	impacts := make([]ImpactEntry, 0, len(queue))
	for len(queue) > 0 {
		nextQueue := make([]visit, 0)
		levelImpacts := make([]ImpactEntry, 0, len(queue))
		for _, current := range queue {
			if err := ctx.Err(); err != nil {
				return impacts, exhausted, err
			}
			levelImpacts = append(levelImpacts, ImpactEntry{
				Node: nodes[current.id].clone(), Path: current.path, Distance: len(current.path) - 1,
			})
			if len(current.path)-1 == maxDepth {
				continue
			}
			for _, target := range adjacency[current.id] {
				if err := ctx.Err(); err != nil {
					return impacts, exhausted, err
				}
				if _, visited := seen[target]; visited {
					continue
				}
				if len(seen) >= maxVisited {
					exhausted = true
					continue
				}
				seen[target] = struct{}{}
				path := append(append([]string(nil), current.path...), target)
				nextQueue = append(nextQueue, visit{id: target, path: path})
			}
		}
		sort.Slice(levelImpacts, func(i, j int) bool { return levelImpacts[i].Node.ID < levelImpacts[j].Node.ID })
		impacts = append(impacts, levelImpacts...)
		queue = nextQueue
	}
	sort.Slice(impacts, func(i, j int) bool {
		if impacts[i].Distance != impacts[j].Distance {
			return impacts[i].Distance < impacts[j].Distance
		}
		return impacts[i].Node.ID < impacts[j].Node.ID
	})
	return impacts, exhausted, nil
}
