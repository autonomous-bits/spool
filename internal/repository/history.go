package repository

import (
	"errors"
	"sort"
	"time"
)

var (
	// ErrInvalidHistorySelector reports a selector that names both or neither branch and commit.
	ErrInvalidHistorySelector = errors.New("history selector must identify exactly one branch or commit")
	// ErrInvalidContainmentSelector reports a containment selector with an invalid combination of keys.
	ErrInvalidContainmentSelector = errors.New("containment selector must identify exactly one entity ID, snapshot, or natural key")
	// ErrEntityHistoryNotFound reports an empty entity identifier or no matching history.
	ErrEntityHistoryNotFound = errors.New("entity has no history")
)

// HistoryRequest selects an entity's commit history from a branch or commit.
type HistoryRequest struct {
	// Selector identifies the traversal starting commit.
	Selector DiffSelector `json:"selector"`
	// EntityID identifies the node or edge whose changes are returned.
	EntityID string `json:"entityId"`
	// AllParents includes all parent links rather than only each commit's first parent.
	AllParents bool `json:"allParents,omitempty"`
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
}

// History returns commits that affected the selected entity, or ErrEntityHistoryNotFound.
func (r *Repository) History(request HistoryRequest) (HistoryResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.ensureOpenLocked(); err != nil {
		return HistoryResult{}, err
	}
	if request.EntityID == "" {
		return HistoryResult{}, ErrEntityHistoryNotFound
	}
	start, err := r.resolveHistorySelectorLocked(request.Selector)
	if err != nil {
		return HistoryResult{}, err
	}
	entries := make([]HistoryEntry, 0)
	for _, id := range r.historyTraversalLocked(start, request.AllParents) {
		entry, affects := r.historyEntryLocked(id, request.EntityID)
		if affects {
			entries = append(entries, entry)
		}
	}
	if len(entries) == 0 {
		return HistoryResult{}, ErrEntityHistoryNotFound
	}
	return HistoryResult{Entries: entries}, nil
}

// BranchesContaining returns ordered branches whose history contains the selected entity or snapshot.
func (r *Repository) BranchesContaining(selector ContainmentSelector) (BranchContainmentResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.ensureOpenLocked(); err != nil {
		return BranchContainmentResult{}, err
	}
	if (selector.EntityID != "") == (selector.SnapshotID != "") && selector.NaturalKey == "" ||
		selector.NaturalKey != "" && (selector.EntityID != "" || selector.SnapshotID != "") {
		return BranchContainmentResult{}, ErrInvalidContainmentSelector
	}
	branches := make([]string, 0)
	for branchName, head := range r.branches {
		for _, id := range r.historyTraversalLocked(head, true) {
			if selector.SnapshotID != "" && r.commits[id].Snapshot == selector.SnapshotID {
				branches = append(branches, branchName)
				break
			}
			if selector.EntityID != "" {
				_, affects := r.historyEntryLocked(id, selector.EntityID)
				if affects {
					branches = append(branches, branchName)
					break
				}
			}
		}
	}
	sort.Strings(branches)
	return BranchContainmentResult{Branches: branches}, nil
}

func (r *Repository) resolveHistorySelectorLocked(selector DiffSelector) (ObjectID, error) {
	if (selector.Branch == "") == (selector.Commit == "") {
		return "", ErrInvalidHistorySelector
	}
	if selector.Branch != "" {
		commit, ok := r.branches[selector.Branch]
		if !ok {
			return "", ErrBranchNotFound
		}
		return commit, nil
	}
	commit := ObjectID(selector.Commit)
	if _, ok := r.commits[commit]; !ok {
		return "", ErrCommitNotFound
	}
	return commit, nil
}

func (r *Repository) historyTraversalLocked(start ObjectID, allParents bool) []ObjectID {
	result, queue, seen := make([]ObjectID, 0), []ObjectID{start}, map[ObjectID]struct{}{}
	for len(queue) > 0 {
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
	return result
}

func (r *Repository) historyEntryLocked(id ObjectID, entityID string) (HistoryEntry, bool) {
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
	if beforeNodeExists && afterNodeExists && beforeNode.Title != afterNode.Title {
		entry.ChangedFields = []string{"title"}
		affects = true
	}
	for edgeID, edge := range afterEdges {
		beforeEdge, exists := beforeEdges[edgeID]
		if (!exists || beforeEdge != edge) && (edge.ID == entityID || edge.Source == entityID || edge.Target == entityID || (exists && (beforeEdge.Source == entityID || beforeEdge.Target == entityID))) {
			entry.EdgeAdditions = append(entry.EdgeAdditions, edge)
			affects = true
		}
	}
	for edgeID, edge := range beforeEdges {
		afterEdge, exists := afterEdges[edgeID]
		if (!exists || afterEdge != edge) && (edge.ID == entityID || edge.Source == entityID || edge.Target == entityID || (exists && (afterEdge.Source == entityID || afterEdge.Target == entityID))) {
			entry.EdgeRemovals = append(entry.EdgeRemovals, edge)
			affects = true
		}
	}
	sort.Slice(entry.EdgeAdditions, func(i, j int) bool { return entry.EdgeAdditions[i].ID < entry.EdgeAdditions[j].ID })
	sort.Slice(entry.EdgeRemovals, func(i, j int) bool { return entry.EdgeRemovals[i].ID < entry.EdgeRemovals[j].ID })
	return entry, affects
}
