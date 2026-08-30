// Package resolve provides node-resolution queries and their tool adapter.
package resolve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/repository/integrity"
)

var (
	// ErrMissingBranch reports a resolution selector without a branch.
	ErrMissingBranch = repository.ErrBranchRequired
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
	// Completion reports query completion and full-envelope byte accounting.
	Completion QueryCompletionMetadata `json:"completion"`
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
	// Budget is the effective query budget used by the tool adapter.
	Budget QueryBudget `json:"budget"`
	// Completion reports query completion and full-envelope byte accounting.
	Completion QueryCompletionMetadata `json:"completion"`
	// DiffResult retains the bounded diff payload and pagination fields.
	repository.DiffResult
}

// HistoryResult contains entity history and provenance for its pinned start snapshot.
type HistoryResult struct {
	// Snapshot identifies the pinned snapshot from which history was traversed.
	Snapshot SnapshotMetadata `json:"snapshot"`
	// Projection describes the snapshot's projection provenance.
	Projection ProjectionMetadata `json:"projection"`
	// Budget is the effective query budget used by the tool adapter.
	Budget QueryBudget `json:"budget"`
	// Completion reports query completion and full-envelope byte accounting.
	Completion QueryCompletionMetadata `json:"completion"`
	// HistoryResult retains the entity history payload.
	repository.HistoryResult
}

// ImpactResult contains impact analysis and provenance for its pinned snapshot.
type ImpactResult struct {
	// Snapshot identifies the pinned snapshot analyzed.
	Snapshot SnapshotMetadata `json:"snapshot"`
	// Projection describes the snapshot's projection provenance.
	Projection ProjectionMetadata `json:"projection"`
	// Budget is the effective query budget used by the tool adapter.
	Budget QueryBudget `json:"budget"`
	// Completion reports query completion and full-envelope byte accounting.
	Completion QueryCompletionMetadata `json:"completion"`
	// ImpactResult retains the bounded impact payload.
	repository.ImpactResult
}

// BranchesContainingResult contains a bounded containment page and query metadata.
type BranchesContainingResult struct {
	// Budget is the effective query budget used by the tool adapter.
	Budget QueryBudget `json:"budget"`
	// Completion reports query completion and full-envelope byte accounting.
	Completion QueryCompletionMetadata `json:"completion"`
	// BranchContainmentResult retains the containment page and continuation token.
	repository.BranchContainmentResult
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

	resolution, err := r.repo.ResolvePinnedContext(ctx, commitID, nodeID)
	if err != nil {
		return ResolveResult{}, err
	}
	return ResolveResult{
		Node:       resolution.Node,
		Snapshot:   r.snapshotMetadata(selector.Branch, resolution.Commit, resolution.Snapshot),
		Projection: r.projectionMetadataContext(ctx, selector.Branch, resolution),
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
		Projection: r.projectionMetadataForCommitContext(ctx, selector.Branch, resolution.Commit),
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

func (r *Resolver) projectionMetadataContext(ctx context.Context, branch string, resolution repository.Resolution) ProjectionMetadata {
	return r.projectionMetadataForRootsContext(ctx, branch, resolution.Commit, resolution.NodeRoot)
}

func (r *Resolver) projectionMetadataForCommitContext(ctx context.Context, branch string, commit repository.ObjectID) ProjectionMetadata {
	if err := ctx.Err(); err != nil {
		return ProjectionMetadata{State: "unavailable"}
	}
	record, err := r.repo.PinnedSnapshotRecordContext(ctx, commit)
	if err != nil {
		return ProjectionMetadata{State: "unavailable"}
	}
	return r.projectionMetadataForRootsContext(ctx, branch, record.Commit, record.NodeRoot)
}

func (r *Resolver) projectionMetadataForRootsContext(ctx context.Context, branch string, commit, nodeRoot repository.ObjectID) ProjectionMetadata {
	metadata := ProjectionMetadata{State: "unavailable"}
	status, err := r.repo.ProjectionStatusContext(ctx)
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

func (r *Resolver) snapshotMetadataForCommitContext(ctx context.Context, branch string, commit repository.ObjectID) (SnapshotMetadata, error) {
	record, err := r.repo.PinnedSnapshotRecordContext(ctx, commit)
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
		commitID, err = r.repo.ResolveExplicitCommitContext(
			ctx,
			selector.Branch,
			repository.ObjectID(*selector.Commit),
			r.allowDetachedCommit,
		)
		if errors.Is(err, repository.ErrCommitNotReachable) {
			return "", ErrUnsupportedCommit
		}
	} else {
		commitID, err = r.repo.PinBranchContext(ctx, selector.Branch)
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
	// ContinuationToken resumes a compatible history request.
	ContinuationToken string `json:"continuationToken,omitempty"`
	// Budget optionally narrows configured query limits.
	Budget QueryBudgetRequest `json:"budget"`
}

// ContainmentSelector is the repository containment selector accepted by ResolveTool.
type ContainmentSelector = repository.ContainmentSelector

// BranchesContainingRequest describes a bounded branch-containment page.
type BranchesContainingRequest struct {
	// Selector identifies the entity or snapshot to find.
	Selector ContainmentSelector `json:"selector"`
	// ContinuationToken resumes a compatible containment request.
	ContinuationToken string `json:"continuationToken,omitempty"`
	// Budget optionally narrows configured query limits.
	Budget QueryBudgetRequest `json:"budget"`
}

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

// ResolveTool adapts snapshot queries to context-aware tool methods.
type ResolveTool struct {
	resolver    *Resolver
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

// SPLFsck honors cancellation and returns a deterministic integrity report.
func (t *FsckTool) SPLFsck(ctx context.Context) (repository.FsckResult, error) {
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
		queryBudget: options.QueryBudget,
		repository:  repo,
	}
}

// SPLResolve resolves one node within the effective query deadline.
func (t *ResolveTool) SPLResolve(ctx context.Context, request ResolveRequest) (ResolveResult, error) {
	budget := NormalizeQueryBudget(request.Budget, t.queryBudget)
	queryCtx, execution, cancel := BeginQuery(ctx, budget)
	defer cancel()
	if err := queryCtx.Err(); err != nil {
		return ResolveResult{}, err
	}
	result, err := t.resolver.Resolve(queryCtx, request.Selector, request.NodeID)
	if err != nil {
		return ResolveResult{}, err
	}
	if err := queryCtx.Err(); err != nil {
		return ResolveResult{}, err
	}
	result.Budget = budget
	if err := finalizeToolQuery(&result, &result.Completion, execution, budget.MaxResponseBytes, false, false, 1); err != nil {
		return ResolveResult{}, err
	}
	return result, nil
}

// SPLValidateSchema honors cancellation and validates one immutable snapshot.
func (t *ResolveTool) SPLValidateSchema(ctx context.Context, request SchemaValidationRequest) (SchemaValidationResult, error) {
	if err := ctx.Err(); err != nil {
		return SchemaValidationResult{}, err
	}
	return t.resolver.ValidateSchema(ctx, request.Selector)
}

// SPLDiff returns a deadline-bounded repository diff page.
func (t *ResolveTool) SPLDiff(ctx context.Context, request DiffRequest) (DiffResult, error) {
	budget := NormalizeQueryBudget(request.Budget, t.queryBudget)
	queryCtx, execution, cancel := BeginQuery(ctx, budget)
	defer cancel()
	if err := queryCtx.Err(); err != nil {
		return DiffResult{}, err
	}
	base, err := t.resolver.resolveSnapshotCommit(queryCtx, request.Base)
	if err != nil {
		return DiffResult{}, err
	}
	target, err := t.resolver.resolveSnapshotCommit(queryCtx, request.Target)
	if err != nil {
		return DiffResult{}, err
	}
	baseSnapshot, err := t.resolver.snapshotMetadataForCommitContext(queryCtx, request.Base.Branch, base)
	if err != nil {
		return DiffResult{}, err
	}
	targetSnapshot, err := t.resolver.snapshotMetadataForCommitContext(queryCtx, request.Target.Branch, target)
	if err != nil {
		return DiffResult{}, err
	}
	result := DiffResult{
		Base: baseSnapshot, Target: targetSnapshot,
		Projection: t.resolver.projectionMetadataForCommitContext(queryCtx, request.Target.Branch, target),
		Budget:     budget,
		Completion: queryCompletionTemplate(budget),
		DiffResult: repository.DiffResult{
			BaseCommit: base, TargetCommit: target, Changes: make([]repository.DiffEntry, 0),
		},
	}
	payloadBudget, err := publicPayloadBudget(result, result.DiffResult, budget.MaxResponseBytes)
	if err != nil {
		return DiffResult{}, err
	}
	payload, err := t.resolver.repo.DiffContext(queryCtx, repository.DiffRequest{
		Base: base, Target: target, Filter: request.Filter,
		MaxRows: budget.MaxRows, MaxResponseBytes: payloadBudget,
		IncludeOneHop: request.IncludeOneHop, ContinuationToken: request.ContinuationToken,
	})
	timedOut := isTimedOutPrefix(err, len(payload.Changes) > 0)
	if err != nil && !timedOut {
		return DiffResult{}, err
	}
	result.DiffResult = payload
	truncated := payload.ContinuationToken != "" || payload.ContextTruncated || timedOut
	if err := finalizeToolQuery(
		&result, &result.Completion, execution, budget.MaxResponseBytes,
		truncated, timedOut, len(payload.Changes)+len(payload.Context),
	); err != nil {
		return DiffResult{}, err
	}
	return result, nil
}

func queryCompletionTemplate(budget QueryBudget) QueryCompletionMetadata {
	elapsedMs := budget.Timeout.Milliseconds()
	if elapsedMs < 0 {
		elapsedMs = 0
	}
	return QueryCompletionMetadata{
		Complete:      false,
		Truncated:     false,
		TimedOut:      false,
		Visited:       budget.MaxRows,
		ElapsedMs:     elapsedMs,
		ResponseBytes: budget.MaxResponseBytes,
	}
}

// publicPayloadBudget reserves public-envelope metadata before passing the
// remaining byte capacity to a repository page API.
func publicPayloadBudget(envelope, payload any, maxBytes int) (int, error) {
	if maxBytes <= 0 {
		return 0, repository.ErrResponseBudgetTooSmall
	}
	encodedEnvelope, err := json.Marshal(envelope)
	if err != nil || len(encodedEnvelope) > maxBytes {
		return 0, repository.ErrResponseBudgetTooSmall
	}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return 0, repository.ErrResponseBudgetTooSmall
	}
	overhead := len(encodedEnvelope) - len(encodedPayload)
	if overhead >= maxBytes {
		return 0, repository.ErrResponseBudgetTooSmall
	}
	return maxBytes - overhead, nil
}

func isTimedOutPrefix(err error, hasPrefix bool) bool {
	return hasPrefix && errors.Is(err, context.DeadlineExceeded)
}

func finalizeToolQuery(
	envelope any,
	completion *QueryCompletionMetadata,
	execution QueryExecutionMetadata,
	maxResponseBytes int,
	truncated, timedOut bool,
	visited int,
) error {
	final := CompleteQuery(execution, time.Now())
	final.Truncated = truncated
	final.TimedOut = timedOut
	final.Complete = !truncated && !timedOut
	final.Visited = visited
	*completion = final
	if _, err := FinalizeQueryResponse(envelope, completion, maxResponseBytes); err != nil {
		if errors.Is(err, ErrResponseBudgetExceeded) {
			return repository.ErrResponseBudgetTooSmall
		}
		return err
	}
	return nil
}

// SPLHistory returns a deadline-bounded repository entity-history page.
func (t *ResolveTool) SPLHistory(ctx context.Context, request HistoryRequest) (HistoryResult, error) {
	budget := NormalizeQueryBudget(request.Budget, t.queryBudget)
	queryCtx, execution, cancel := BeginQuery(ctx, budget)
	defer cancel()
	if err := queryCtx.Err(); err != nil {
		return HistoryResult{}, err
	}
	commit, err := t.resolver.resolveSnapshotCommit(queryCtx, request.Selector)
	if err != nil {
		return HistoryResult{}, err
	}
	snapshot, err := t.resolver.snapshotMetadataForCommitContext(queryCtx, request.Selector.Branch, commit)
	if err != nil {
		return HistoryResult{}, err
	}
	result := HistoryResult{
		Snapshot:   snapshot,
		Projection: t.resolver.projectionMetadataForCommitContext(queryCtx, request.Selector.Branch, commit),
		Budget:     budget,
		Completion: queryCompletionTemplate(budget),
		HistoryResult: repository.HistoryResult{
			Entries: make([]repository.HistoryEntry, 0),
		},
	}
	payloadBudget, err := publicPayloadBudget(result, result.HistoryResult, budget.MaxResponseBytes)
	if err != nil {
		return HistoryResult{}, err
	}
	payload, err := t.resolver.repo.HistoryContext(queryCtx, repository.HistoryRequest{
		Commit: commit, EntityID: request.EntityID, AllParents: request.AllParents,
		MaxRows: budget.MaxRows, MaxResponseBytes: payloadBudget,
		ContinuationToken: request.ContinuationToken,
	})
	timedOut := isTimedOutPrefix(err, len(payload.Entries) > 0)
	if err != nil && !timedOut {
		return HistoryResult{}, err
	}
	result.HistoryResult = payload
	truncated := payload.ContinuationToken != "" || timedOut
	if err := finalizeToolQuery(
		&result, &result.Completion, execution, budget.MaxResponseBytes,
		truncated, timedOut, len(payload.Entries),
	); err != nil {
		return HistoryResult{}, err
	}
	return result, nil
}

// SPLBranchesContaining returns the first bounded containment page for selector.
// SPLBranchesContainingPage accepts continuation and narrowed-budget controls.
func (t *ResolveTool) SPLBranchesContaining(ctx context.Context, selector ContainmentSelector) (BranchesContainingResult, error) {
	return t.SPLBranchesContainingPage(ctx, BranchesContainingRequest{Selector: selector})
}

// SPLBranchesContainingPage returns a deadline-bounded containment page.
func (t *ResolveTool) SPLBranchesContainingPage(ctx context.Context, request BranchesContainingRequest) (BranchesContainingResult, error) {
	budget := NormalizeQueryBudget(request.Budget, t.queryBudget)
	queryCtx, execution, cancel := BeginQuery(ctx, budget)
	defer cancel()
	if err := queryCtx.Err(); err != nil {
		return BranchesContainingResult{}, err
	}
	result := BranchesContainingResult{
		Budget:     budget,
		Completion: queryCompletionTemplate(budget),
		BranchContainmentResult: repository.BranchContainmentResult{
			Branches: make([]string, 0),
		},
	}
	payloadBudget, err := publicPayloadBudget(result, result.BranchContainmentResult, budget.MaxResponseBytes)
	if err != nil {
		return BranchesContainingResult{}, err
	}
	payload, err := t.resolver.repo.BranchesContainingContext(queryCtx, repository.BranchesContainingRequest{
		Selector: request.Selector, MaxRows: budget.MaxRows, MaxResponseBytes: payloadBudget,
		ContinuationToken: request.ContinuationToken,
	})
	timedOut := isTimedOutPrefix(err, len(payload.Branches) > 0)
	if err != nil && !timedOut {
		return BranchesContainingResult{}, err
	}
	result.BranchContainmentResult = payload
	truncated := payload.ContinuationToken != "" || timedOut
	if err := finalizeToolQuery(
		&result, &result.Completion, execution, budget.MaxResponseBytes,
		truncated, timedOut, len(payload.Branches),
	); err != nil {
		return BranchesContainingResult{}, err
	}
	return result, nil
}

// SPLImpact returns a deadline-bounded non-persistent impact page.
func (t *ResolveTool) SPLImpact(ctx context.Context, request ImpactRequest) (ImpactResult, error) {
	budget := NormalizeQueryBudget(request.Budget, t.queryBudget)
	queryCtx, execution, cancel := BeginQuery(ctx, budget)
	defer cancel()
	if err := queryCtx.Err(); err != nil {
		return ImpactResult{}, err
	}
	commit, err := t.resolver.resolveSnapshotCommit(queryCtx, request.Selector)
	if err != nil {
		return ImpactResult{}, err
	}
	snapshot, err := t.resolver.snapshotMetadataForCommitContext(queryCtx, request.Selector.Branch, commit)
	if err != nil {
		return ImpactResult{}, err
	}
	result := ImpactResult{
		Snapshot:   snapshot,
		Projection: t.resolver.projectionMetadataForCommitContext(queryCtx, request.Selector.Branch, commit),
		Budget:     budget,
		Completion: queryCompletionTemplate(budget),
		ImpactResult: repository.ImpactResult{
			Commit: commit, Snapshot: repository.ObjectID(snapshot.Root), Impacts: make([]repository.ImpactEntry, 0),
		},
	}
	payloadBudget, err := publicPayloadBudget(result, result.ImpactResult, budget.MaxResponseBytes)
	if err != nil {
		return ImpactResult{}, err
	}
	request.Request.MaxDepth = budget.MaxDepth
	request.Request.MaxVisited = budget.MaxVisited
	request.Request.MaxRows = budget.MaxRows
	request.Request.MaxResponseBytes = payloadBudget
	request.Request.Commit = commit
	payload, err := t.resolver.repo.ImpactContext(queryCtx, request.Request)
	timedOut := isTimedOutPrefix(err, len(payload.Impacts) > 0)
	if err != nil && !timedOut {
		return ImpactResult{}, err
	}
	result.ImpactResult = payload
	truncated := payload.ContinuationToken != "" || payload.CapacityExhausted || timedOut
	if err := finalizeToolQuery(
		&result, &result.Completion, execution, budget.MaxResponseBytes,
		truncated, timedOut, len(payload.Impacts),
	); err != nil {
		return ImpactResult{}, err
	}
	return result, nil
}
