// Package merge defines merge transaction lifecycle operations.
package merge

import "github.com/autonomous-bits/spool/internal/repository"

// ObjectID is the durable identifier used by repository merge operations.
type ObjectID = repository.ObjectID

// PreviewBinding pins the commits inspected by a merge preview.
type PreviewBinding struct {
	// MergeBase is the common ancestor used to create the preview.
	MergeBase ObjectID
	// SourceCommit is the source head inspected by the preview.
	SourceCommit ObjectID
	// TargetCommit is the target head inspected by the preview.
	TargetCommit ObjectID
}

func (b PreviewBinding) record() repository.MergePreviewBinding {
	return repository.MergePreviewBinding{
		MergeBase: b.MergeBase, SourceCommit: b.SourceCommit, TargetCommit: b.TargetCommit,
	}
}

var (
	// ErrMissingPreviewBinding reports an incomplete preview binding.
	ErrMissingPreviewBinding = repository.ErrMissingMergePreviewBinding
	// ErrMissingTransactionID reports an empty merge transaction ID.
	ErrMissingTransactionID = repository.ErrMissingMergeTransactionID
	// ErrStalePreview reports a preview whose branch state has changed.
	ErrStalePreview = repository.ErrStaleMergePreview
	// ErrConflicted reports a merge requiring resolution.
	ErrConflicted = repository.ErrMergeConflicted
	// ErrLeaseHeldByOther reports a target transaction owned by another caller.
	ErrLeaseHeldByOther = repository.ErrMergeLeaseHeldByOther
	// ErrOperationNotOwner reports an operation from a non-owning transaction.
	ErrOperationNotOwner = repository.ErrMergeOperationNotOwner
	// ErrNotInProgress reports a missing merge transaction.
	ErrNotInProgress = repository.ErrMergeNotInProgress
	// ErrResolutionIncomplete reports finalization before required resolution steps.
	ErrResolutionIncomplete = repository.ErrMergeResolutionIncomplete
	// ErrStagedSnapshotMissing reports an unknown resolution snapshot.
	ErrStagedSnapshotMissing = repository.ErrMergeStagedSnapshotMissing
	// ErrTargetLeaseHeld reports a branch mutation blocked by a merge lease.
	ErrTargetLeaseHeld = repository.ErrMergeTargetLeaseHeld
)

// Store is the durable repository contract required by merge lifecycle
// operations. Repository owns atomic mutation and recovery; this package owns
// the merge-facing operation boundary.
type Store interface {
	// PreviewMerge computes an immutable three-way merge without moving refs.
	PreviewMerge(string, string) (repository.MergePreview, error)
	// ApplyMergePreview recomputes and applies an exact clean preview.
	ApplyMergePreview(string, string, string, repository.ObjectID, string, string) (repository.ObjectID, error)
	// ApplyCleanBoundMerge validates a preview binding and commits a clean merge.
	ApplyCleanBoundMerge(string, string, string, repository.MergePreviewBinding) (repository.ObjectID, error)
	// ApplyConflictedBoundMerge records a conflicted merge transaction.
	ApplyConflictedBoundMerge(string, string, string, repository.MergePreviewBinding) error
	// ResolveMergeTransaction records an owning transaction's resolution snapshot.
	ResolveMergeTransaction(string, string, repository.ObjectID) error
	// RestageMergeTransaction records that an owning transaction was restaged.
	RestageMergeTransaction(string, string) error
	// FinalizeMergeTransaction commits and removes a completed transaction.
	FinalizeMergeTransaction(string, string) (repository.ObjectID, error)
	// AbortMergeTransaction removes an owning transaction.
	AbortMergeTransaction(string, string) error
}

// Service coordinates merge transaction lifecycle requests.
type Service struct{ store Store }

// NewService returns a merge lifecycle service backed by store.
func NewService(store Store) Service { return Service{store: store} }

// Preview computes a deterministic merge preview without changing branch refs.
func (s Service) Preview(source, target string) (repository.MergePreview, error) {
	return s.store.PreviewMerge(source, target)
}

// ApplyPreview applies a reviewed clean preview using caller commit metadata.
func (s Service) ApplyPreview(source, target, transactionID string, previewID ObjectID, author, message string) (ObjectID, error) {
	return s.store.ApplyMergePreview(source, target, transactionID, previewID, author, message)
}

// ApplyClean validates and applies a clean preview-bound merge.
func (s Service) ApplyClean(source, target, transactionID string, binding PreviewBinding) (ObjectID, error) {
	return s.store.ApplyCleanBoundMerge(source, target, transactionID, binding.record())
}

// ApplyConflicted records a conflicted preview-bound merge transaction.
func (s Service) ApplyConflicted(source, target, transactionID string, binding PreviewBinding) error {
	return s.store.ApplyConflictedBoundMerge(source, target, transactionID, binding.record())
}

// Resolve records the snapshot produced by resolving a transaction.
func (s Service) Resolve(target, transactionID string, stagedSnapshot ObjectID) error {
	return s.store.ResolveMergeTransaction(target, transactionID, stagedSnapshot)
}

// Restage records that a transaction's resolution was restaged.
func (s Service) Restage(target, transactionID string) error {
	return s.store.RestageMergeTransaction(target, transactionID)
}

// Finalize commits a resolved and restaged transaction.
func (s Service) Finalize(target, transactionID string) (ObjectID, error) {
	return s.store.FinalizeMergeTransaction(target, transactionID)
}

// Abort removes an owning merge transaction.
func (s Service) Abort(target, transactionID string) error {
	return s.store.AbortMergeTransaction(target, transactionID)
}
