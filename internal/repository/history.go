package repository

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"time"
)

var (
	// ErrInvalidContainmentSelector reports a containment selector with an invalid combination of keys.
	ErrInvalidContainmentSelector = errors.New("containment selector must identify exactly one entity ID, snapshot, or natural key")
	// ErrEntityHistoryNotFound reports an empty entity identifier or no matching history.
	ErrEntityHistoryNotFound = errors.New("entity has no history")
)

// HistoryRequest selects an entity's commit history from an already pinned commit.
type HistoryRequest struct {
	// Commit identifies the traversal starting commit.
	Commit ObjectID `json:"commit"`
	// EntityID identifies the node or edge whose changes are returned.
	EntityID string `json:"entityId"`
	// AllParents includes all parent links rather than only each commit's first parent.
	AllParents bool `json:"allParents,omitempty"`
	// MaxRows limits entries in one page. Zero preserves the legacy unbounded read.
	MaxRows int `json:"maxRows,omitempty"`
	// MaxResponseBytes limits the JSON-encoded HistoryResult payload. Zero preserves
	// the legacy unbounded read; adapters must reserve their envelope overhead.
	MaxResponseBytes int `json:"maxResponseBytes,omitempty"`
	// ContinuationToken resumes a matching paged history request.
	ContinuationToken string `json:"continuationToken,omitempty"`
}

// HistoryEntry describes one commit that affected the requested entity.
type HistoryEntry struct {
	// Commit identifies the affecting commit.
	Commit ObjectID `json:"commit"`
	// BeforeSnapshot identifies the first-parent snapshot before the commit, when present.
	BeforeSnapshot ObjectID `json:"beforeSnapshot,omitempty"`
	// AfterSnapshot identifies the snapshot created by the commit.
	AfterSnapshot ObjectID `json:"afterSnapshot"`
	// ChangedFields names node fields changed by the commit.
	ChangedFields []string `json:"changedFields,omitempty"`
	// EdgeAdditions contains relevant added or updated edge values.
	EdgeAdditions []Edge `json:"edgeAdditions,omitempty"`
	// EdgeRemovals contains relevant removed or replaced edge values.
	EdgeRemovals []Edge `json:"edgeRemovals,omitempty"`
	// Author is the commit author.
	Author string `json:"author"`
	// Time is the UTC time recorded for the commit.
	Time time.Time `json:"time"`
	// Message is the commit message.
	Message string `json:"message"`
}

// HistoryResult contains commits affecting the requested entity in traversal order.
type HistoryResult struct {
	// Entries contains the matching history entries.
	Entries []HistoryEntry `json:"entries"`
	// ContinuationToken resumes remaining entries with the same request.
	ContinuationToken string `json:"continuationToken,omitempty"`
}

// ContainmentSelector identifies the entity or snapshot for branch containment lookup.
type ContainmentSelector struct {
	// EntityID selects branches with commits affecting this node or edge.
	EntityID string `json:"entityId,omitempty"`
	// SnapshotID selects branches whose ancestry contains this snapshot.
	SnapshotID ObjectID `json:"snapshotId,omitempty"`
	// NaturalKey is reserved for a natural-key selector.
	NaturalKey string `json:"naturalKey,omitempty"`
}

// BranchContainmentResult lists lexically ordered branches matching a containment selector.
type BranchContainmentResult struct {
	// Branches contains matching branch names in lexical order.
	Branches []string `json:"branches"`
	// ContinuationToken resumes remaining branches with the same request.
	ContinuationToken string `json:"continuationToken,omitempty"`
}

// BranchesContainingRequest describes a bounded containment query. The response
// byte limit applies to BranchContainmentResult, so public adapters can reserve
// their own envelope overhead before calling BranchesContainingContext.
type BranchesContainingRequest struct {
	// Selector identifies the entity or snapshot to find.
	Selector ContainmentSelector `json:"selector"`
	// MaxRows limits branch names in one page. It must be positive.
	MaxRows int `json:"maxRows"`
	// MaxResponseBytes limits the JSON-encoded repository result. It must be positive.
	MaxResponseBytes int `json:"maxResponseBytes"`
	// ContinuationToken resumes a matching containment query.
	ContinuationToken string `json:"continuationToken,omitempty"`
}

// History returns commits that affected the selected entity, or ErrEntityHistoryNotFound.
func (r *Repository) History(request HistoryRequest) (HistoryResult, error) {
	return r.HistoryContext(context.Background(), request)
}

// HistoryContext returns a context-cancelable page of entity history. A zero
// MaxRows or MaxResponseBytes preserves legacy unbounded History behavior.
func (r *Repository) HistoryContext(ctx context.Context, request HistoryRequest) (HistoryResult, error) {
	if err := ctx.Err(); err != nil {
		return HistoryResult{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.ensureOpenLocked(); err != nil {
		return HistoryResult{}, err
	}
	if request.EntityID == "" {
		return HistoryResult{}, ErrEntityHistoryNotFound
	}
	if request.MaxRows < 0 || request.MaxResponseBytes < 0 {
		return HistoryResult{}, ErrInvalidListBudget
	}
	start, err := r.requirePinnedCommitLocked(request.Commit)
	if err != nil {
		return HistoryResult{}, err
	}
	fingerprint := queryFingerprint(struct {
		Commit           ObjectID
		EntityID         string
		AllParents       bool
		MaxRows          int
		MaxResponseBytes int
	}{start, request.EntityID, request.AllParents, request.MaxRows, request.MaxResponseBytes})
	offset, err := decodeContinuation(request.ContinuationToken, fingerprint)
	if err != nil {
		return HistoryResult{}, err
	}
	entries, traversalErr := r.historyEntriesContextLocked(ctx, start, request.EntityID, request.AllParents)
	if traversalErr != nil && !errors.Is(traversalErr, context.DeadlineExceeded) {
		return HistoryResult{}, traversalErr
	}
	if len(entries) == 0 {
		if traversalErr != nil {
			return HistoryResult{}, traversalErr
		}
		return HistoryResult{}, ErrEntityHistoryNotFound
	}
	if offset > len(entries) {
		if traversalErr != nil {
			return HistoryResult{}, traversalErr
		}
		return HistoryResult{}, ErrInvalidContinuation
	}
	timedOut := errors.Is(traversalErr, context.DeadlineExceeded) && len(entries) > offset
	if traversalErr != nil && !timedOut {
		return HistoryResult{}, traversalErr
	}
	result := HistoryResult{Entries: make([]HistoryEntry, 0)}
	if !resultFits(result, request.MaxResponseBytes) {
		return HistoryResult{}, ErrResponseBudgetTooSmall
	}
	limit := len(entries)
	if request.MaxRows > 0 && request.MaxRows < limit {
		limit = request.MaxRows
	}
	next := offset
	pageCtx := ctx
	if timedOut {
		pageCtx = context.Background()
	}
	for next < len(entries) && len(result.Entries) < limit {
		if err := pageCtx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) && next > offset {
				result.ContinuationToken = encodeContinuation(fingerprint, next)
				return result, err
			}
			return HistoryResult{}, err
		}
		candidate := result
		candidate.Entries = append(append([]HistoryEntry(nil), result.Entries...), entries[next])
		candidate.ContinuationToken = encodeContinuation(fingerprint, next+1)
		if !resultFits(candidate, request.MaxResponseBytes) {
			break
		}
		if next+1 == len(entries) {
			candidate.ContinuationToken = ""
		}
		result = candidate
		next++
	}
	if next < len(entries) {
		if next == offset {
			return HistoryResult{}, ErrResponseBudgetTooSmall
		}
		result.ContinuationToken = encodeContinuation(fingerprint, next)
		if !resultFits(result, request.MaxResponseBytes) {
			return HistoryResult{}, ErrResponseBudgetTooSmall
		}
	}
	if timedOut {
		result.ContinuationToken = encodeContinuation(fingerprint, next)
		if !resultFits(result, request.MaxResponseBytes) {
			return HistoryResult{}, ErrResponseBudgetTooSmall
		}
		return result, traversalErr
	}
	return result, nil
}

// BranchesContaining returns ordered branches whose history contains the selected entity or snapshot.
func (r *Repository) BranchesContaining(selector ContainmentSelector) (BranchContainmentResult, error) {
	return r.branchesContainingContext(context.Background(), selector, 0, 0, "")
}

// BranchesContainingContext returns a context-cancelable bounded branch page.
// Its limits are required so callers cannot accidentally expose an unbounded
// public list; legacy callers may continue to use BranchesContaining.
func (r *Repository) BranchesContainingContext(ctx context.Context, request BranchesContainingRequest) (BranchContainmentResult, error) {
	if request.MaxRows <= 0 || request.MaxResponseBytes <= 0 {
		return BranchContainmentResult{}, ErrInvalidListBudget
	}
	return r.branchesContainingContext(ctx, request.Selector, request.MaxRows, request.MaxResponseBytes, request.ContinuationToken)
}

func (r *Repository) branchesContainingContext(ctx context.Context, selector ContainmentSelector, maxRows, maxResponseBytes int, token string) (BranchContainmentResult, error) {
	if err := ctx.Err(); err != nil {
		return BranchContainmentResult{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.ensureOpenLocked(); err != nil {
		return BranchContainmentResult{}, err
	}
	if (selector.EntityID != "") == (selector.SnapshotID != "") && selector.NaturalKey == "" ||
		selector.NaturalKey != "" && (selector.EntityID != "" || selector.SnapshotID != "") {
		return BranchContainmentResult{}, ErrInvalidContainmentSelector
	}
	fingerprint := queryFingerprint(struct {
		Selector         ContainmentSelector
		MaxRows          int
		MaxResponseBytes int
	}{selector, maxRows, maxResponseBytes})
	offset, err := decodeContinuation(token, fingerprint)
	if err != nil {
		return BranchContainmentResult{}, err
	}
	branchNames := make([]string, 0, len(r.branches))
	for branchName := range r.branches {
		if err := ctx.Err(); err != nil {
			return BranchContainmentResult{}, err
		}
		branchNames = append(branchNames, branchName)
	}
	sort.Strings(branchNames)

	branches := make([]string, 0)
	var traversalErr error
	for _, branchName := range branchNames {
		if err := ctx.Err(); err != nil {
			traversalErr = err
			break
		}
		head := r.branches[branchName]
		traversal, err := r.historyTraversalContextLocked(ctx, head, true)
		if err != nil {
			traversalErr = err
			break
		}
		for _, id := range traversal {
			if err := ctx.Err(); err != nil {
				traversalErr = err
				break
			}
			if selector.SnapshotID != "" && r.commits[id].Snapshot == selector.SnapshotID {
				branches = append(branches, branchName)
				break
			}
			if selector.EntityID != "" {
				_, affects, err := r.historyEntryContextLocked(ctx, id, selector.EntityID)
				if err != nil {
					traversalErr = err
					break
				}
				if affects {
					branches = append(branches, branchName)
					break
				}
			}
		}
		if traversalErr != nil {
			break
		}
	}
	if traversalErr != nil && !errors.Is(traversalErr, context.DeadlineExceeded) {
		return BranchContainmentResult{}, traversalErr
	}
	if offset > len(branches) {
		if traversalErr != nil {
			return BranchContainmentResult{}, traversalErr
		}
		return BranchContainmentResult{}, ErrInvalidContinuation
	}
	timedOut := errors.Is(traversalErr, context.DeadlineExceeded) && len(branches) > offset
	if traversalErr != nil && !timedOut {
		return BranchContainmentResult{}, traversalErr
	}
	result := BranchContainmentResult{Branches: make([]string, 0)}
	if !resultFits(result, maxResponseBytes) {
		return BranchContainmentResult{}, ErrResponseBudgetTooSmall
	}
	limit := len(branches)
	if maxRows > 0 && maxRows < limit {
		limit = maxRows
	}
	next := offset
	pageCtx := ctx
	if timedOut {
		pageCtx = context.Background()
	}
	for next < len(branches) && len(result.Branches) < limit {
		if err := pageCtx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) && next > offset {
				result.ContinuationToken = encodeContinuation(fingerprint, next)
				return result, err
			}
			return BranchContainmentResult{}, err
		}
		candidate := result
		candidate.Branches = append(append([]string(nil), result.Branches...), branches[next])
		candidate.ContinuationToken = encodeContinuation(fingerprint, next+1)
		if !resultFits(candidate, maxResponseBytes) {
			break
		}
		if next+1 == len(branches) {
			candidate.ContinuationToken = ""
		}
		result = candidate
		next++
	}
	if next < len(branches) {
		if next == offset {
			return BranchContainmentResult{}, ErrResponseBudgetTooSmall
		}
		result.ContinuationToken = encodeContinuation(fingerprint, next)
		if !resultFits(result, maxResponseBytes) {
			return BranchContainmentResult{}, ErrResponseBudgetTooSmall
		}
	}
	if timedOut {
		result.ContinuationToken = encodeContinuation(fingerprint, next)
		if !resultFits(result, maxResponseBytes) {
			return BranchContainmentResult{}, ErrResponseBudgetTooSmall
		}
		return result, traversalErr
	}
	return result, nil
}

func (r *Repository) historyTraversalLocked(start ObjectID, allParents bool) []ObjectID {
	result, _ := r.historyTraversalContextLocked(context.Background(), start, allParents)
	return result
}

func (r *Repository) historyTraversalContextLocked(ctx context.Context, start ObjectID, allParents bool) ([]ObjectID, error) {
	result, queue, seen := make([]ObjectID, 0), []ObjectID{start}, map[ObjectID]struct{}{}
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		current := queue[0]
		queue = queue[1:]
		if _, ok := seen[current]; ok {
			continue
		}
		seen[current] = struct{}{}
		result = append(result, current)
		parents := r.commits[current].Parents
		if !allParents && len(parents) > 1 {
			parents = parents[:1]
		}
		queue = append(queue, parents...)
	}
	return result, nil
}

func (r *Repository) historyEntriesContextLocked(ctx context.Context, start ObjectID, entityID string, allParents bool) ([]HistoryEntry, error) {
	entries := make([]HistoryEntry, 0)
	queue, seen := []ObjectID{start}, map[ObjectID]struct{}{}
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return entries, err
		}
		current := queue[0]
		queue = queue[1:]
		if _, ok := seen[current]; ok {
			continue
		}
		seen[current] = struct{}{}
		parents := r.commits[current].Parents
		if !allParents && len(parents) > 1 {
			parents = parents[:1]
		}
		queue = append(queue, parents...)

		entry, affects, err := r.historyEntryContextLocked(ctx, current, entityID)
		if err != nil {
			return entries, err
		}
		if affects {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func (r *Repository) historyEntryLocked(id ObjectID, entityID string) (HistoryEntry, bool) {
	entry, affects, _ := r.historyEntryContextLocked(context.Background(), id, entityID)
	return entry, affects
}

func (r *Repository) historyEntryContextLocked(ctx context.Context, id ObjectID, entityID string) (HistoryEntry, bool, error) {
	current := r.commits[id]
	after := r.snapshots[current.Snapshot]
	entry := HistoryEntry{Commit: id, AfterSnapshot: current.Snapshot, Author: current.Author, Time: current.Time, Message: current.Message}
	var before graphSnapshot
	if len(current.Parents) > 0 {
		entry.BeforeSnapshot = r.commits[current.Parents[0]].Snapshot
		before = r.snapshots[entry.BeforeSnapshot]
	}
	beforeNodes, afterNodes := r.projections[before.NodeRoot], r.projections[after.NodeRoot]
	beforeEdges, afterEdges := r.edgeProjections[entry.BeforeSnapshot], r.edgeProjections[current.Snapshot]
	beforeNode, beforeNodeExists := beforeNodes[entityID]
	afterNode, afterNodeExists := afterNodes[entityID]
	_, beforeEdgeExists := beforeEdges[entityID]
	_, afterEdgeExists := afterEdges[entityID]
	affects := beforeNodeExists != afterNodeExists || beforeEdgeExists != afterEdgeExists
	if beforeNodeExists && afterNodeExists {
		entry.ChangedFields = changedNodeFields(beforeNode, afterNode)
		if len(entry.ChangedFields) > 0 {
			affects = true
		}
	}
	for edgeID, edge := range afterEdges {
		if err := ctx.Err(); err != nil {
			return HistoryEntry{}, false, err
		}
		beforeEdge, exists := beforeEdges[edgeID]
		if (!exists || !beforeEdge.Equal(edge)) && (edge.ID == entityID || edge.Source == entityID || edge.Target == entityID || (exists && (beforeEdge.Source == entityID || beforeEdge.Target == entityID))) {
			entry.EdgeAdditions = append(entry.EdgeAdditions, edge.clone())
			affects = true
		}
	}
	for edgeID, edge := range beforeEdges {
		if err := ctx.Err(); err != nil {
			return HistoryEntry{}, false, err
		}
		afterEdge, exists := afterEdges[edgeID]
		if (!exists || !afterEdge.Equal(edge)) && (edge.ID == entityID || edge.Source == entityID || edge.Target == entityID || (exists && (afterEdge.Source == entityID || afterEdge.Target == entityID))) {
			entry.EdgeRemovals = append(entry.EdgeRemovals, edge.clone())
			affects = true
		}
	}
	sort.Slice(entry.EdgeAdditions, func(i, j int) bool { return entry.EdgeAdditions[i].ID < entry.EdgeAdditions[j].ID })
	sort.Slice(entry.EdgeRemovals, func(i, j int) bool { return entry.EdgeRemovals[i].ID < entry.EdgeRemovals[j].ID })
	return entry, affects, nil
}

func changedNodeFields(before, after Node) []string {
	fields := make([]string, 0, 3)
	if before.Title != after.Title {
		fields = append(fields, "title")
	}
	if !reflect.DeepEqual(before.Labels, after.Labels) {
		fields = append(fields, "labels")
	}
	if !reflect.DeepEqual(before.Properties, after.Properties) {
		fields = append(fields, "properties")
	}
	return fields
}
