package repository

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/autonomous-bits/spool/internal/repository/branch"
)

func TestHistoryReturnsCommitMetadataAndEntityChanges(t *testing.T) {
	repo := NewSeedRepository()
	now := time.Date(2026, 8, 17, 18, 0, 0, 0, time.UTC)
	repo.now = func() time.Time { return now }
	if _, err := repo.StageMutationBatch(StageMutationRequest{
		Branch: "main",
		Operations: []MutationOperation{
			{Action: "update", Entity: "node", ID: SeedNodeID, Title: "Updated seed"},
			{Action: "add", Entity: "edge", ID: "edge-1", Source: SeedNodeID, Target: SeedNodeID},
		},
	}); err != nil {
		t.Fatalf("stage: %v", err)
	}
	committed, err := repo.CommitStagedMutationBatch(CommitStagedMutationRequest{
		Branch: "main", Author: "alice", Message: "update seed",
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}

	result, err := repo.History(HistoryRequest{
		Commit: repo.branches["main"], EntityID: SeedNodeID,
	})
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if result.Entries[0].Commit != committed.Commit || result.Entries[0].Author != "alice" ||
		!result.Entries[0].Time.Equal(now) || result.Entries[0].Message != "update seed" {
		t.Fatalf("metadata = %#v", result.Entries[0])
	}
	if !reflect.DeepEqual(result.Entries[0].ChangedFields, []string{"title"}) ||
		len(result.Entries[0].EdgeAdditions) != 1 || result.Entries[0].BeforeSnapshot == "" {
		t.Fatalf("changes = %#v", result.Entries[0])
	}
	if len(result.Entries) != 2 {
		t.Fatalf("history entries = %#v", result.Entries)
	}
	if _, err := repo.StageMutationBatch(StageMutationRequest{
		Branch: "main", Operations: []MutationOperation{{Action: "add", Entity: "node", ID: "unrelated", Title: "Unrelated"}},
	}); err != nil {
		t.Fatalf("stage unrelated: %v", err)
	}
	if _, err := repo.CommitStagedMutations("main"); err != nil {
		t.Fatalf("commit unrelated: %v", err)
	}
	result, err = repo.History(HistoryRequest{Commit: repo.branches["main"], EntityID: SeedNodeID})
	if err != nil {
		t.Fatalf("History after unrelated commit: %v", err)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("unrelated commit included in history: %#v", result.Entries)
	}
}

func TestHistoryAllParentsAndContainmentTraverseMergedHistory(t *testing.T) {
	repo := NewSeedRepository()
	base, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("pin base: %v", err)
	}
	if _, err := repo.CreateBranch("feature", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("create feature: %v", err)
	}
	if _, err := repo.StageMutationBatch(StageMutationRequest{
		Branch: "feature", Operations: []MutationOperation{{Action: "add", Entity: "node", ID: "feature-node", Title: "Feature"}},
	}); err != nil {
		t.Fatalf("stage feature: %v", err)
	}
	feature, err := repo.CommitStagedMutations("feature")
	if err != nil {
		t.Fatalf("commit feature: %v", err)
	}
	main, err := repo.AdvanceBranch("main")
	if err != nil {
		t.Fatalf("advance main: %v", err)
	}
	if _, err := repo.ApplyCleanBoundMerge("feature", "main", "merge", MergePreviewBinding{
		MergeBase: base, SourceCommit: feature.Commit, TargetCommit: main,
	}); err != nil {
		t.Fatalf("merge: %v", err)
	}

	firstParent, err := repo.History(HistoryRequest{Commit: repo.branches["main"], EntityID: "feature-node"})
	if err != nil {
		t.Fatalf("first-parent history: %v", err)
	}
	if len(firstParent.Entries) != 1 {
		t.Fatalf("first-parent history = %#v", firstParent)
	}
	history, err := repo.History(HistoryRequest{
		Commit: repo.branches["main"], EntityID: "feature-node", AllParents: true,
	})
	if err != nil {
		t.Fatalf("all-parent history: %v", err)
	}
	if len(history.Entries) != 2 || history.Entries[0].Commit != firstParent.Entries[0].Commit || history.Entries[1].Commit != feature.Commit {
		t.Fatalf("history = %#v", history)
	}
	contained, err := repo.BranchesContaining(ContainmentSelector{EntityID: "feature-node"})
	if err != nil {
		t.Fatalf("BranchesContaining: %v", err)
	}
	if !reflect.DeepEqual(contained.Branches, []string{"feature", "main"}) {
		t.Fatalf("branches = %#v", contained.Branches)
	}
	if result, err := repo.BranchesContaining(ContainmentSelector{NaturalKey: "feature"}); err != nil || len(result.Branches) != 0 {
		t.Fatalf("natural key result/error = %#v/%v", result, err)
	}
}

func TestBranchesContainingMatchesSnapshotsAndRejectsAmbiguousSelectors(t *testing.T) {
	repo := NewSeedRepository()
	head, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("pin branch: %v", err)
	}

	snapshot := repo.commits[head].Snapshot
	result, err := repo.BranchesContaining(ContainmentSelector{SnapshotID: snapshot})
	if err != nil || !reflect.DeepEqual(result.Branches, []string{"main"}) {
		t.Fatalf("snapshot result/error = %#v/%v", result, err)
	}
	if _, err := repo.BranchesContaining(ContainmentSelector{EntityID: SeedNodeID, SnapshotID: snapshot}); !errors.Is(err, ErrInvalidContainmentSelector) {
		t.Fatalf("ambiguous selector error = %v", err)
	}
}

func TestHistoryRepresentsEdgeUpdatesAsRemovalAndAddition(t *testing.T) {
	repo := NewSeedRepository()
	if _, err := repo.StageMutationBatch(StageMutationRequest{
		Branch: "main",
		Operations: []MutationOperation{
			{Action: "add", Entity: "node", ID: "node-2", Title: "Second"},
			{Action: "add", Entity: "edge", ID: "edge-1", Source: SeedNodeID, Target: "node-2"},
		},
	}); err != nil {
		t.Fatalf("stage graph: %v", err)
	}
	if _, err := repo.CommitStagedMutations("main"); err != nil {
		t.Fatalf("commit graph: %v", err)
	}
	if _, err := repo.StageMutationBatch(StageMutationRequest{
		Branch: "main", Operations: []MutationOperation{{Action: "update", Entity: "edge", ID: "edge-1", Source: "node-2", Target: SeedNodeID}},
	}); err != nil {
		t.Fatalf("stage edge update: %v", err)
	}
	if _, err := repo.CommitStagedMutations("main"); err != nil {
		t.Fatalf("commit edge update: %v", err)
	}
	history, err := repo.History(HistoryRequest{Commit: repo.branches["main"], EntityID: SeedNodeID})
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history.Entries) != 3 || len(history.Entries[0].EdgeAdditions) != 1 || len(history.Entries[0].EdgeRemovals) != 1 {
		t.Fatalf("history = %#v", history)
	}
}
