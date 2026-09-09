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
	if incremental.PackFormat != PushPackFormatV2 {
		t.Fatalf("incremental PackFormat = %d, want %d", incremental.PackFormat, PushPackFormatV2)
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

func TestBuildPushPackSupportsMergeCommits(t *testing.T) {
	repo := newTestSeedRepository(t)
	root := repo.branches["main"]
	rootCommit := repo.commits[root]

	commitTestMutation(t, repo, "push-node-1", "First", "alice", "first commit")
	c1ID := repo.branches["main"]

	// Synthesize a diverged commit c2 branching off root.
	c2Commit := rootCommit
	c2Commit.Parents = []ObjectID{root}
	c2Commit.Message = "feature branch commit"
	c2ID := repo.store("commit", c2Commit)
	repo.commits[c2ID] = c2Commit

	// Synthesize a merge commit m with parents [c1ID, c2ID].
	mergeCommit := repo.commits[c1ID]
	mergeCommit.Parents = []ObjectID{c1ID, c2ID}
	mergeCommit.Message = "merge commit"
	mergeID := repo.store("commit", mergeCommit)
	repo.commits[mergeID] = mergeCommit
	repo.branches["main"] = mergeID

	// 1. Root push includes all 4 commits (root, c1, c2, merge).
	pack, err := repo.BuildPushPack(context.Background(), "main", "")
	if err != nil {
		t.Fatalf("BuildPushPack: %v", err)
	}
	if len(pack.Commits) != 4 {
		t.Fatalf("commits = %d, want 4", len(pack.Commits))
	}

	// Map local commits to wire IDs.
	commitIndices := make(map[string]int)
	for idx, record := range pack.Commits {
		commitIndices[record.ID] = idx
	}

	mergeWireRecord := pack.Commits[len(pack.Commits)-1]
	if len(mergeWireRecord.Commit.Parents) != 2 {
		t.Fatalf("merge commit parents = %d, want 2", len(mergeWireRecord.Commit.Parents))
	}

	// Verify topological ordering: parents appear before the merge commit.
	p0 := string(mergeWireRecord.Commit.Parents[0])
	p1 := string(mergeWireRecord.Commit.Parents[1])
	if commitIndices[p0] >= commitIndices[mergeWireRecord.ID] {
		t.Fatalf("parent 0 index %d not before merge index %d", commitIndices[p0], commitIndices[mergeWireRecord.ID])
	}
	if commitIndices[p1] >= commitIndices[mergeWireRecord.ID] {
		t.Fatalf("parent 1 index %d not before merge index %d", commitIndices[p1], commitIndices[mergeWireRecord.ID])
	}

	// Verify decoded CBOR pack frame.
	var decoded pushPackFrame
	if err := cbor.Unmarshal(pack.PackData, &decoded); err != nil {
		t.Fatalf("decode pack: %v", err)
	}
	decodedMerge := decoded.Commits[len(decoded.Commits)-1]
	if len(decodedMerge.Parents) != 2 {
		t.Fatalf("decoded merge commit parents = %d, want 2", len(decodedMerge.Parents))
	}
	if decodedMerge.Parents[0].ID != p0 || decodedMerge.Parents[1].ID != p1 {
		t.Fatalf("decoded merge parents = [%q, %q], want [%q, %q]",
			decodedMerge.Parents[0].ID, decodedMerge.Parents[1].ID, p0, p1)
	}

	// 2. Incremental push where baseCommit is c1 wire ID:
	// Only c2 and merge commit should be pushed, since root and c1 are already on remote.
	c1WireID := p0
	incPack, err := repo.BuildPushPack(context.Background(), "main", c1WireID)
	if err != nil {
		t.Fatalf("BuildPushPack (incremental): %v", err)
	}
	if len(incPack.Commits) != 2 {
		t.Fatalf("incremental commits = %d, want 2 (c2 and merge)", len(incPack.Commits))
	}
	if incPack.BaseCommit != c1WireID {
		t.Fatalf("incremental BaseCommit = %q, want %q", incPack.BaseCommit, c1WireID)
	}
	if incPack.TargetCommit != mergeWireRecord.ID {
		t.Fatalf("incremental TargetCommit = %q, want %q", incPack.TargetCommit, mergeWireRecord.ID)
	}
	if pack.PackFormat != PushPackFormatV3 {
		t.Fatalf("PackFormat = %d, want %d", pack.PackFormat, PushPackFormatV3)
	}
	if incPack.PackFormat != PushPackFormatV3 {
		t.Fatalf("incPack.PackFormat = %d, want %d", incPack.PackFormat, PushPackFormatV3)
	}
}
