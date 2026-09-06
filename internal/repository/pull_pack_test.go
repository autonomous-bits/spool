package repository

import (
	"context"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

func TestInstallPullPackFastForwardsFromMatchingBase(t *testing.T) {
	ctx := context.Background()
	source := newTestSeedRepository(t)
	dest := newTestSeedRepository(t)

	seedPack, err := source.BuildPushPack(ctx, "main", "")
	if err != nil {
		t.Fatalf("BuildPushPack (seed): %v", err)
	}

	commitTestMutation(t, source, "pull-node-1", "First", "alice", "first commit")
	commitTestMutation(t, source, "pull-node-2", "Second", "bob", "second commit")

	pack, err := source.BuildPushPack(ctx, "main", seedPack.TargetCommit)
	if err != nil {
		t.Fatalf("BuildPushPack (incremental): %v", err)
	}
	if pack.BaseCommit != seedPack.TargetCommit {
		t.Fatalf("pack.BaseCommit = %q, want %q", pack.BaseCommit, seedPack.TargetCommit)
	}

	result, err := dest.InstallPullPack(ctx, "main", [][]byte{pack.PackData}, pack.TargetCommit)
	if err != nil {
		t.Fatalf("InstallPullPack: %v", err)
	}
	if result.CommitsInstalled != 2 {
		t.Fatalf("CommitsInstalled = %d, want 2", result.CommitsInstalled)
	}
	if result.Branch != "main" {
		t.Fatalf("Branch = %q, want main", result.Branch)
	}
	if result.HeadCommit == "" {
		t.Fatal("HeadCommit is empty")
	}

	for _, id := range []string{"pull-node-1", "pull-node-2"} {
		resolution, err := dest.ResolvePinned(result.HeadCommit, id)
		if err != nil {
			t.Fatalf("ResolvePinned(%s): %v", id, err)
		}
		if resolution.Node.ID != id {
			t.Fatalf("resolved node ID = %q, want %q", resolution.Node.ID, id)
		}
	}

	// Installed history must recompute the exact same wire chain the
	// source repository produced, since Author/Message/Time were
	// preserved verbatim.
	again, err := dest.BuildPushPack(ctx, "main", "")
	if err != nil {
		t.Fatalf("BuildPushPack (dest, verify): %v", err)
	}
	sourceAgain, err := source.BuildPushPack(ctx, "main", "")
	if err != nil {
		t.Fatalf("BuildPushPack (source, verify): %v", err)
	}
	if again.TargetCommit != sourceAgain.TargetCommit {
		t.Fatalf("dest recomputed head = %q, want %q matching source", again.TargetCommit, sourceAgain.TargetCommit)
	}
}

func TestInstallPullPackMultiplePacksChain(t *testing.T) {
	ctx := context.Background()
	source := newTestSeedRepository(t)
	dest := newTestSeedRepository(t)

	seedPack, err := source.BuildPushPack(ctx, "main", "")
	if err != nil {
		t.Fatalf("BuildPushPack (seed): %v", err)
	}

	commitTestMutation(t, source, "pull-node-1", "First", "alice", "first commit")
	first, err := source.BuildPushPack(ctx, "main", seedPack.TargetCommit)
	if err != nil {
		t.Fatalf("BuildPushPack (first): %v", err)
	}

	commitTestMutation(t, source, "pull-node-2", "Second", "bob", "second commit")
	second, err := source.BuildPushPack(ctx, "main", first.TargetCommit)
	if err != nil {
		t.Fatalf("BuildPushPack (second): %v", err)
	}

	result, err := dest.InstallPullPack(ctx, "main", [][]byte{first.PackData, second.PackData}, second.TargetCommit)
	if err != nil {
		t.Fatalf("InstallPullPack: %v", err)
	}
	if result.CommitsInstalled != 2 {
		t.Fatalf("CommitsInstalled = %d, want 2", result.CommitsInstalled)
	}
}

func TestInstallPullPackRejectsBaseMismatch(t *testing.T) {
	ctx := context.Background()
	source := newTestSeedRepository(t)
	dest := newTestSeedRepository(t)

	commitTestMutation(t, source, "pull-node-1", "First", "alice", "first commit")
	seedPack, err := source.BuildPushPack(ctx, "main", "")
	if err != nil {
		t.Fatalf("BuildPushPack (seed): %v", err)
	}
	commitTestMutation(t, source, "pull-node-2", "Second", "bob", "second commit")
	pack, err := source.BuildPushPack(ctx, "main", seedPack.TargetCommit)
	if err != nil {
		t.Fatalf("BuildPushPack: %v", err)
	}

	// dest never had "pull-node-1" committed, so its recomputed head wire
	// ID cannot equal pack.BaseCommit.
	_, err = dest.InstallPullPack(ctx, "main", [][]byte{pack.PackData}, pack.TargetCommit)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestInstallPullPackRejectsBootstrap(t *testing.T) {
	ctx := context.Background()
	source := newTestSeedRepository(t)
	dest := newTestSeedRepository(t)

	commitTestMutation(t, source, "pull-node-1", "First", "alice", "first commit")
	pack, err := source.BuildPushPack(ctx, "main", "")
	if err != nil {
		t.Fatalf("BuildPushPack: %v", err)
	}
	if pack.BaseCommit != "" {
		t.Fatalf("BaseCommit = %q, want empty for a root pack", pack.BaseCommit)
	}

	_, err = dest.InstallPullPack(ctx, "main", [][]byte{pack.PackData}, pack.TargetCommit)
	if err == nil {
		t.Fatal("expected ErrPullBootstrapUnsupported, got nil")
	}
}

func TestInstallPullPackRejectsTamperedObjectHash(t *testing.T) {
	ctx := context.Background()
	source := newTestSeedRepository(t)
	dest := newTestSeedRepository(t)

	seedPack, err := source.BuildPushPack(ctx, "main", "")
	if err != nil {
		t.Fatalf("BuildPushPack (seed): %v", err)
	}
	commitTestMutation(t, source, "pull-node-1", "First", "alice", "first commit")
	pack, err := source.BuildPushPack(ctx, "main", seedPack.TargetCommit)
	if err != nil {
		t.Fatalf("BuildPushPack: %v", err)
	}

	var frame pushPackFrame
	if err := cbor.Unmarshal(pack.PackData, &frame); err != nil {
		t.Fatalf("decode pack: %v", err)
	}
	frame.Objects[0].Data = append([]byte(nil), frame.Objects[0].Data...)
	frame.Objects[0].Data[0] ^= 0xFF
	tampered, err := pushCanonicalCBOR.Marshal(frame)
	if err != nil {
		t.Fatalf("re-marshal tampered pack: %v", err)
	}

	_, err = dest.InstallPullPack(ctx, "main", [][]byte{tampered}, pack.TargetCommit)
	if err == nil {
		t.Fatal("expected an error for tampered object hash, got nil")
	}
}

func TestInstallPullPackRejectsTamperedTarget(t *testing.T) {
	ctx := context.Background()
	source := newTestSeedRepository(t)
	dest := newTestSeedRepository(t)

	seedPack, err := source.BuildPushPack(ctx, "main", "")
	if err != nil {
		t.Fatalf("BuildPushPack (seed): %v", err)
	}
	commitTestMutation(t, source, "pull-node-1", "First", "alice", "first commit")
	pack, err := source.BuildPushPack(ctx, "main", seedPack.TargetCommit)
	if err != nil {
		t.Fatalf("BuildPushPack: %v", err)
	}

	var frame pushPackFrame
	if err := cbor.Unmarshal(pack.PackData, &frame); err != nil {
		t.Fatalf("decode pack: %v", err)
	}
	// Declare a bogus target that does not match the recomputed wire ID of
	// the commit actually carried in the pack's Commits list.
	frame.Target.ID = frame.Target.ID + "-tampered"
	tampered, err := pushCanonicalCBOR.Marshal(frame)
	if err != nil {
		t.Fatalf("re-marshal tampered pack: %v", err)
	}

	_, err = dest.InstallPullPack(ctx, "main", [][]byte{tampered}, pack.TargetCommit)
	if err == nil {
		t.Fatal("expected an error for tampered pack target, got nil")
	}
}

func TestInstallPullPackNoPacksIsNoOp(t *testing.T) {
	ctx := context.Background()
	dest := newTestSeedRepository(t)
	head := dest.branches["main"]

	result, err := dest.InstallPullPack(ctx, "main", nil, "")
	if err != nil {
		t.Fatalf("InstallPullPack: %v", err)
	}
	if result.CommitsInstalled != 0 {
		t.Fatalf("CommitsInstalled = %d, want 0", result.CommitsInstalled)
	}
	if result.HeadCommit != head {
		t.Fatalf("HeadCommit = %q, want unchanged %q", result.HeadCommit, head)
	}
}

func TestInstallPullPackUnknownBranch(t *testing.T) {
	dest := newTestSeedRepository(t)
	_, err := dest.InstallPullPack(context.Background(), "does-not-exist", nil, "")
	if err != ErrBranchNotFound {
		t.Fatalf("err = %v, want ErrBranchNotFound", err)
	}
}
