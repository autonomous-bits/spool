// Package prune defines graph pruning operations for excising ephemeral planning entities.
package prune

import (
	"context"
	"errors"

	"github.com/autonomous-bits/spool/internal/repository/branch"
)

var (
	// ErrBranchRequired reports a prune request without a branch.
	ErrBranchRequired = branch.ErrRequired
	// ErrBranchNotFound reports a requested branch that does not exist.
	ErrBranchNotFound = branch.ErrNotFound
	// ErrProtectedBranch reports an attempt to prune a protected default branch without force override.
	ErrProtectedBranch = errors.New("cannot prune protected branch without force")
	// ErrUncommittedStagedChanges reports an attempt to prune a branch with uncommitted staged mutations.
	ErrUncommittedStagedChanges = errors.New("branch has uncommitted staged changes")
)

// Request describes the options for a graph pruning operation.
type Request struct {
	// Branch identifies the branch whose ephemeral entities will be pruned.
	Branch string `json:"branch"`
	// DryRun reports whether to simulate pruning without writing commits or advancing branch refs.
	DryRun bool `json:"dryRun,omitempty"`
	// Force permits pruning on protected default branches.
	Force bool `json:"force,omitempty"`
	// Author optionally overrides the commit author for non-dry-run executions.
	Author string `json:"author,omitempty"`
	// Message optionally overrides the commit message for non-dry-run executions.
	Message string `json:"message,omitempty"`
}

// Result summarizes the entities removed, cascading edges excised, and durable orphans detected.
type Result struct {
	// Branch identifies the pruned branch.
	Branch string `json:"branch"`
	// Commit identifies the newly created pruning commit, or the unchanged head commit when dry-run/no-op.
	Commit string `json:"commit,omitempty"`
	// DryRun indicates whether this result was produced by a preview simulation.
	DryRun bool `json:"dryRun,omitempty"`
	// PrunedNodesCount is the number of ephemeral nodes removed.
	PrunedNodesCount int `json:"prunedNodesCount"`
	// PrunedEdgesCount is the number of cascading incident edges removed.
	PrunedEdgesCount int `json:"prunedEdgesCount"`
	// PrunedNodeIDs lists the identifiers of all ephemeral nodes excised.
	PrunedNodeIDs []string `json:"prunedNodeIds"`
	// OrphanedDurableNodes lists the identifiers of durable nodes that lost all connected edges.
	OrphanedDurableNodes []string `json:"orphanedDurableNodes"`
}

// Store provides the repository-level pruning persistence operations.
type Store interface {
	// Prune performs ephemeral entity discovery, cascading edge excision, orphan detection, and commit generation.
	Prune(request Request) (Result, error)
}

// Service validates prune requests and delegates execution to Store.
type Service struct {
	store Store
}

// NewService returns a pruning service backed by store.
func NewService(store Store) Service {
	return Service{store: store}
}

// Prune validates the request, honors context cancellation, and executes pruning through Store.
func (s Service) Prune(ctx context.Context, request Request) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if request.Branch == "" {
		return Result{}, ErrBranchRequired
	}
	return s.store.Prune(request)
}
