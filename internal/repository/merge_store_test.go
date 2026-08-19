package repository

import (
	"errors"
	"reflect"
	"testing"
)

func TestConflictedMergeRetainsLeaseAfterCommittedStateWriteError(t *testing.T) {
	repo := NewSeedRepository()
	repo.mergeStateDir = t.TempDir()
	base, source, target := createDivergedBranchHeads(repo)
	repo.persistStateFn = func(string, string, *mergeTransaction) error {
		return durableWriteCommittedError{err: errors.New("injected directory sync failure")}
	}

	err := repo.ApplyConflictedBoundMerge("feature", "main", "owner", MergePreviewBinding{
		MergeBase: base, SourceCommit: source, TargetCommit: target,
	})
	if err == nil || !durableWriteCommitted(err) {
		t.Fatalf("ApplyConflictedBoundMerge error = %v, want committed-write error", err)
	}
	if got := repo.mergeLeases["main"]; got != "owner" {
		t.Fatalf("lease owner = %q, want owner", got)
	}
	if transaction, ok := repo.mergeTransactions["main"]; !ok || transaction.OwnerTransactionID != "owner" {
		t.Fatalf("transaction = %#v, want owner transaction", transaction)
	}
	if err := repo.ApplyConflictedBoundMerge("feature", "main", "other", MergePreviewBinding{
		MergeBase: base, SourceCommit: source, TargetCommit: target,
	}); !errors.Is(err, ErrMergeLeaseHeldByOther) {
		t.Fatalf("other owner error = %v, want ErrMergeLeaseHeldByOther", err)
	}
}

func TestApplyCleanBoundMergeMovesTargetToReachableMergeCommit(t *testing.T) {
	repo := NewSeedRepository()
	baseCommit, sourceCommit, targetCommit := createDivergedBranchHeads(repo)

	mergedCommit, err := repo.ApplyCleanBoundMerge("feature", "main", "test-transaction", MergePreviewBinding{
		MergeBase:    baseCommit,
		SourceCommit: sourceCommit,
		TargetCommit: targetCommit,
	})
	if err != nil {
		t.Fatalf("ApplyCleanBoundMerge: %v", err)
	}

	repo.mu.RLock()
	defer repo.mu.RUnlock()
	if got := repo.branches["main"]; got != mergedCommit {
		t.Fatalf("main head = %q, want merged commit %q", got, mergedCommit)
	}
	if mergedCommit == targetCommit {
		t.Fatal("main head did not move")
	}
	merged, ok := repo.commits[mergedCommit]
	if !ok {
		t.Fatalf("merged commit %q was not stored", mergedCommit)
	}
	if len(merged.Parents) != 2 || merged.Parents[0] != targetCommit || merged.Parents[1] != sourceCommit {
		t.Fatalf("merge parents = %#v, want [%q %q]", merged.Parents, targetCommit, sourceCommit)
	}
	if got := repo.branches["feature"]; got != sourceCommit {
		t.Fatalf("feature head = %q, want %q", got, sourceCommit)
	}
}

func TestApplyCleanBoundMergeRejectsDifferentMergeBase(t *testing.T) {
	repo := NewSeedRepository()
	_, sourceCommit, targetCommit := createDivergedBranchHeads(repo)

	_, err := repo.ApplyCleanBoundMerge("feature", "main", "test-transaction", MergePreviewBinding{
		MergeBase:    sourceCommit,
		SourceCommit: sourceCommit,
		TargetCommit: targetCommit,
	})
	if !errors.Is(err, ErrStaleMergePreview) {
		t.Fatalf("ApplyCleanBoundMerge error = %v, want ErrStaleMergePreview", err)
	}

	repo.mu.RLock()
	defer repo.mu.RUnlock()
	if got := repo.branches["main"]; got != targetCommit {
		t.Fatalf("main head = %q, want unchanged target %q", got, targetCommit)
	}
}

func TestMergeApplyRejectsStaleSourceOrTargetWithoutMutation(t *testing.T) {
	operations := []struct {
		name  string
		apply func(*Repository, MergePreviewBinding) error
	}{
		{
			name: "clean",
			apply: func(repo *Repository, binding MergePreviewBinding) error {
				_, err := repo.ApplyCleanBoundMerge("feature", "main", "test-transaction", binding)
				return err
			},
		},
		{
			name: "conflicted",
			apply: func(repo *Repository, binding MergePreviewBinding) error {
				return repo.ApplyConflictedBoundMerge("feature", "main", "test-transaction", binding)
			},
		},
	}
	staleBranches := []string{"feature", "main"}

	for _, operation := range operations {
		for _, staleBranch := range staleBranches {
			t.Run(operation.name+"/"+staleBranch, func(t *testing.T) {
				repo := NewSeedRepository()
				baseCommit, sourceCommit, targetCommit := createDivergedBranchHeads(repo)
				binding := MergePreviewBinding{
					MergeBase:    baseCommit,
					SourceCommit: sourceCommit,
					TargetCommit: targetCommit,
				}
				if _, err := repo.AdvanceBranch(staleBranch); err != nil {
					t.Fatalf("AdvanceBranch(%q): %v", staleBranch, err)
				}

				repo.mu.RLock()
				expectedSource := repo.branches["feature"]
				expectedTarget := repo.branches["main"]
				expectedCommitCount := len(repo.commits)
				repo.mu.RUnlock()

				err := operation.apply(repo, binding)
				if !errors.Is(err, ErrStaleMergePreview) {
					t.Fatalf("merge apply error = %v, want ErrStaleMergePreview", err)
				}

				repo.mu.RLock()
				defer repo.mu.RUnlock()
				if got := repo.branches["feature"]; got != expectedSource {
					t.Fatalf("feature head = %q, want unchanged %q", got, expectedSource)
				}
				if got := repo.branches["main"]; got != expectedTarget {
					t.Fatalf("main head = %q, want unchanged %q", got, expectedTarget)
				}
				if got := len(repo.commits); got != expectedCommitCount {
					t.Fatalf("commit count = %d, want unchanged count %d", got, expectedCommitCount)
				}
			})
		}
	}
}

func TestMergeApplyRejectsMissingPreviewBindingWithoutMutation(t *testing.T) {
	operations := []struct {
		name  string
		apply func(*Repository, MergePreviewBinding) error
	}{
		{
			name: "clean",
			apply: func(repo *Repository, binding MergePreviewBinding) error {
				_, err := repo.ApplyCleanBoundMerge("feature", "main", "test-transaction", binding)
				return err
			},
		},
		{
			name: "conflicted",
			apply: func(repo *Repository, binding MergePreviewBinding) error {
				return repo.ApplyConflictedBoundMerge("feature", "main", "test-transaction", binding)
			},
		},
	}
	missingBindings := []struct {
		name   string
		mutate func(*MergePreviewBinding)
	}{
		{
			name: "all fields",
			mutate: func(binding *MergePreviewBinding) {
				*binding = MergePreviewBinding{}
			},
		},
		{
			name: "merge base",
			mutate: func(binding *MergePreviewBinding) {
				binding.MergeBase = ""
			},
		},
		{
			name: "source commit",
			mutate: func(binding *MergePreviewBinding) {
				binding.SourceCommit = ""
			},
		},
		{
			name: "target commit",
			mutate: func(binding *MergePreviewBinding) {
				binding.TargetCommit = ""
			},
		},
	}

	for _, operation := range operations {
		for _, missingBinding := range missingBindings {
			t.Run(operation.name+"/"+missingBinding.name, func(t *testing.T) {
				repo := NewSeedRepository()
				baseCommit, sourceCommit, targetCommit := createDivergedBranchHeads(repo)
				binding := MergePreviewBinding{
					MergeBase:    baseCommit,
					SourceCommit: sourceCommit,
					TargetCommit: targetCommit,
				}
				missingBinding.mutate(&binding)

				repo.mu.RLock()
				initialCommitCount := len(repo.commits)
				repo.mu.RUnlock()

				err := operation.apply(repo, binding)
				if !errors.Is(err, ErrMissingMergePreviewBinding) {
					t.Fatalf("merge apply error = %v, want ErrMissingMergePreviewBinding", err)
				}

				repo.mu.RLock()
				defer repo.mu.RUnlock()
				if got := repo.branches["main"]; got != targetCommit {
					t.Fatalf("main head = %q, want unchanged target %q", got, targetCommit)
				}
				if got := repo.branches["feature"]; got != sourceCommit {
					t.Fatalf("feature head = %q, want unchanged source %q", got, sourceCommit)
				}
				if got := len(repo.commits); got != initialCommitCount {
					t.Fatalf("commit count = %d, want unchanged count %d", got, initialCommitCount)
				}
			})
		}
	}
}

func TestApplyConflictedBoundMergeRefusesWithoutMutation(t *testing.T) {
	repo := NewSeedRepository()
	baseCommit, sourceCommit, targetCommit := createDivergedBranchHeads(repo)

	repo.mu.RLock()
	initialCommitCount := len(repo.commits)
	repo.mu.RUnlock()

	err := repo.ApplyConflictedBoundMerge("feature", "main", "test-transaction", MergePreviewBinding{
		MergeBase:    baseCommit,
		SourceCommit: sourceCommit,
		TargetCommit: targetCommit,
	})
	if !errors.Is(err, ErrMergeConflicted) {
		t.Fatalf("ApplyConflictedBoundMerge error = %v, want ErrMergeConflicted", err)
	}

	repo.mu.RLock()
	defer repo.mu.RUnlock()
	if got := repo.branches["main"]; got != targetCommit {
		t.Fatalf("main head = %q, want unchanged target %q", got, targetCommit)
	}
	if got := repo.branches["feature"]; got != sourceCommit {
		t.Fatalf("feature head = %q, want unchanged source %q", got, sourceCommit)
	}
	if got := len(repo.commits); got != initialCommitCount {
		t.Fatalf("commit count = %d, want unchanged count %d", got, initialCommitCount)
	}
}

func TestMergeApplyRejectsLeaseHeldByOtherTransactionWithoutMutation(t *testing.T) {
	repo := NewSeedRepository()
	baseCommit, sourceCommit, targetCommit := createDivergedBranchHeads(repo)
	binding := MergePreviewBinding{
		MergeBase:    baseCommit,
		SourceCommit: sourceCommit,
		TargetCommit: targetCommit,
	}

	err := repo.ApplyConflictedBoundMerge("feature", "main", "owner", binding)
	if !errors.Is(err, ErrMergeConflicted) {
		t.Fatalf("owner conflicted apply error = %v, want ErrMergeConflicted", err)
	}

	repo.mu.RLock()
	initialCommitCount := len(repo.commits)
	repo.mu.RUnlock()

	_, err = repo.ApplyCleanBoundMerge("feature", "main", "other", binding)
	if !errors.Is(err, ErrMergeLeaseHeldByOther) {
		t.Fatalf("other transaction apply error = %v, want ErrMergeLeaseHeldByOther", err)
	}

	err = repo.ApplyConflictedBoundMerge("feature", "main", "owner", binding)
	if !errors.Is(err, ErrMergeConflicted) {
		t.Fatalf("owner retry error = %v, want ErrMergeConflicted", err)
	}

	repo.mu.RLock()
	defer repo.mu.RUnlock()
	if got := repo.branches["main"]; got != targetCommit {
		t.Fatalf("main head = %q, want unchanged target %q", got, targetCommit)
	}
	if got := repo.branches["feature"]; got != sourceCommit {
		t.Fatalf("feature head = %q, want unchanged source %q", got, sourceCommit)
	}
	if got := len(repo.commits); got != initialCommitCount {
		t.Fatalf("commit count = %d, want unchanged count %d", got, initialCommitCount)
	}
	if got := repo.mergeLeases["main"]; got != "owner" {
		t.Fatalf("main lease holder = %q, want owner", got)
	}
}

func TestApplyCleanBoundMergeClearsTransactionLease(t *testing.T) {
	repo := NewSeedRepository()
	baseCommit, sourceCommit, targetCommit := createDivergedBranchHeads(repo)
	binding := MergePreviewBinding{
		MergeBase:    baseCommit,
		SourceCommit: sourceCommit,
		TargetCommit: targetCommit,
	}

	if _, err := repo.ApplyCleanBoundMerge("feature", "main", "first", binding); err != nil {
		t.Fatalf("first ApplyCleanBoundMerge: %v", err)
	}

	repo.mu.RLock()
	_, held := repo.mergeLeases["main"]
	repo.mu.RUnlock()
	if held {
		t.Fatal("clean merge left a transaction lease on main")
	}

	baseCommit, sourceCommit, targetCommit = createDivergedBranchHeads(repo)
	_, err := repo.ApplyCleanBoundMerge("feature", "main", "second", MergePreviewBinding{
		MergeBase:    baseCommit,
		SourceCommit: sourceCommit,
		TargetCommit: targetCommit,
	})
	if err != nil {
		t.Fatalf("second ApplyCleanBoundMerge: %v", err)
	}
}

func TestMergeApplyRejectsMissingTransactionID(t *testing.T) {
	repo := NewSeedRepository()
	baseCommit, sourceCommit, targetCommit := createDivergedBranchHeads(repo)

	_, err := repo.ApplyCleanBoundMerge("feature", "main", "", MergePreviewBinding{
		MergeBase:    baseCommit,
		SourceCommit: sourceCommit,
		TargetCommit: targetCommit,
	})
	if !errors.Is(err, ErrMissingMergeTransactionID) {
		t.Fatalf("ApplyCleanBoundMerge error = %v, want ErrMissingMergeTransactionID", err)
	}
}

func TestMergeTransactionOperationsRejectNonOwnerWithoutMutation(t *testing.T) {
	operations := []struct {
		name string
		run  func(*Repository, ObjectID) error
	}{
		{
			name: "resolve",
			run: func(repo *Repository, snapshot ObjectID) error {
				return repo.ResolveMergeTransaction("main", "other", snapshot)
			},
		},
		{
			name: "restage",
			run: func(repo *Repository, _ ObjectID) error {
				return repo.RestageMergeTransaction("main", "other")
			},
		},
		{
			name: "finalize",
			run: func(repo *Repository, _ ObjectID) error {
				_, err := repo.FinalizeMergeTransaction("main", "other")
				return err
			},
		},
		{
			name: "abort",
			run: func(repo *Repository, _ ObjectID) error {
				return repo.AbortMergeTransaction("main", "other")
			},
		},
	}

	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			repo := NewSeedRepository()
			base, source, target := createDivergedBranchHeads(repo)
			binding := MergePreviewBinding{MergeBase: base, SourceCommit: source, TargetCommit: target}
			if err := repo.ApplyConflictedBoundMerge("feature", "main", "owner", binding); !errors.Is(err, ErrMergeConflicted) {
				t.Fatalf("ApplyConflictedBoundMerge error = %v, want ErrMergeConflicted", err)
			}

			repo.mu.RLock()
			initialSource := repo.branches["feature"]
			initialTarget := repo.branches["main"]
			initialCommitCount := len(repo.commits)
			initialLease := repo.mergeLeases["main"]
			initialTransaction := repo.mergeTransactions["main"]
			stagedSnapshot := repo.commits[target].Snapshot
			repo.mu.RUnlock()

			err := operation.run(repo, stagedSnapshot)
			if !errors.Is(err, ErrMergeOperationNotOwner) {
				t.Fatalf("%s error = %v, want ErrMergeOperationNotOwner", operation.name, err)
			}

			repo.mu.RLock()
			defer repo.mu.RUnlock()
			if got := repo.branches["feature"]; got != initialSource {
				t.Fatalf("feature head = %q, want unchanged %q", got, initialSource)
			}
			if got := repo.branches["main"]; got != initialTarget {
				t.Fatalf("main head = %q, want unchanged %q", got, initialTarget)
			}
			if got := len(repo.commits); got != initialCommitCount {
				t.Fatalf("commit count = %d, want unchanged count %d", got, initialCommitCount)
			}
			if got := repo.mergeLeases["main"]; got != initialLease {
				t.Fatalf("lease holder = %q, want %q", got, initialLease)
			}
			if got := repo.mergeTransactions["main"]; !reflect.DeepEqual(got, initialTransaction) {
				t.Fatalf("transaction = %#v, want %#v", got, initialTransaction)
			}
		})
	}
}

func TestMergeTransactionOwnerCanResolveRestageAndFinalize(t *testing.T) {
	repo := NewSeedRepository()
	base, source, target := createDivergedBranchHeads(repo)
	binding := MergePreviewBinding{MergeBase: base, SourceCommit: source, TargetCommit: target}
	if err := repo.ApplyConflictedBoundMerge("feature", "main", "owner", binding); !errors.Is(err, ErrMergeConflicted) {
		t.Fatalf("ApplyConflictedBoundMerge error = %v, want ErrMergeConflicted", err)
	}

	repo.mu.RLock()
	stagedSnapshot := repo.commits[target].Snapshot
	repo.mu.RUnlock()
	if err := repo.ResolveMergeTransaction("main", "owner", stagedSnapshot); err != nil {
		t.Fatalf("ResolveMergeTransaction: %v", err)
	}
	if err := repo.RestageMergeTransaction("main", "owner"); err != nil {
		t.Fatalf("RestageMergeTransaction: %v", err)
	}
	merged, err := repo.FinalizeMergeTransaction("main", "owner")
	if err != nil {
		t.Fatalf("FinalizeMergeTransaction: %v", err)
	}

	repo.mu.RLock()
	defer repo.mu.RUnlock()
	if got := repo.branches["main"]; got != merged {
		t.Fatalf("main head = %q, want %q", got, merged)
	}
	if got := repo.commits[merged].Snapshot; got != stagedSnapshot {
		t.Fatalf("merged snapshot = %q, want staged snapshot %q", got, stagedSnapshot)
	}
	if _, held := repo.mergeLeases["main"]; held {
		t.Fatal("finalized merge retained its lease")
	}
	if _, active := repo.mergeTransactions["main"]; active {
		t.Fatal("finalized merge retained its transaction state")
	}
}

func TestMergeTransactionRequiresResolutionAndRestage(t *testing.T) {
	repo := NewSeedRepository()
	base, source, target := createDivergedBranchHeads(repo)
	binding := MergePreviewBinding{MergeBase: base, SourceCommit: source, TargetCommit: target}
	if err := repo.ApplyConflictedBoundMerge("feature", "main", "owner", binding); !errors.Is(err, ErrMergeConflicted) {
		t.Fatalf("ApplyConflictedBoundMerge error = %v, want ErrMergeConflicted", err)
	}

	if _, err := repo.FinalizeMergeTransaction("main", "owner"); !errors.Is(err, ErrMergeResolutionIncomplete) {
		t.Fatalf("FinalizeMergeTransaction before resolution error = %v, want ErrMergeResolutionIncomplete", err)
	}
	repo.mu.RLock()
	stagedSnapshot := repo.commits[target].Snapshot
	repo.mu.RUnlock()
	if err := repo.ResolveMergeTransaction("main", "owner", stagedSnapshot); err != nil {
		t.Fatalf("ResolveMergeTransaction: %v", err)
	}
	if _, err := repo.FinalizeMergeTransaction("main", "owner"); !errors.Is(err, ErrMergeResolutionIncomplete) {
		t.Fatalf("FinalizeMergeTransaction before restage error = %v, want ErrMergeResolutionIncomplete", err)
	}
}

func TestMergeTransactionAbortAndTargetLeaseProtection(t *testing.T) {
	repo := NewSeedRepository()
	base, source, target := createDivergedBranchHeads(repo)
	binding := MergePreviewBinding{MergeBase: base, SourceCommit: source, TargetCommit: target}
	if err := repo.ApplyConflictedBoundMerge("feature", "main", "owner", binding); !errors.Is(err, ErrMergeConflicted) {
		t.Fatalf("ApplyConflictedBoundMerge error = %v, want ErrMergeConflicted", err)
	}
	if _, err := repo.AdvanceBranch("main"); !errors.Is(err, ErrMergeTargetLeaseHeld) {
		t.Fatalf("AdvanceBranch error = %v, want ErrMergeTargetLeaseHeld", err)
	}
	if err := repo.AbortMergeTransaction("main", "owner"); err != nil {
		t.Fatalf("AbortMergeTransaction: %v", err)
	}

	repo.mu.RLock()
	if got := repo.branches["main"]; got != target {
		t.Fatalf("main head = %q, want unchanged %q", got, target)
	}
	repo.mu.RUnlock()
	if _, err := repo.AdvanceBranch("main"); err != nil {
		t.Fatalf("AdvanceBranch after abort: %v", err)
	}
}

func TestCleanRetryCannotReleaseConflictedTransactionLease(t *testing.T) {
	repo := NewSeedRepository()
	base, source, target := createDivergedBranchHeads(repo)
	binding := MergePreviewBinding{MergeBase: base, SourceCommit: source, TargetCommit: target}
	if err := repo.ApplyConflictedBoundMerge("feature", "main", "owner", binding); !errors.Is(err, ErrMergeConflicted) {
		t.Fatalf("ApplyConflictedBoundMerge error = %v, want ErrMergeConflicted", err)
	}

	if _, err := repo.ApplyCleanBoundMerge("feature", "main", "owner", binding); !errors.Is(err, ErrMergeNotInProgress) {
		t.Fatalf("ApplyCleanBoundMerge error = %v, want ErrMergeNotInProgress", err)
	}
	if err := repo.ApplyConflictedBoundMerge("feature", "main", "other", binding); !errors.Is(err, ErrMergeLeaseHeldByOther) {
		t.Fatalf("other conflicted apply error = %v, want ErrMergeLeaseHeldByOther", err)
	}

	repo.mu.RLock()
	stagedSnapshot := repo.commits[target].Snapshot
	repo.mu.RUnlock()
	if err := repo.ResolveMergeTransaction("main", "owner", stagedSnapshot); err != nil {
		t.Fatalf("ResolveMergeTransaction: %v", err)
	}
}

func TestFinalizeMergeTransactionRejectsMovedTarget(t *testing.T) {
	repo := NewSeedRepository()
	base, source, target := createDivergedBranchHeads(repo)
	binding := MergePreviewBinding{MergeBase: base, SourceCommit: source, TargetCommit: target}
	if err := repo.ApplyConflictedBoundMerge("feature", "main", "owner", binding); !errors.Is(err, ErrMergeConflicted) {
		t.Fatalf("ApplyConflictedBoundMerge error = %v, want ErrMergeConflicted", err)
	}

	repo.mu.RLock()
	stagedSnapshot := repo.commits[target].Snapshot
	repo.mu.RUnlock()
	if err := repo.ResolveMergeTransaction("main", "owner", stagedSnapshot); err != nil {
		t.Fatalf("ResolveMergeTransaction: %v", err)
	}
	if err := repo.RestageMergeTransaction("main", "owner"); err != nil {
		t.Fatalf("RestageMergeTransaction: %v", err)
	}

	repo.mu.Lock()
	moved := commit{Snapshot: stagedSnapshot, Parents: []ObjectID{target}, Message: "simulate external target update"}
	movedID := repo.store("commit", moved)
	repo.commits[movedID] = moved
	repo.branches["main"] = movedID
	repo.mu.Unlock()

	if _, err := repo.FinalizeMergeTransaction("main", "owner"); !errors.Is(err, ErrStaleMergePreview) {
		t.Fatalf("FinalizeMergeTransaction error = %v, want ErrStaleMergePreview", err)
	}
	repo.mu.RLock()
	defer repo.mu.RUnlock()
	if got := repo.branches["main"]; got != movedID {
		t.Fatalf("main head = %q, want unchanged moved head %q", got, movedID)
	}
	if _, active := repo.mergeTransactions["main"]; !active {
		t.Fatal("stale finalization cleared active transaction")
	}
}

func createDivergedBranchHeads(repo *Repository) (base, source, target ObjectID) {
	repo.mu.Lock()
	defer repo.mu.Unlock()

	base = repo.branches["main"]
	snapshot := repo.commits[base].Snapshot
	source = repo.store("commit", commit{
		Snapshot: snapshot,
		Parents:  []ObjectID{base},
		Message:  "advance feature branch",
	})
	target = repo.store("commit", commit{
		Snapshot: snapshot,
		Parents:  []ObjectID{base},
		Message:  "advance main branch",
	})
	repo.commits[source] = commit{
		Snapshot: snapshot,
		Parents:  []ObjectID{base},
		Message:  "advance feature branch",
	}
	repo.commits[target] = commit{
		Snapshot: snapshot,
		Parents:  []ObjectID{base},
		Message:  "advance main branch",
	}
	repo.branches["feature"] = source
	repo.branches["main"] = target
	return base, source, target
}
