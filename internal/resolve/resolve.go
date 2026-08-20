// Package resolve provides node-resolution queries and their tool adapter.
package resolve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/repository/branch"
	"github.com/autonomous-bits/spool/internal/repository/integrity"
	"github.com/autonomous-bits/spool/internal/repository/merge"
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

// SnapshotMetadata identifies the bound repository snapshot used to resolve a node.
type SnapshotMetadata struct {
	// Repository identifies the repository that resolved the snapshot.
	Repository string `json:"repository"`
	// Branch identifies the requested branch.
	Branch string `json:"branch"`
	// Commit identifies the selected commit.
	Commit string `json:"commit"`
	// Root identifies the selected graph snapshot.
	Root string `json:"root"`
}

// ProjectionMetadata describes the node projection returned by resolution.
type ProjectionMetadata struct {
	// NodeRoot identifies the projection watermark when it matches Snapshot.
	NodeRoot string `json:"nodeRoot"`
	// State describes availability for Snapshot. A nonmatching or absent
	// branch-head projection is unavailable.
	State string `json:"state"`
	// SchemaVersion identifies the projection schema version.
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

// SchemaMetadata identifies the schema used to validate a snapshot.
type SchemaMetadata struct {
	// Root identifies the durable schema object.
	Root string `json:"root"`
	// Version identifies the schema version.
	Version uint16 `json:"version"`
	// Permissive reports whether the schema only enforces graph integrity and
	// global invariants.
	Permissive bool `json:"permissive"`
}

// SchemaValidationResult reports conformance of one immutable snapshot.
type SchemaValidationResult struct {
	// Snapshot identifies the commit and graph snapshot validated.
	Snapshot SnapshotMetadata `json:"snapshot"`
	// Projection describes the projection provenance for Snapshot.
	Projection ProjectionMetadata `json:"projection"`
	// Schema identifies the schema applied to Snapshot.
	Schema SchemaMetadata `json:"schema"`
	// Valid reports whether the snapshot conforms to Schema.
	Valid bool `json:"valid"`
	// Violations contains every failed schema constraint when Valid is false.
	Violations []repository.SchemaViolation `json:"violations"`
}

// DiffResult contains a bounded diff page and provenance for both pinned snapshots.
type DiffResult struct {
	// Base identifies the pinned base snapshot.
	Base SnapshotMetadata `json:"base"`
	// Target identifies the pinned target snapshot.
	Target SnapshotMetadata `json:"target"`
	// Projection describes the target snapshot's projection provenance.
	Projection ProjectionMetadata `json:"projection"`
	// DiffResult retains the bounded diff payload and pagination fields.
	repository.DiffResult
}

// HistoryResult contains entity history and provenance for its pinned start snapshot.
type HistoryResult struct {
	// Snapshot identifies the pinned snapshot from which history was traversed.
	Snapshot SnapshotMetadata `json:"snapshot"`
	// Projection describes the snapshot's projection provenance.
	Projection ProjectionMetadata `json:"projection"`
	// HistoryResult retains the entity history payload.
	repository.HistoryResult
}

// ImpactResult contains impact analysis and provenance for its pinned snapshot.
type ImpactResult struct {
	// Snapshot identifies the pinned snapshot analyzed.
	Snapshot SnapshotMetadata `json:"snapshot"`
	// Projection describes the snapshot's projection provenance.
	Projection ProjectionMetadata `json:"projection"`
	// ImpactResult retains the bounded impact payload.
	repository.ImpactResult
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
	commitID, err := r.resolveSnapshotCommit(ctx, selector)
	if err != nil {
		return ResolveResult{}, err
	}

	resolution, err := r.repo.ResolvePinned(commitID, nodeID)
	if err != nil {
		return ResolveResult{}, err
	}
	return ResolveResult{
		Node:       resolution.Node,
		Snapshot:   r.snapshotMetadata(selector.Branch, resolution.Commit, resolution.Snapshot),
		Projection: r.projectionMetadata(selector.Branch, resolution),
	}, nil
}

// ValidateSchema pins selector's branch and validates that immutable snapshot.
func (r *Resolver) ValidateSchema(ctx context.Context, selector SnapshotSelector) (SchemaValidationResult, error) {
	commitID, err := r.resolveSnapshotCommit(ctx, selector)
	if err != nil {
		return SchemaValidationResult{}, err
	}
	resolution, err := r.repo.ValidatePinnedSchema(commitID)
	if err != nil {
		return SchemaValidationResult{}, err
	}
	return SchemaValidationResult{
		Snapshot:   r.snapshotMetadata(selector.Branch, resolution.Commit, resolution.Snapshot),
		Projection: r.projectionMetadataForCommit(selector.Branch, resolution.Commit),
		Schema: SchemaMetadata{
			Root: string(resolution.SchemaRoot), Version: resolution.Schema.Version, Permissive: resolution.Schema.Permissive,
		},
		Valid: resolution.Valid, Violations: resolution.Violations,
	}, nil
}

func (r *Resolver) snapshotMetadata(branch string, commit, root repository.ObjectID) SnapshotMetadata {
	return SnapshotMetadata{
		Repository: r.repo.RepositoryID(),
		Branch:     branch,
		Commit:     string(commit),
		Root:       string(root),
	}
}

func (r *Resolver) projectionMetadata(branch string, resolution repository.Resolution) ProjectionMetadata {
	return r.projectionMetadataForRoots(branch, resolution.Commit, resolution.NodeRoot)
}

func (r *Resolver) projectionMetadataForCommit(branch string, commit repository.ObjectID) ProjectionMetadata {
	record, err := r.repo.PinnedSnapshotRecord(commit)
	if err != nil {
		return ProjectionMetadata{State: "unavailable"}
	}
	return r.projectionMetadataForRoots(branch, record.Commit, record.NodeRoot)
}

func (r *Resolver) projectionMetadataForRoots(branch string, commit, nodeRoot repository.ObjectID) ProjectionMetadata {
	metadata := ProjectionMetadata{State: "unavailable"}
	status, err := r.repo.ProjectionStatus()
	if err != nil ||
		status.Branch != branch ||
		status.Commit != commit ||
		status.NodeRoot != nodeRoot {
		return metadata
	}
	return ProjectionMetadata{
		NodeRoot:      string(status.NodeRoot),
		State:         status.State,
		SchemaVersion: fmt.Sprintf("v%d", status.SchemaVersion),
	}
}

func (r *Resolver) snapshotMetadataForCommit(branch string, commit repository.ObjectID) (SnapshotMetadata, error) {
	record, err := r.repo.PinnedSnapshotRecord(commit)
	if err != nil {
		return SnapshotMetadata{}, err
	}
	return r.snapshotMetadata(branch, record.Commit, record.Snapshot), nil
}

func (r *Resolver) resolveSnapshotCommit(ctx context.Context, selector SnapshotSelector) (repository.ObjectID, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if selector.Branch == "" {
		return "", ErrMissingBranch
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
			return "", ErrUnsupportedCommit
		}
	} else {
		commitID, err = r.repo.PinBranch(selector.Branch)
	}
	if err != nil {
		return "", err
	}

	if r.afterBranchResolved != nil {
		r.afterBranchResolved()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return commitID, nil
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

// SchemaValidationRequest identifies the snapshot to validate.
type SchemaValidationRequest struct {
	// Selector identifies the branch and optional reachable commit.
	Selector SnapshotSelector `json:"selector"`
}

// DiffRequest combines a repository diff request with optional tool query limits.
type DiffRequest struct {
	// Base identifies the required branch and optional reachable base commit.
	Base SnapshotSelector `json:"base"`
	// Target identifies the required branch and optional reachable target commit.
	Target SnapshotSelector `json:"target"`
	// Filter optionally restricts returned changes.
	Filter repository.DiffFilter `json:"filter,omitempty"`
	// IncludeOneHop requests related unchanged context.
	IncludeOneHop bool `json:"includeOneHop,omitempty"`
	// ContinuationToken resumes a compatible diff request.
	ContinuationToken string `json:"continuationToken,omitempty"`
	// Budget optionally narrows configured query limits.
	Budget QueryBudgetRequest `json:"budget"`
}

// HistoryRequest identifies the branch-constrained history traversal to perform.
type HistoryRequest struct {
	// Selector identifies the required branch and optional reachable starting commit.
	Selector SnapshotSelector `json:"selector"`
	// EntityID identifies the node or edge whose changes are returned.
	EntityID string `json:"entityId"`
	// AllParents includes all parent links rather than only each commit's first parent.
	AllParents bool `json:"allParents,omitempty"`
}

// ContainmentSelector is the repository containment selector accepted by ResolveTool.
type ContainmentSelector = repository.ContainmentSelector

// ImpactRequest combines a repository impact request with optional tool query limits.
type ImpactRequest struct {
	// Selector identifies the required branch and optional reachable commit to analyze.
	Selector SnapshotSelector `json:"selector"`
	// Request contains the hypothetical repository impact operation.
	Request repository.ImpactRequest `json:"request"`
	// Budget optionally narrows configured query limits.
	Budget QueryBudgetRequest `json:"budget"`
}

// MergeApplyRequest identifies a reviewed clean preview and its merge commit metadata.
type MergeApplyRequest struct {
	SourceBranch  string              `json:"sourceBranch"`
	TargetBranch  string              `json:"targetBranch"`
	TransactionID string              `json:"transactionId"`
	PreviewID     repository.ObjectID `json:"previewId"`
	Author        string              `json:"author,omitempty"`
	Message       string              `json:"message,omitempty"`
}

// MergeConflictsRequest identifies an owning conflicted merge transaction.
type MergeConflictsRequest struct {
	TargetBranch  string `json:"targetBranch"`
	TransactionID string `json:"transactionId"`
}

// MergeTransactionRequest identifies an owning conflicted merge transaction.
type MergeTransactionRequest = MergeConflictsRequest

// MergeResolveRequest supplies conflict selections and optional corrective mutations.
type MergeResolveRequest = repository.ResolveConflictedMergeRequest

// ResolveTool adapts resolver, branch, and repository operations to context-aware tool methods.
type ResolveTool struct {
	resolver    *Resolver
	branches    branch.Service
	merges      merge.Service
	queryBudget *QueryBudget
	repository  *repository.Repository
}

// FsckTool exposes repository-integrity checks through the context-aware tool surface.
type FsckTool struct {
	check func(context.Context) (repository.FsckResult, error)
}

// NewFsckTool returns a tool that checks an already-open repository.
func NewFsckTool(repo *repository.Repository) *FsckTool {
	return &FsckTool{check: func(context.Context) (repository.FsckResult, error) {
		return repo.Fsck()
	}}
}

// FsckTool returns an integrity-check tool for the repository backing t.
func (t *ResolveTool) FsckTool() *FsckTool {
	return NewFsckTool(t.repository)
}

// NewPersistentFsckTool returns a tool that can inspect durable state even
// when corruption prevents opening the repository normally.
func NewPersistentFsckTool(stateDir string) *FsckTool {
	service := integrity.NewService(stateDir)
	return &FsckTool{check: func(ctx context.Context) (repository.FsckResult, error) {
		return service.Check(ctx)
	}}
}

// EDGFsck honors cancellation and returns a deterministic integrity report.
func (t *FsckTool) EDGFsck(ctx context.Context) (repository.FsckResult, error) {
	if err := ctx.Err(); err != nil {
		return repository.FsckResult{}, err
	}
	return t.check(ctx)
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
		merges:      merge.NewService(repo),
		queryBudget: options.QueryBudget,
		repository:  repo,
	}
}

// EDGGC honors cancellation before beginning the atomic maintenance operation.
// Once GC starts, it must run to a durable result so cancellation cannot leave
// a caller uncertain whether publication or cleanup occurred.
func (t *ResolveTool) EDGGC(ctx context.Context, options repository.GCOptions) (repository.GCResult, error) {
	if err := ctx.Err(); err != nil {
		return repository.GCResult{}, err
	}
	return t.repository.GC(options)
}

// EDGMergePreview computes a deterministic, non-mutating three-way merge preview.
func (t *ResolveTool) EDGMergePreview(ctx context.Context, sourceBranch, targetBranch string) (repository.MergePreview, error) {
	if err := ctx.Err(); err != nil {
		return repository.MergePreview{}, err
	}
	return t.merges.Preview(sourceBranch, targetBranch)
}

// EDGApplyMergePreview applies an exact clean preview.
func (t *ResolveTool) EDGApplyMergePreview(ctx context.Context, request MergeApplyRequest) (repository.ObjectID, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return t.merges.ApplyPreview(
		request.SourceBranch, request.TargetBranch, request.TransactionID,
		request.PreviewID, request.Author, request.Message,
	)
}

// EDGMergeConflicts returns the durable preview and resolution state for its owner.
func (t *ResolveTool) EDGMergeConflicts(ctx context.Context, request MergeConflictsRequest) (repository.MergeTransactionStatus, error) {
	if err := ctx.Err(); err != nil {
		return repository.MergeTransactionStatus{}, err
	}
	return t.merges.Conflicts(request.TargetBranch, request.TransactionID)
}

// EDGFinalizeMerge commits a fully resolved conflicted merge.
func (t *ResolveTool) EDGFinalizeMerge(ctx context.Context, request MergeTransactionRequest) (repository.ObjectID, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return t.merges.Finalize(request.TargetBranch, request.TransactionID)
}

// EDGResolveMerge persists a complete, schema-valid conflict resolution.
func (t *ResolveTool) EDGResolveMerge(ctx context.Context, request MergeResolveRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return t.merges.ResolveConflicts(request)
}

// EDGAbortMerge durably abandons a conflicted merge and releases its target lease.
func (t *ResolveTool) EDGAbortMerge(ctx context.Context, request MergeTransactionRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return t.merges.Abort(request.TargetBranch, request.TransactionID)
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

// EDGValidateSchema honors cancellation and validates one immutable snapshot.
func (t *ResolveTool) EDGValidateSchema(ctx context.Context, request SchemaValidationRequest) (SchemaValidationResult, error) {
	if err := ctx.Err(); err != nil {
		return SchemaValidationResult{}, err
	}
	return t.resolver.ValidateSchema(ctx, request.Selector)
}

// EDGDiff honors cancellation and returns a budgeted repository diff page.
func (t *ResolveTool) EDGDiff(ctx context.Context, request DiffRequest) (DiffResult, error) {
	if err := ctx.Err(); err != nil {
		return DiffResult{}, err
	}
	base, err := t.resolver.resolveSnapshotCommit(ctx, request.Base)
	if err != nil {
		return DiffResult{}, err
	}
	target, err := t.resolver.resolveSnapshotCommit(ctx, request.Target)
	if err != nil {
		return DiffResult{}, err
	}
	baseSnapshot, err := t.resolver.snapshotMetadataForCommit(request.Base.Branch, base)
	if err != nil {
		return DiffResult{}, err
	}
	targetSnapshot, err := t.resolver.snapshotMetadataForCommit(request.Target.Branch, target)
	if err != nil {
		return DiffResult{}, err
	}
	result := DiffResult{
		Base: baseSnapshot, Target: targetSnapshot,
		Projection: t.resolver.projectionMetadataForCommit(request.Target.Branch, target),
		DiffResult: repository.DiffResult{
			BaseCommit: base, TargetCommit: target, Changes: make([]repository.DiffEntry, 0),
		},
	}
	budget := NormalizeQueryBudget(request.Budget, t.queryBudget)
	payloadBudget, err := diffPayloadBudget(result, budget.MaxResponseBytes)
	if err != nil {
		return DiffResult{}, err
	}
	payload, err := t.resolver.repo.Diff(repository.DiffRequest{
		Base: base, Target: target, Filter: request.Filter,
		MaxRows: budget.MaxRows, MaxResponseBytes: payloadBudget,
		IncludeOneHop: request.IncludeOneHop, ContinuationToken: request.ContinuationToken,
	})
	if err != nil {
		return DiffResult{}, err
	}
	result.DiffResult = payload
	if !diffEnvelopeFits(result, budget.MaxResponseBytes) {
		return DiffResult{}, repository.ErrResponseBudgetTooSmall
	}
	return result, nil
}

// diffPayloadBudget reserves the public envelope's JSON overhead for repository.Diff.
func diffPayloadBudget(result DiffResult, maxBytes int) (int, error) {
	envelope, err := json.Marshal(result)
	if err != nil || len(envelope) > maxBytes {
		return 0, repository.ErrResponseBudgetTooSmall
	}
	payload, err := json.Marshal(result.DiffResult)
	if err != nil {
		return 0, repository.ErrResponseBudgetTooSmall
	}
	return maxBytes - (len(envelope) - len(payload)), nil
}

func diffEnvelopeFits(result DiffResult, maxBytes int) bool {
	data, err := json.Marshal(result)
	return err == nil && len(data) <= maxBytes
}

// EDGHistory honors cancellation and returns repository entity history.
func (t *ResolveTool) EDGHistory(ctx context.Context, request HistoryRequest) (HistoryResult, error) {
	if err := ctx.Err(); err != nil {
		return HistoryResult{}, err
	}
	commit, err := t.resolver.resolveSnapshotCommit(ctx, request.Selector)
	if err != nil {
		return HistoryResult{}, err
	}
	payload, err := t.resolver.repo.History(repository.HistoryRequest{
		Commit: commit, EntityID: request.EntityID, AllParents: request.AllParents,
	})
	if err != nil {
		return HistoryResult{}, err
	}
	snapshot, err := t.resolver.snapshotMetadataForCommit(request.Selector.Branch, commit)
	if err != nil {
		return HistoryResult{}, err
	}
	return HistoryResult{
		Snapshot: snapshot, Projection: t.resolver.projectionMetadataForCommit(request.Selector.Branch, commit),
		HistoryResult: payload,
	}, nil
}

// EDGBranchesContaining honors cancellation and returns matching repository branches.
func (t *ResolveTool) EDGBranchesContaining(ctx context.Context, selector ContainmentSelector) (repository.BranchContainmentResult, error) {
	if err := ctx.Err(); err != nil {
		return repository.BranchContainmentResult{}, err
	}
	return t.resolver.repo.BranchesContaining(selector)
}

// EDGImpact honors cancellation and applies effective budgets to a non-persistent impact query.
func (t *ResolveTool) EDGImpact(ctx context.Context, request ImpactRequest) (ImpactResult, error) {
	if err := ctx.Err(); err != nil {
		return ImpactResult{}, err
	}
	commit, err := t.resolver.resolveSnapshotCommit(ctx, request.Selector)
	if err != nil {
		return ImpactResult{}, err
	}
	budget := NormalizeQueryBudget(request.Budget, t.queryBudget)
	request.Request.MaxDepth = budget.MaxDepth
	request.Request.MaxVisited = budget.MaxVisited
	request.Request.Commit = commit
	payload, err := t.resolver.repo.Impact(request.Request)
	if err != nil {
		return ImpactResult{}, err
	}
	snapshot, err := t.resolver.snapshotMetadataForCommit(request.Selector.Branch, commit)
	if err != nil {
		return ImpactResult{}, err
	}
	return ImpactResult{
		Snapshot: snapshot, Projection: t.resolver.projectionMetadataForCommit(request.Selector.Branch, commit),
		ImpactResult: payload,
	}, nil
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

// EDGStageSchemaMigration honors cancellation and atomically stages a target
// schema with the graph mutations required to conform to it.
func (t *ResolveTool) EDGStageSchemaMigration(ctx context.Context, request repository.SchemaMigrationRequest) (repository.StageMutationResult, error) {
	if err := ctx.Err(); err != nil {
		return repository.StageMutationResult{}, err
	}
	return t.resolver.repo.StageSchemaMigration(request)
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
