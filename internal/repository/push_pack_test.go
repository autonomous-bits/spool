package repository

import (
	"context"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
)

func commitTestMutation(t *testing.T, repo *Repository, id, title, author, message string) {
	t.Helper()
	if _, err := repo.StageMutationBatch(StageMutationRequest{
		Branch: "main",
		Operations: []MutationOperation{
			{Action: "add", Entity: "node", ID: id, Title: title, Labels: []string{"Fixture"}},
		},
	}); err != nil {
		t.Fatalf("stage %s: %v", id, err)
	}
	if _, err := repo.CommitStagedMutationBatch(CommitStagedMutationRequest{
		Branch: "main", Author: author, Message: message,
	}); err != nil {
		t.Fatalf("commit %s: %v", id, err)
	}
}

func TestBuildPushPackRootPushIncludesEntireHistory(t *testing.T) {
	repo := newTestSeedRepository(t)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	repo.now = func() time.Time { return now }
	commitTestMutation(t, repo, "push-node-1", "First", "alice", "first commit")
	commitTestMutation(t, repo, "push-node-2", "Second", "bob", "second commit")

	pack, err := repo.BuildPushPack(context.Background(), "main", "")
	if err != nil {
		t.Fatalf("BuildPushPack: %v", err)
	}
	// Seed commit + first commit + second commit.
	if len(pack.Commits) != 3 {
		t.Fatalf("commits = %d, want 3: %#v", len(pack.Commits), pack.Commits)
	}
	if len(pack.Objects) != 3 {
		t.Fatalf("objects = %d, want 3", len(pack.Objects))
	}
	if pack.BaseCommit != "" {
		t.Fatalf("BaseCommit = %q, want empty", pack.BaseCommit)
	}
	if pack.TargetCommit != pack.Commits[len(pack.Commits)-1].ID {
		t.Fatalf("TargetCommit = %q, want last commit ID %q", pack.TargetCommit, pack.Commits[len(pack.Commits)-1].ID)
	}
	if pack.PackFormat != PushPackFormatV2 {
		t.Fatalf("PackFormat = %d, want %d", pack.PackFormat, PushPackFormatV2)
	}
	if got := pushContentID(pack.PackData); got != pack.PackHash {
		t.Fatalf("PackHash = %q, want content ID %q", pack.PackHash, got)
	}
	// Root commit must have no parents; every later commit's first wire
	// parent must be the previous commit's wire ID.
	var decoded pushPackFrame
	if err := cbor.Unmarshal(pack.PackData, &decoded); err != nil {
		t.Fatalf("decode pack: %v", err)
	}
	if len(decoded.Commits[0].Parents) != 0 {
		t.Fatalf("root commit frame parents = %#v, want none", decoded.Commits[0].Parents)
	}
	for i := 1; i < len(decoded.Commits); i++ {
		if decoded.Commits[i].Parents[0].ID != pack.Commits[i-1].ID {
			t.Fatalf("commit %d parent = %q, want %q", i, decoded.Commits[i].Parents[0].ID, pack.Commits[i-1].ID)
		}
	}
}

func TestBuildPushPackIncrementalPushOnlyIncludesNewCommits(t *testing.T) {
	repo := newTestSeedRepository(t)
	commitTestMutation(t, repo, "push-node-1", "First", "alice", "first commit")

	base, err := repo.BuildPushPack(context.Background(), "main", "")
	if err != nil {
		t.Fatalf("BuildPushPack (root): %v", err)
	}

	commitTestMutation(t, repo, "push-node-2", "Second", "bob", "second commit")

	incremental, err := repo.BuildPushPack(context.Background(), "main", base.TargetCommit)
	if err != nil {
		t.Fatalf("BuildPushPack (incremental): %v", err)
	}
	if len(incremental.Commits) != 1 {
		t.Fatalf("incremental commits = %d, want 1: %#v", len(incremental.Commits), incremental.Commits)
	}
	if incremental.BaseCommit != base.TargetCommit {
		t.Fatalf("BaseCommit = %q, want %q", incremental.BaseCommit, base.TargetCommit)
	}
	if incremental.Commits[0].ID == base.TargetCommit {
		t.Fatal("incremental push resent the already-known base commit")
	}

	// Recomputation is deterministic: rebuilding the full root pack after
	// both commits must reach the same target the incremental push did.
	again, err := repo.BuildPushPack(context.Background(), "main", "")
	if err != nil {
		t.Fatalf("BuildPushPack (root, again): %v", err)
	}
	if again.TargetCommit != incremental.TargetCommit {
		t.Fatalf("recomputed target = %q, want stable %q", again.TargetCommit, incremental.TargetCommit)
	}
	if len(again.Commits) != 3 {
		t.Fatalf("recomputed root commits = %d, want 3", len(again.Commits))
	}
	if again.Commits[1].ID != base.TargetCommit {
		t.Fatalf("recomputed root commit[1] = %q, want stable %q", again.Commits[1].ID, base.TargetCommit)
	}
}

func TestBuildPushPackNothingToPush(t *testing.T) {
	repo := newTestSeedRepository(t)
	commitTestMutation(t, repo, "push-node-1", "First", "alice", "first commit")

	pack, err := repo.BuildPushPack(context.Background(), "main", "")
	if err != nil {
		t.Fatalf("BuildPushPack: %v", err)
	}
	if _, err := repo.BuildPushPack(context.Background(), "main", pack.TargetCommit); err != ErrNothingToPush {
		t.Fatalf("err = %v, want ErrNothingToPush", err)
	}
}

func TestBuildPushPackUnknownBaseCommit(t *testing.T) {
	repo := newTestSeedRepository(t)
	commitTestMutation(t, repo, "push-node-1", "First", "alice", "first commit")

	if _, err := repo.BuildPushPack(context.Background(), "main", "not-a-real-commit"); err != ErrPushBaseNotFound {
		t.Fatalf("err = %v, want ErrPushBaseNotFound", err)
	}
}

func TestBuildPushPackUnknownBranch(t *testing.T) {
	repo := newTestSeedRepository(t)
	if _, err := repo.BuildPushPack(context.Background(), "does-not-exist", ""); err != ErrBranchNotFound {
		t.Fatalf("err = %v, want ErrBranchNotFound", err)
	}
}

func TestBuildPushPackRejectsMergeCommits(t *testing.T) {
	repo := newTestSeedRepository(t)
	commitTestMutation(t, repo, "push-node-1", "First", "alice", "first commit")

	// Synthesize a merge commit directly: a real two-parent commit requires
	// the full branch/preview/apply merge lifecycle, which is unrelated to
	// what BuildPushPack itself needs to reject. It only inspects
	// commit.Parents before touching a commit's snapshot, so a minimal
	// fixture commit exercises the same guard.
	root := repo.branches["main"]
	rootCommit := repo.commits[root]
	mergeCommit := rootCommit
	mergeCommit.Parents = []ObjectID{root, root}
	mergeCommit.Message = "synthetic merge commit"
	mergeID := repo.store("commit", mergeCommit)
	repo.commits[mergeID] = mergeCommit
	repo.branches["main"] = mergeID

	if _, err := repo.BuildPushPack(context.Background(), "main", ""); err != ErrPushMergeCommitUnsupported {
		t.Fatalf("err = %v, want ErrPushMergeCommitUnsupported", err)
	}
}
