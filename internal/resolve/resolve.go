// Package resolve provides node-resolution queries and their tool adapter.
package resolve

import (
	"context"
	"errors"
	"fmt"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/repository/branch"
)

var (
	// ErrMissingBranch reports a resolution selector without a branch.
	ErrMissingBranch = errors.New("branch is required")
	// ErrUnsupportedCommit reports an explicit commit that policy does not permit.
	ErrUnsupportedCommit = errors.New("commit selectors are not supported")
	// ErrBranchNotFound reports an absent repository branch.
	ErrBranchNotFound = repository.ErrBranchNotFound
	// ErrNodeNotFound reports an absent node in the selected snapshot.
	ErrNodeNotFound = repository.ErrNodeNotFound
)

// SnapshotSelector selects a branch and optionally an explicit commit from that branch.
type SnapshotSelector struct {
	// Branch identifies the required branch.
	Branch string `json:"branch"`
	// Commit optionally identifies a commit constrained by resolver policy.
	Commit *string `json:"commit,omitempty"`
}

// Node is the immutable graph node representation returned by repository resolution.
type Node = repository.Node

// SnapshotMetadata identifies the immutable snapshot used to resolve a node.
type SnapshotMetadata struct {
	// Commit identifies the selected commit.
	Commit string `json:"commit"`
	// Root identifies the selected graph snapshot.
	Root string `json:"root"`
}

// ProjectionMetadata describes the node projection returned by resolution.
type ProjectionMetadata struct {
	// NodeRoot identifies the durable root of the node projection.
	NodeRoot string `json:"nodeRoot"`
	// State describes projection availability.
	State string `json:"state"`
	// SchemaVersion identifies the graph schema version.
	SchemaVersion string `json:"schemaVersion"`
}

// Options configures static resolver policy.
type Options struct {
	// AllowDetachedCommit permits explicit commits not reachable from the selected branch.
	AllowDetachedCommit bool
	// QueryBudget provides configured upper bounds for tool queries.
	QueryBudget *QueryBudget
}

// ResolveResult contains a node and metadata for its immutable resolved snapshot.
type ResolveResult struct {
	// Node is the resolved immutable node.
	Node Node `json:"node"`
	// Snapshot identifies the commit and graph snapshot read.
	Snapshot SnapshotMetadata `json:"snapshot"`
	// Projection describes the returned node projection.
	Projection ProjectionMetadata `json:"projection"`
	// Budget is the effective query budget used by the tool adapter.
	Budget QueryBudget `json:"budget"`
}

// Resolver resolves nodes against pinned repository commits.
type Resolver struct {
	repo                *repository.Repository
	allowDetachedCommit bool
	afterBranchResolved func()
}

// NewResolver returns a resolver with default policy.
func NewResolver(repo *repository.Repository) *Resolver {
	return NewResolverWithOptions(repo, Options{})
}

// NewResolverWithOptions returns a resolver using options for commit-selection policy.
func NewResolverWithOptions(repo *repository.Repository, options Options) *Resolver {
	return &Resolver{repo: repo, allowDetachedCommit: options.AllowDetachedCommit}
}

// Resolve pins selector's branch, honors cancellation, and reads nodeID from that immutable commit.
func (r *Resolver) Resolve(ctx context.Context, selector SnapshotSelector, nodeID string) (ResolveResult, error) {
	if err := ctx.Err(); err != nil {
		return ResolveResult{}, err
	}
	if selector.Branch == "" {
		return ResolveResult{}, ErrMissingBranch
	}
	var commitID repository.ObjectID
	var err error
	if selector.Commit != nil {
		commitID, err = r.repo.ResolveExplicitCommit(
			selector.Branch,
			repository.ObjectID(*selector.Commit),
			r.allowDetachedCommit,
		)
		if errors.Is(err, repository.ErrCommitNotReachable) {
			return ResolveResult{}, ErrUnsupportedCommit
		}
	} else {
		commitID, err = r.repo.PinBranch(selector.Branch)
	}
	if err != nil {
		return ResolveResult{}, err
	}

	if r.afterBranchResolved != nil {
		r.afterBranchResolved()
	}
	if err := ctx.Err(); err != nil {
		return ResolveResult{}, err
	}

	resolution, err := r.repo.ResolvePinned(commitID, nodeID)
	if err != nil {
		return ResolveResult{}, err
	}
	return ResolveResult{
		Node: resolution.Node,
		Snapshot: SnapshotMetadata{
			Commit: string(resolution.Commit),
			Root:   string(resolution.Snapshot),
		},
		Projection: ProjectionMetadata{
			NodeRoot:      string(resolution.NodeRoot),
			State:         "ready",
			SchemaVersion: fmt.Sprintf("v%d", resolution.SchemaVersion),
		},
	}, nil
}

// ResolveRequest combines a node selector with optional tool query limits.
type ResolveRequest struct {
	// Selector identifies the snapshot to resolve.
	Selector SnapshotSelector `json:"selector"`
	// NodeID identifies the requested node.
	NodeID string `json:"nodeId"`
	// Budget optionally narrows configured query limits.
	Budget QueryBudgetRequest `json:"budget"`
}

// DiffRequest combines a repository diff request with optional tool query limits.
type DiffRequest struct {
	// Base identifies the base snapshot.
	Base repository.DiffSelector `json:"base"`
	// Target identifies the target snapshot.
	Target repository.DiffSelector `json:"target"`
	// Filter optionally restricts returned changes.
	Filter repository.DiffFilter `json:"filter,omitempty"`
	// IncludeOneHop requests related unchanged context.
	IncludeOneHop bool `json:"includeOneHop,omitempty"`
	// ContinuationToken resumes a compatible diff request.
	ContinuationToken string `json:"continuationToken,omitempty"`
	// Budget optionally narrows configured query limits.
	Budget QueryBudgetRequest `json:"budget"`
}

// HistoryRequest is the repository history request accepted by ResolveTool.
type HistoryRequest = repository.HistoryRequest

// ContainmentSelector is the repository containment selector accepted by ResolveTool.
type ContainmentSelector = repository.ContainmentSelector

// ImpactRequest combines a repository impact request with optional tool query limits.
type ImpactRequest struct {
	// Request contains the hypothetical repository impact operation.
	Request repository.ImpactRequest `json:"request"`
	// Budget optionally narrows configured query limits.
	Budget QueryBudgetRequest `json:"budget"`
}

// ResolveTool adapts resolver, branch, and repository operations to context-aware tool methods.
type ResolveTool struct {
	resolver    *Resolver
	branches    branch.Service
	queryBudget *QueryBudget
}

// NewResolveTool returns a tool adapter with default policy and budgets.
func NewResolveTool(repo *repository.Repository) *ResolveTool {
	return NewResolveToolWithOptions(repo, Options{})
}

// NewResolveToolWithOptions returns a tool adapter configured with options.
func NewResolveToolWithOptions(repo *repository.Repository, options Options) *ResolveTool {
	return &ResolveTool{
		resolver:    NewResolverWithOptions(repo, options),
		branches:    branch.NewService(repo),
		queryBudget: options.QueryBudget,
	}
}

// EDGResolve honors cancellation, resolves a node, and reports the effective budget.
func (t *ResolveTool) EDGResolve(ctx context.Context, request ResolveRequest) (ResolveResult, error) {
	if err := ctx.Err(); err != nil {
		return ResolveResult{}, err
	}
	result, err := t.resolver.Resolve(ctx, request.Selector, request.NodeID)
	if err != nil {
		return ResolveResult{}, err
	}
	result.Budget = NormalizeQueryBudget(request.Budget, t.queryBudget)
	return result, nil
}

// EDGDiff honors cancellation and returns a budgeted repository diff page.
func (t *ResolveTool) EDGDiff(ctx context.Context, request DiffRequest) (repository.DiffResult, error) {
	if err := ctx.Err(); err != nil {
		return repository.DiffResult{}, err
	}
	budget := NormalizeQueryBudget(request.Budget, t.queryBudget)
	return t.resolver.repo.Diff(repository.DiffRequest{
		Base: request.Base, Target: request.Target, Filter: request.Filter,
		MaxRows: budget.MaxRows, MaxResponseBytes: budget.MaxResponseBytes,
		IncludeOneHop: request.IncludeOneHop, ContinuationToken: request.ContinuationToken,
	})
}

// EDGHistory honors cancellation and returns repository entity history.
func (t *ResolveTool) EDGHistory(ctx context.Context, request HistoryRequest) (repository.HistoryResult, error) {
	if err := ctx.Err(); err != nil {
		return repository.HistoryResult{}, err
	}
	return t.resolver.repo.History(request)
}

// EDGBranchesContaining honors cancellation and returns matching repository branches.
func (t *ResolveTool) EDGBranchesContaining(ctx context.Context, selector ContainmentSelector) (repository.BranchContainmentResult, error) {
	if err := ctx.Err(); err != nil {
		return repository.BranchContainmentResult{}, err
	}
	return t.resolver.repo.BranchesContaining(selector)
}

// EDGImpact honors cancellation and applies effective budgets to a non-persistent impact query.
func (t *ResolveTool) EDGImpact(ctx context.Context, request ImpactRequest) (repository.ImpactResult, error) {
	if err := ctx.Err(); err != nil {
		return repository.ImpactResult{}, err
	}
	budget := NormalizeQueryBudget(request.Budget, t.queryBudget)
	request.Request.MaxDepth = budget.MaxDepth
	request.Request.MaxVisited = budget.MaxVisited
	return t.resolver.repo.Impact(request.Request)
}

// EDGCreateBranch delegates a context-aware branch creation request.
func (t *ResolveTool) EDGCreateBranch(ctx context.Context, request branch.CreateRequest) (branch.CreateResult, error) {
	return t.branches.Create(ctx, request)
}

// EDGListBranches delegates a context-aware branch listing request.
func (t *ResolveTool) EDGListBranches(ctx context.Context) (branch.ListResult, error) {
	return t.branches.List(ctx)
}

// EDGDeleteBranch delegates a context-aware branch deletion request.
func (t *ResolveTool) EDGDeleteBranch(ctx context.Context, request branch.DeleteRequest) (branch.DeleteResult, error) {
	return t.branches.Delete(ctx, request)
}

// EDGSwitchBranch delegates a context-aware branch switch request.
func (t *ResolveTool) EDGSwitchBranch(ctx context.Context, request branch.SwitchRequest) (branch.SwitchResult, error) {
	return t.branches.Switch(ctx, request)
}

// EDGStageMutationBatch honors cancellation and replaces a branch's shared staged mutations.
func (t *ResolveTool) EDGStageMutationBatch(ctx context.Context, request repository.StageMutationRequest) (repository.StageMutationResult, error) {
	if err := ctx.Err(); err != nil {
		return repository.StageMutationResult{}, err
	}
	return t.resolver.repo.StageMutationBatch(request)
}

// EDGBranchStagingStatus honors cancellation and returns a branch's shared staging summary.
func (t *ResolveTool) EDGBranchStagingStatus(ctx context.Context, branch string) (repository.BranchStagingStatus, error) {
	if err := ctx.Err(); err != nil {
		return repository.BranchStagingStatus{}, err
	}
	return t.resolver.repo.BranchStagingStatus(branch)
}

// EDGCommitStagedMutations honors cancellation and commits a branch's staged mutations.
func (t *ResolveTool) EDGCommitStagedMutations(ctx context.Context, branch string) (repository.CommitStagedMutationResult, error) {
	if err := ctx.Err(); err != nil {
		return repository.CommitStagedMutationResult{}, err
	}
	return t.resolver.repo.CommitStagedMutations(branch)
}

// EDGCommitStagedMutationBatch honors cancellation and commits staged mutations with metadata.
func (t *ResolveTool) EDGCommitStagedMutationBatch(ctx context.Context, request repository.CommitStagedMutationRequest) (repository.CommitStagedMutationResult, error) {
	if err := ctx.Err(); err != nil {
		return repository.CommitStagedMutationResult{}, err
	}
	return t.resolver.repo.CommitStagedMutationBatch(request)
}
