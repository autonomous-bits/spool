package repository

import (
	"context"
	"errors"
	"testing"
)

// setupDivergedReconciliation builds a dest repository with a local "main"
// branch that has diverged from a freshly fetched "reconcile/main" branch
// mirroring a remote's independent advance, both descending from the same
// deterministic seed commit.
func setupDivergedReconciliation(t *testing.T, localMutation, remoteMutation func(*Repository)) *Repository {
	t.Helper()
	ctx := context.Background()
	source := newTestSeedRepository(t)
	dest := newTestSeedRepository(t)

	remoteMutation(source)
	remotePack, err := source.BuildPushPack(ctx, "main", "")
	if err != nil {
		t.Fatalf("BuildPushPack (source): %v", err)
	}

	localMutation(dest)

	if _, err := dest.InstallReconciliationPack(ctx, ReconciliationBranchName("main"), [][]byte{remotePack.PackData}, remotePack.TargetCommit); err != nil {
		t.Fatalf("InstallReconciliationPack: %v", err)
	}
	return dest
}

func TestReconcileBranchProducesFastForwardEligibleCommit(t *testing.T) {
	ctx := context.Background()
	dest := setupDivergedReconciliation(t,
		func(r *Repository) { commitTestMutation(t, r, "local-only-node", "Local", "alice", "local advance") },
		func(r *Repository) { commitTestMutation(t, r, "remote-only-node", "Remote", "rack-bot", "remote advance") },
	)

	remoteBranch := ReconciliationBranchName("main")
	result, err := dest.ReconcileBranch("main", remoteBranch, "alice", "reconcile onto rack")
	if err != nil {
		t.Fatalf("ReconcileBranch: %v", err)
	}
	if !result.Preview.Clean {
		t.Fatalf("preview.Clean = false, want true (conflicts: %+v)", result.Preview.Conflicts)
	}

	// Both sides' content must be present in the reconciled commit.
	for _, id := range []string{"local-only-node", "remote-only-node"} {
		if _, err := dest.ResolvePinned(result.Commit, id); err != nil {
			t.Fatalf("ResolvePinned(%s): %v", id, err)
		}
	}

	// main must now point at the reconciled commit.
	if dest.branches["main"] != result.Commit {
		t.Fatalf("main head = %q, want reconciled commit %q", dest.branches["main"], result.Commit)
	}

	// The reconciled commit must have exactly one parent (remoteBranch's
	// head), so BuildPushPack's linear-history walk succeeds all the way
	// back to root, and the pack it builds is fast-forward eligible from
	// remoteBranch's own recomputed wire head.
	reconciled := dest.commits[result.Commit]
	if len(reconciled.Parents) != 1 {
		t.Fatalf("reconciled commit has %d parents, want 1", len(reconciled.Parents))
	}
	if reconciled.Parents[0] != dest.branches[remoteBranch] {
		t.Fatalf("reconciled commit parent = %q, want remote branch head %q", reconciled.Parents[0], dest.branches[remoteBranch])
	}

	remoteWirePack, err := dest.BuildPushPack(ctx, remoteBranch, "")
	if err != nil {
		t.Fatalf("BuildPushPack (remote branch): %v", err)
	}
	retryPack, err := dest.BuildPushPack(ctx, "main", remoteWirePack.TargetCommit)
	if err != nil {
		t.Fatalf("BuildPushPack (main, retry): %v", err)
	}
	if retryPack.BaseCommit != remoteWirePack.TargetCommit {
		t.Fatalf("retryPack.BaseCommit = %q, want %q", retryPack.BaseCommit, remoteWirePack.TargetCommit)
	}
	if len(retryPack.Commits) != 1 {
		t.Fatalf("retryPack carries %d commits, want exactly 1 (the reconciled commit)", len(retryPack.Commits))
	}
}

func TestReconcileBranchReportsConflictsWithoutMutating(t *testing.T) {
	dest := setupDivergedReconciliation(t,
		func(r *Repository) { commitTestMutation(t, r, "shared-node", "Local title", "alice", "local edit") },
		func(r *Repository) { commitTestMutation(t, r, "shared-node", "Remote title", "rack-bot", "remote edit") },
	)

	remoteBranch := ReconciliationBranchName("main")
	localHeadBefore := dest.branches["main"]
	remoteHeadBefore := dest.branches[remoteBranch]

	_, err := dest.ReconcileBranch("main", remoteBranch, "alice", "reconcile onto rack")
	if !errors.Is(err, ErrMergeConflicted) {
		t.Fatalf("err = %v, want ErrMergeConflicted", err)
	}

	if dest.branches["main"] != localHeadBefore {
		t.Fatalf("main head changed to %q, want unchanged %q", dest.branches["main"], localHeadBefore)
	}
	if dest.branches[remoteBranch] != remoteHeadBefore {
		t.Fatalf("%s head changed to %q, want unchanged %q", remoteBranch, dest.branches[remoteBranch], remoteHeadBefore)
	}
}
