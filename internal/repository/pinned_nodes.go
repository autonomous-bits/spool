package repository

import (
	"context"
	"sort"
)

// PinnedNodesContext returns canonical node values from a previously pinned
// commit. It is a snapshot read primitive for graph-specific use cases.
func (r *Repository) PinnedNodesContext(ctx context.Context, commitID ObjectID) ([]Node, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := r.ensureOpenLocked(); err != nil {
		return nil, err
	}
	commit, ok := r.commits[commitID]
	if !ok {
		return nil, ErrCommitNotFound
	}
	snapshot, ok := r.snapshots[commit.Snapshot]
	if !ok {
		return nil, ErrCommitNotFound
	}
	if err := r.ensureSnapshotProjectionLocked(commit.Snapshot); err != nil {
		return nil, err
	}
	nodes, ok := r.projections[snapshot.NodeRoot]
	if !ok {
		return nil, ErrProjectionUnavailable
	}
	result := make([]Node, 0, len(nodes))
	for _, node := range nodes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		normalized, err := node.Normalize()
		if err != nil {
			return nil, err
		}
		result = append(result, normalized)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

// PinnedNodes returns canonical node values from a previously pinned commit.
func (r *Repository) PinnedNodes(commitID ObjectID) ([]Node, error) {
	return r.PinnedNodesContext(context.Background(), commitID)
}
