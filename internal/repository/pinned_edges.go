package repository

import (
	"context"
	"sort"
)

// PinnedEdgesContext returns canonical edge values from a previously pinned
// commit. It is a snapshot read primitive for graph-specific use cases; callers
// are responsible for applying their own traversal bounds.
func (r *Repository) PinnedEdgesContext(ctx context.Context, commitID ObjectID) ([]Edge, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
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
	edges := r.edgeProjections[commit.Snapshot]
	result := make([]Edge, 0, len(edges))
	for _, edge := range edges {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		normalized, err := edge.Normalize()
		if err != nil {
			return nil, err
		}
		result = append(result, normalized)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

// PinnedEdges returns canonical edge values from a previously pinned commit.
func (r *Repository) PinnedEdges(commitID ObjectID) ([]Edge, error) {
	return r.PinnedEdgesContext(context.Background(), commitID)
}
