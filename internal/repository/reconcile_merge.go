package repository

import (
	"errors"
	"fmt"
)

// ReconciliationBranchName returns the deterministic local branch name used
// to mirror a remote branch's complete current history during non-fast-forward
// push reconciliation (see spl push --reconcile). It is a plain naming
// convention, not itself durable state: the branch is (re)created fresh by
// InstallReconciliationPack on every reconciliation attempt.
func ReconciliationBranchName(branch string) string {
	return "reconcile/" + branch
}

// ReconcileResult reports the outcome of ReconcileBranch.
type ReconcileResult struct {
	// Commit is the new fast-forward-eligible commit branch was advanced
	// to. Its sole parent is remoteBranch's head commit, so a subsequent
	// BuildPushPack/push against remoteBranch's real (Rack) head succeeds
	// under push's linear-history-only requirement.
	Commit ObjectID
	// Preview is the merge preview the reconciled commit's content was
	// computed from. Callers that need conflict detail on an unclean
	// result should inspect Preview.Conflicts (and Preview.Clean is always
	// false in that case; ReconcileBranch returns ErrMergeConflicted
	// alongside it).
	Preview MergePreview
}

// ReconcileBranch rebases branch's independent local changes onto
// remoteBranch's content, producing a single-parent commit suitable for a
// fast-forward retry push, and advances branch to it.
//
// It reuses the same deterministic three-way merge computation as `spl
// merge preview` (source=branch, target=remoteBranch), but — unlike
// ApplyMergePreview, which records a two-parent merge commit on the target
// branch — records a clean result as a single-parent commit on branch whose
// only parent is remoteBranch's head commit. This matters because
// BuildPushPack (and therefore `spl push`) only supports linear,
// single-parent history: a real two-parent merge commit could never be
// pushed. Because the merged content is otherwise identical to a genuine
// three-way merge, this is a "rebase local changes onto Rack's current
// truth" operation, not a content-losing shortcut.
//
// If the preview is not clean, branch and remoteBranch are left completely
// untouched (both histories are retained) and ReconcileBranch returns
// ErrMergeConflicted alongside the conflicted MergePreview so the caller can
// report it; the caller may resolve those conflicts with the existing `spl
// merge preview/apply/conflicts/resolve/finalize` commands against branch
// and remoteBranch directly, then retry reconciliation.
func (r *Repository) ReconcileBranch(branch, remoteBranch, author, message string) (ReconcileResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpenLocked(); err != nil {
		return ReconcileResult{}, err
	}
	if _, active := r.mergeTransactions[branch]; active {
		return ReconcileResult{}, ErrMergeTargetLeaseHeld
	}

	candidate, err := r.previewMergeLocked(branch, remoteBranch)
	if err != nil {
		return ReconcileResult{}, err
	}
	if !candidate.preview.Clean {
		return ReconcileResult{Preview: candidate.preview}, ErrMergeConflicted
	}

	previousHead := candidate.preview.Binding.SourceCommit
	remoteHead := candidate.preview.Binding.TargetCommit

	objects, snapshots, projections, edgeProjections := r.objects, r.snapshots, r.projections, r.edgeProjections
	materializedSnapshots, historicalProjectionLRU := r.materializedSnapshots, r.historicalProjectionLRU
	commits, branches := r.commits, r.branches
	restore := func() {
		r.objects, r.snapshots, r.projections, r.edgeProjections = objects, snapshots, projections, edgeProjections
		r.materializedSnapshots, r.historicalProjectionLRU = materializedSnapshots, historicalProjectionLRU
		r.commits, r.branches = commits, branches
	}
	r.objects, r.snapshots = cloneObjects(r.objects), cloneSnapshots(r.snapshots)
	r.projections, r.edgeProjections = cloneProjectionMap(r.projections), cloneEdgeProjectionMap(r.edgeProjections)
	r.materializedSnapshots, r.historicalProjectionLRU = cloneMaterializedSnapshots(r.materializedSnapshots), append([]ObjectID(nil), r.historicalProjectionLRU...)
	r.commits, r.branches = cloneCommits(r.commits), cloneBranches(r.branches)

	snapshot, err := r.materializeSnapshotLocked(candidate.nodes, candidate.edges, candidate.schemaRoot)
	if err != nil {
		restore()
		return ReconcileResult{}, fmt.Errorf("materialize reconciled snapshot: %w", err)
	}
	snapshotID, err := r.storeObject("graph-snapshot", snapshot)
	if err != nil {
		restore()
		return ReconcileResult{}, fmt.Errorf("store reconciled snapshot: %w", err)
	}
	r.snapshots[snapshotID] = snapshot
	if err := r.reconstructSnapshotProjectionsLocked(snapshotID, snapshot); err != nil {
		restore()
		return ReconcileResult{}, fmt.Errorf("reconstruct reconciled snapshot: %w", err)
	}

	// The reconciled commit's sole parent is remoteBranch's head: from
	// Rack's perspective this is a plain fast-forward child of the commit
	// it already has, even though it also carries branch's independent
	// local changes.
	reconciled := r.newCommit(snapshotID, []ObjectID{remoteHead}, author, message)
	reconciledID, err := r.storeObject("commit", reconciled)
	if err != nil {
		restore()
		return ReconcileResult{}, fmt.Errorf("store reconciled commit: %w", err)
	}
	r.commits[reconciledID], r.branches[branch] = reconciled, reconciledID
	if err := r.ensureBranchHeadProjectionsLocked(); err != nil {
		restore()
		return ReconcileResult{}, fmt.Errorf("pin reconciled snapshot: %w", err)
	}

	if err := r.writeRefLocked(branch, previousHead, reconciledID, "reconcile"); err != nil {
		if durableWriteCommitted(err) {
			return ReconcileResult{Commit: reconciledID, Preview: candidate.preview},
				fmt.Errorf("reconciliation committed but directory sync failed: %w", errors.Join(err, r.maintainActiveProjectionLocked(branch)))
		}
		restore()
		_ = r.ensureBranchHeadProjectionsLocked()
		return ReconcileResult{}, err
	}
	if err := r.maintainActiveProjectionLocked(branch); err != nil {
		return ReconcileResult{Commit: reconciledID, Preview: candidate.preview}, fmt.Errorf("reconciliation completed but projection maintenance failed: %w", err)
	}
	return ReconcileResult{Commit: reconciledID, Preview: candidate.preview}, nil
}
