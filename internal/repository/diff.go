package repository

import (
	"context"
	"errors"
	"sort"
)

var (
	// ErrInvalidContinuation reports a continuation token from another request or with invalid encoding.
	ErrInvalidContinuation = errors.New("diff continuation token does not match request")
	// ErrInvalidDiffBudget reports a non-positive row budget.
	ErrInvalidDiffBudget = errors.New("diff max rows must be positive")
)

// DiffFilter restricts diff changes by identifiers or node title substring.
type DiffFilter struct {
	// NodeIDs restricts returned node changes when non-empty.
	NodeIDs []string `json:"nodeIds,omitempty"`
	// EdgeIDs restricts returned edge changes when non-empty.
	EdgeIDs []string `json:"edgeIds,omitempty"`
	// NodeTitleSubstr restricts node changes to titles containing this substring.
	NodeTitleSubstr string `json:"nodeTitleSubstring,omitempty"`
}

// DiffRequest describes a bounded, optionally filtered comparison of two snapshots.
type DiffRequest struct {
	// Base identifies the already pinned older comparison commit.
	Base ObjectID `json:"base"`
	// Target identifies the already pinned newer comparison commit.
	Target ObjectID `json:"target"`
	// Filter optionally limits the returned changes.
	Filter DiffFilter `json:"filter,omitempty"`
	// MaxRows limits changes and context entries returned in this page.
	MaxRows int `json:"maxRows"`
	// MaxResponseBytes limits the JSON-encoded response size.
	MaxResponseBytes int `json:"maxResponseBytes"`
	// IncludeOneHop includes related unchanged nodes and edges within remaining budgets.
	IncludeOneHop bool `json:"includeOneHop,omitempty"`
	// ContinuationToken resumes a prior request with matching comparison and budgets.
	ContinuationToken string `json:"continuationToken,omitempty"`
}

// DiffEntry describes an added, removed, or modified graph entity.
type DiffEntry struct {
	// Entity is "node" or "edge".
	Entity string `json:"entity"`
	// Change is "added", "removed", or "modified".
	Change string `json:"change"`
	// ID identifies the changed entity.
	ID string `json:"id"`
	// Node is populated when Entity is "node".
	Node *Node `json:"node,omitempty"`
	// Edge is populated when Entity is "edge".
	Edge *Edge `json:"edge,omitempty"`
}

// DiffContext describes an unchanged entity included as one-hop context.
type DiffContext struct {
	// Entity is "node" or "edge".
	Entity string `json:"entity"`
	// ID identifies the context entity.
	ID string `json:"id"`
	// Node is populated for node context.
	Node *Node `json:"node,omitempty"`
	// Edge is populated for edge context.
	Edge *Edge `json:"edge,omitempty"`
}

// DiffResult is one bounded page of changes and optional related context.
type DiffResult struct {
	// BaseCommit is the resolved commit selected by Base.
	BaseCommit ObjectID `json:"baseCommit"`
	// TargetCommit is the resolved commit selected by Target.
	TargetCommit ObjectID `json:"targetCommit"`
	// Changes contains the ordered page of matching changes.
	Changes []DiffEntry `json:"changes"`
	// Context contains related unchanged entities when requested and budget permits.
	Context []DiffContext `json:"context,omitempty"`
	// ContinuationToken resumes remaining changes with the same request.
	ContinuationToken string `json:"continuationToken,omitempty"`
	// ContextTruncated reports that requested one-hop context exceeded a page budget.
	ContextTruncated bool `json:"contextTruncated,omitempty"`
}

// Diff returns a deterministic, budgeted page comparing two repository snapshots.
func (r *Repository) Diff(request DiffRequest) (DiffResult, error) {
	return r.DiffContext(context.Background(), request)
}

// DiffContext returns a deterministic, budgeted page and stops scanning when ctx
// is canceled. MaxResponseBytes applies to DiffResult, not an adapter envelope.
func (r *Repository) DiffContext(ctx context.Context, request DiffRequest) (DiffResult, error) {
	if err := ctx.Err(); err != nil {
		return DiffResult{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.ensureOpenLocked(); err != nil {
		return DiffResult{}, err
	}
	base, err := r.requirePinnedCommitLocked(request.Base)
	if err != nil {
		return DiffResult{}, err
	}
	target, err := r.requirePinnedCommitLocked(request.Target)
	if err != nil {
		return DiffResult{}, err
	}
	fingerprint := diffFingerprint(base, target, request)
	offset, err := decodeContinuation(request.ContinuationToken, fingerprint)
	if err != nil {
		return DiffResult{}, err
	}
	if request.MaxRows <= 0 {
		return DiffResult{}, ErrInvalidDiffBudget
	}
	if request.MaxResponseBytes <= 0 {
		return DiffResult{}, ErrResponseBudgetTooSmall
	}
	changes, scanErr := r.diffChangesContextLocked(ctx, base, target, request.Filter)
	if scanErr != nil && !errors.Is(scanErr, context.DeadlineExceeded) {
		return DiffResult{}, scanErr
	}
	if offset > len(changes) {
		if scanErr != nil {
			return DiffResult{}, scanErr
		}
		return DiffResult{}, ErrInvalidContinuation
	}
	timedOut := errors.Is(scanErr, context.DeadlineExceeded) && len(changes) > offset
	if scanErr != nil && !timedOut {
		return DiffResult{}, scanErr
	}
	result := DiffResult{BaseCommit: base, TargetCommit: target, Changes: make([]DiffEntry, 0)}
	if !resultFits(result, request.MaxResponseBytes) {
		return DiffResult{}, ErrResponseBudgetTooSmall
	}
	limit := request.MaxRows
	next := offset
	pageCtx := ctx
	if timedOut {
		pageCtx = context.Background()
	}
	for next < len(changes) && len(result.Changes) < limit {
		if err := pageCtx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) && next > offset {
				result.ContinuationToken = encodeContinuation(fingerprint, next)
				return result, err
			}
			return DiffResult{}, err
		}
		candidate := result
		candidate.Changes = append(append([]DiffEntry(nil), result.Changes...), changes[next])
		candidate.ContinuationToken = encodeContinuation(fingerprint, next+1)
		if !resultFits(candidate, request.MaxResponseBytes) {
			break
		}
		if next+1 == len(changes) {
			candidate.ContinuationToken = ""
		}
		result = candidate
		next++
	}
	if next < len(changes) {
		if next == offset {
			return DiffResult{}, ErrResponseBudgetTooSmall
		}
		result.ContinuationToken = encodeContinuation(fingerprint, next)
		if !resultFits(result, request.MaxResponseBytes) {
			return DiffResult{}, ErrResponseBudgetTooSmall
		}
	}
	if timedOut {
		result.ContinuationToken = encodeContinuation(fingerprint, next)
		if !resultFits(result, request.MaxResponseBytes) {
			return DiffResult{}, ErrResponseBudgetTooSmall
		}
		return result, scanErr
	}
	if request.IncludeOneHop {
		contextEntries, err := r.diffContextContextLocked(ctx, base, target, result.Changes)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) && len(result.Changes) > 0 {
				return result, err
			}
			return DiffResult{}, err
		}
		for _, contextEntry := range contextEntries {
			if err := ctx.Err(); err != nil {
				if errors.Is(err, context.DeadlineExceeded) && (len(result.Changes) > 0 || len(result.Context) > 0) {
					return result, err
				}
				return DiffResult{}, err
			}
			if len(result.Changes)+len(result.Context) >= limit {
				result.ContextTruncated = true
				break
			}
			candidate := result
			candidate.Context = append(append([]DiffContext(nil), result.Context...), contextEntry)
			if !resultFits(candidate, request.MaxResponseBytes) {
				result.ContextTruncated = true
				break
			}
			result = candidate
		}
		if result.ContextTruncated {
			for !resultFits(result, request.MaxResponseBytes) && len(result.Context) > 0 {
				result.Context = result.Context[:len(result.Context)-1]
			}
			if !resultFits(result, request.MaxResponseBytes) {
				return DiffResult{}, ErrResponseBudgetTooSmall
			}
		}
	}
	return result, nil
}

func (r *Repository) requirePinnedCommitLocked(commit ObjectID) (ObjectID, error) {
	if _, ok := r.commits[commit]; !ok {
		return "", ErrCommitNotFound
	}
	return commit, nil
}

func (r *Repository) diffChangesContextLocked(ctx context.Context, base, target ObjectID, filter DiffFilter) ([]DiffEntry, error) {
	baseSnapshot, targetSnapshot := r.commits[base].Snapshot, r.commits[target].Snapshot
	baseNodes, targetNodes := r.projections[r.snapshots[baseSnapshot].NodeRoot], r.projections[r.snapshots[targetSnapshot].NodeRoot]
	baseEdges, targetEdges := r.edgeProjections[baseSnapshot], r.edgeProjections[targetSnapshot]
	nodes, err := diffNodeChangesContext(ctx, baseNodes, targetNodes, filter)
	if err != nil {
		return nodes, err
	}
	edges, err := diffEdgeChangesContext(ctx, baseEdges, targetEdges, filter)
	if err != nil {
		return append(nodes, edges...), err
	}
	return append(nodes, edges...), nil
}

func diffNodeChangesContext(ctx context.Context, base, target map[string]Node, filter DiffFilter) ([]DiffEntry, error) {
	return diffEntriesContext(ctx, base, target, filter.NodeIDs, filter.NodeTitleSubstr,
		func(node Node) DiffEntry {
			node = node.clone()
			return DiffEntry{Entity: "node", ID: node.ID, Node: &node}
		},
		func(left, right Node) bool { return left.Equal(right) },
	)
}

func diffEdgeChangesContext(ctx context.Context, base, target map[string]Edge, filter DiffFilter) ([]DiffEntry, error) {
	if filter.NodeTitleSubstr != "" {
		return nil, nil
	}
	return diffEntriesContext(ctx, base, target, filter.EdgeIDs, "",
		func(edge Edge) DiffEntry {
			edge = edge.clone()
			return DiffEntry{Entity: "edge", ID: edge.ID, Edge: &edge}
		},
		func(left, right Edge) bool { return left.Equal(right) },
	)
}

func diffEntriesContext[T any](ctx context.Context, base, target map[string]T, ids []string, title string, entry func(T) DiffEntry, equal func(T, T) bool) ([]DiffEntry, error) {
	allowed := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		allowed[id] = struct{}{}
	}
	targetIDs := make([]string, 0, len(target))
	for id := range target {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		targetIDs = append(targetIDs, id)
	}
	baseIDs := make([]string, 0, len(base))
	for id := range base {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		baseIDs = append(baseIDs, id)
	}
	sort.Strings(targetIDs)
	sort.Strings(baseIDs)

	consider := func(id string, value T) bool {
		if len(allowed) > 0 {
			if _, ok := allowed[id]; !ok {
				return false
			}
		}
		if title != "" {
			node, ok := any(value).(Node)
			return ok && contains(node.Title, title)
		}
		return true
	}
	result := make([]DiffEntry, 0)
	for _, id := range targetIDs {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		targetValue := target[id]
		if _, exists := base[id]; exists || !consider(id, targetValue) {
			continue
		}
		item := entry(targetValue)
		item.Change = "added"
		result = append(result, item)
	}
	for _, id := range baseIDs {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if _, exists := target[id]; exists {
			continue
		}
		baseValue := base[id]
		if !consider(id, baseValue) {
			continue
		}
		item := entry(baseValue)
		item.Change = "removed"
		result = append(result, item)
	}
	for _, id := range targetIDs {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		targetValue := target[id]
		baseValue, exists := base[id]
		if !exists || equal(baseValue, targetValue) || !consider(id, targetValue) {
			continue
		}
		item := entry(targetValue)
		item.Change = "modified"
		result = append(result, item)
	}
	return result, nil
}

func (r *Repository) diffContextContextLocked(ctx context.Context, base, target ObjectID, changes []DiffEntry) ([]DiffContext, error) {
	baseSnapshot, targetSnapshot := r.commits[base].Snapshot, r.commits[target].Snapshot
	nodes := make(map[string]Node)
	edges := make(map[string]Edge)
	for _, snapshot := range []ObjectID{baseSnapshot, targetSnapshot} {
		for id, node := range r.projections[r.snapshots[snapshot].NodeRoot] {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			nodes[id] = node
		}
		for id, edge := range r.edgeProjections[snapshot] {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			edges[id] = edge
		}
	}
	changed := make(map[string]struct{}, len(changes))
	anchors := make(map[string]struct{})
	for _, change := range changes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		changed[change.Entity+":"+change.ID] = struct{}{}
		if change.Entity == "node" {
			anchors[change.ID] = struct{}{}
			continue
		}
		for _, snapshot := range []ObjectID{baseSnapshot, targetSnapshot} {
			if edge, exists := r.edgeProjections[snapshot][change.ID]; exists {
				anchors[edge.Source], anchors[edge.Target] = struct{}{}, struct{}{}
			}
		}
	}
	related := make(map[string]struct{})
	contextEdges := make(map[string]struct{})
	for _, edge := range edges {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		_, source := anchors[edge.Source]
		_, target := anchors[edge.Target]
		if source || target {
			related[edge.Source], related[edge.Target] = struct{}{}, struct{}{}
			contextEdges[edge.ID] = struct{}{}
		}
	}
	context := make([]DiffContext, 0)
	for id, node := range nodes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if _, isChanged := changed["node:"+id]; !isChanged {
			if _, related := related[id]; related {
				value := node.clone()
				context = append(context, DiffContext{Entity: "node", ID: id, Node: &value})
			}
		}
	}
	for id, edge := range edges {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if _, isChanged := changed["edge:"+id]; !isChanged {
			if _, isContext := contextEdges[id]; isContext {
				value := edge.clone()
				context = append(context, DiffContext{Entity: "edge", ID: id, Edge: &value})
			}
		}
	}
	sort.Slice(context, func(i, j int) bool {
		if context[i].Entity != context[j].Entity {
			return context[i].Entity == "node"
		}
		return context[i].ID < context[j].ID
	})
	return context, nil
}

func diffFingerprint(base, target ObjectID, request DiffRequest) string {
	filter := request.Filter
	filter.NodeIDs = append([]string(nil), filter.NodeIDs...)
	filter.EdgeIDs = append([]string(nil), filter.EdgeIDs...)
	sort.Strings(filter.NodeIDs)
	sort.Strings(filter.EdgeIDs)
	return queryFingerprint(struct {
		Base             ObjectID
		Target           ObjectID
		Filter           DiffFilter
		MaxRows          int
		MaxResponseBytes int
		IncludeOneHop    bool
	}{base, target, filter, request.MaxRows, request.MaxResponseBytes, request.IncludeOneHop})
}

func contains(value, substring string) bool {
	return len(substring) == 0 || (len(value) >= len(substring) && containsAt(value, substring))
}

func containsAt(value, substring string) bool {
	for i := 0; i+len(substring) <= len(value); i++ {
		if value[i:i+len(substring)] == substring {
			return true
		}
	}
	return false
}
