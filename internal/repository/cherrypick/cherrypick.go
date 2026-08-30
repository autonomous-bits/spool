// Package cherrypick defines operations for selectively transplanting commits onto target branches.
package cherrypick

import (
	"context"
	"errors"

	"github.com/autonomous-bits/spool/internal/repository/branch"
)

var (
	// ErrCommitRequired reports a cherry-pick request without a source commit hash.
	ErrCommitRequired = errors.New("cherry-pick commit is required")
	// ErrTargetBranchRequired reports a cherry-pick request without a target branch.
	ErrTargetBranchRequired = branch.ErrRequired
	// ErrCommitNotFound reports a requested source commit that does not exist.
	ErrCommitNotFound = errors.New("commit not found")
	// ErrBranchNotFound reports a requested target branch that does not exist.
	ErrBranchNotFound = branch.ErrNotFound
	// ErrUncommittedStagedChanges reports an attempt to cherry-pick onto a branch with uncommitted staged mutations.
	ErrUncommittedStagedChanges = errors.New("branch has uncommitted staged changes")
	// ErrTargetLeaseHeld reports an attempt to cherry-pick onto a branch held by an active merge transaction.
	ErrTargetLeaseHeld = errors.New("merge target branch has an active transaction")
	// ErrConflicts reports that the cherry-pick produces property or structural conflicts against the target branch.
	ErrConflicts = errors.New("cherry-pick contains conflicts")
	// ErrReferentialIntegrityViolation reports missing entity endpoints or orphaned references during preflight validation.
	ErrReferentialIntegrityViolation = errors.New("cherry-pick referential integrity violation")
)

// Change describes an entity changed from the target snapshot by a cherry-pick.
type Change struct {
	Entity string `json:"entity"`
	ID     string `json:"id"`
	Change string `json:"change"`
}

// Conflict describes a deterministic three-way merge disagreement during cherry-picking.
type Conflict struct {
	ConflictID string   `json:"conflictId"`
	Category   string   `json:"category"`
	Entity     string   `json:"entity"`
	ID         string   `json:"id,omitempty"`
	Field      string   `json:"field,omitempty"`
	Paths      []string `json:"paths"`
}

// SchemaViolation describes a schema rule or constraint failure.
type SchemaViolation struct {
	Code     string `json:"code"`
	Entity   string `json:"entity"`
	EntityID string `json:"entityID"`
	Rule     string `json:"rule,omitempty"`
	Field    string `json:"field,omitempty"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
}

// Request describes the options for a cherry-pick transplantation operation.
type Request struct {
	// Commit identifies the source commit hash whose delta will be transplanted.
	Commit string `json:"commit"`
	// TargetBranch identifies the branch onto which the commit delta will be applied.
	TargetBranch string `json:"targetBranch"`
	// DryRun reports whether to simulate cherry-picking without writing commits or advancing branch refs.
	DryRun bool `json:"dryRun,omitempty"`
	// Author optionally overrides the commit author for non-dry-run executions.
	Author string `json:"author,omitempty"`
	// Message optionally overrides the commit message for non-dry-run executions.
	Message string `json:"message,omitempty"`
}

// Result summarizes the transplanted changes, resulting commit, conflicts, and schema violations.
type Result struct {
	// TargetBranch identifies the branch targeted by the cherry-pick.
	TargetBranch string `json:"targetBranch"`
	// SourceCommit identifies the source commit that was transplanted.
	SourceCommit string `json:"sourceCommit"`
	// Commit identifies the newly created commit, or the unchanged target head when dry-run or no-op.
	Commit string `json:"commit,omitempty"`
	// DryRun indicates whether this result was produced by a preview simulation.
	DryRun bool `json:"dryRun,omitempty"`
	// Changes contains the entities added, modified, or removed relative to the target branch head.
	Changes []Change `json:"changes"`
	// Conflicts lists any structural or property collisions detected against the target branch.
	Conflicts []Conflict `json:"conflicts,omitempty"`
	// Violations lists any schema rule violations detected on the candidate target snapshot.
	Violations []SchemaViolation `json:"violations,omitempty"`
}

// Store provides the repository-level cherry-pick persistence operations.
type Store interface {
	// CherryPick performs commit diff delta computation, 3-way property merging,
	// referential preflight verification, and atomic commit generation.
	CherryPick(request Request) (Result, error)
}

// Service validates cherry-pick requests and delegates execution to Store.
type Service struct {
	store Store
}

// NewService returns a cherry-pick service backed by store.
func NewService(store Store) Service {
	return Service{store: store}
}

// CherryPick validates the request, honors context cancellation, and executes cherry-pick through Store.
func (s Service) CherryPick(ctx context.Context, request Request) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if request.Commit == "" {
		return Result{}, ErrCommitRequired
	}
	if request.TargetBranch == "" {
		return Result{}, ErrTargetBranchRequired
	}
	return s.store.CherryPick(request)
}
