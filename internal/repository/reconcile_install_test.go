package repository

import (
	"context"
	"testing"
)

// TestInstallReconciliationPackBootstrapsDivergedBranch verifies the core
// invariant the non-fast-forward reconciliation flow depends on: installing
// a from-scratch (empty base) history fetched from Rack onto a dedicated
// local reconciliation branch succeeds even though the repository's own
// "main" branch has independently diverged, and the shared historical
// prefix collapses onto the same content-addressed local commit objects, so
// the graph merge engine can find a genuine common ancestor between the two
// branches.
func TestInstallReconciliationPackBootstrapsDivergedBranch(t *testing.T) {
	ctx := context.Background()
	source := newTestSeedRepository(t)
	dest := newTestSeedRepository(t)

	// dest and source start from the identical deterministic seed state, so
	// their "main" branches share a real common ancestor (the seed commit)
	// even though it was computed independently in each repository.

	// Remote (source) advances with a commit dest never saw.
	commitTestMutation(t, source, "remote-only-node", "Remote change", "rack-bot", "remote advance")
	remotePack, err := source.BuildPushPack(ctx, "main", "")
	if err != nil {
		t.Fatalf("BuildPushPack (source, from scratch): %v", err)
	}
	if remotePack.BaseCommit != "" {
		t.Fatalf("remotePack.BaseCommit = %q, want empty for a from-scratch pack", remotePack.BaseCommit)
	}

	// Local (dest) independently advances with a different commit,
	// diverging from what source now has.
	commitTestMutation(t, dest, "local-only-node", "Local change", "alice", "local advance")

	result, err := dest.InstallReconciliationPack(ctx, "reconcile/main", [][]byte{remotePack.PackData}, remotePack.TargetCommit)
	if err != nil {
		t.Fatalf("InstallReconciliationPack: %v", err)
	}
	if result.Branch != "reconcile/main" {
		t.Fatalf("Branch = %q, want reconcile/main", result.Branch)
	}
	// The from-scratch pack walks the entire history from the root seed
	// commit, so it carries both the seed commit and the new remote commit.
	if result.CommitsInstalled != 2 {
		t.Fatalf("CommitsInstalled = %d, want 2", result.CommitsInstalled)
	}

	// The reconciliation branch reflects Rack's remote-only content...
	if _, err := dest.ResolvePinned(result.HeadCommit, "remote-only-node"); err != nil {
		t.Fatalf("ResolvePinned(remote-only-node) on reconciliation branch: %v", err)
	}
	if _, err := dest.ResolvePinned(result.HeadCommit, "local-only-node"); err == nil {
		t.Fatal("reconciliation branch unexpectedly contains local-only-node")
	}

	// ...while "main" is left completely untouched.
	mainHead := dest.branches["main"]
	if _, err := dest.ResolvePinned(mainHead, "local-only-node"); err != nil {
		t.Fatalf("ResolvePinned(local-only-node) on main: %v", err)
	}
	if _, err := dest.ResolvePinned(mainHead, "remote-only-node"); err == nil {
		t.Fatal("main branch unexpectedly contains remote-only-node")
	}

	// The two branches must have a genuine common ancestor: the shared
	// seed prefix collapsed onto the same content-addressed local commit,
	// so PreviewMerge must succeed rather than failing to find a merge
	// base, and the clean result must carry both sides' independent
	// changes.
	preview, err := dest.PreviewMerge("main", "reconcile/main")
	if err != nil {
		t.Fatalf("PreviewMerge: %v", err)
	}
	if !preview.Clean {
		t.Fatalf("preview.Clean = false, want true (conflicts: %+v)", preview.Conflicts)
	}
	if len(preview.Changes) == 0 {
		t.Fatal("preview.Changes is empty, want at least the local-only-node addition")
	}
}

// TestInstallReconciliationPackRejectsNonScratchBase verifies that
// InstallReconciliationPack refuses a pack that declares a non-empty base:
// it is only meant to install the complete from-scratch history Rack
// returns for a reconciliation pull, never an incremental one.
func TestInstallReconciliationPackRejectsNonScratchBase(t *testing.T) {
	ctx := context.Background()
	source := newTestSeedRepository(t)
	dest := newTestSeedRepository(t)

	seedPack, err := source.BuildPushPack(ctx, "main", "")
	if err != nil {
		t.Fatalf("BuildPushPack (seed): %v", err)
	}
	commitTestMutation(t, source, "node-1", "First", "alice", "first commit")
	incrementalPack, err := source.BuildPushPack(ctx, "main", seedPack.TargetCommit)
	if err != nil {
		t.Fatalf("BuildPushPack (incremental): %v", err)
	}

	_, err = dest.InstallReconciliationPack(ctx, "reconcile/main", [][]byte{incrementalPack.PackData}, incrementalPack.TargetCommit)
	if err == nil {
		t.Fatal("expected an error for a non-scratch base, got nil")
	}
}
