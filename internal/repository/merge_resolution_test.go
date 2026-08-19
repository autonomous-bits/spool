package repository

import (
	"errors"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository/branch"
)

func TestResolveConflictedMergeAppliesSelectedSourceAndFinalizes(t *testing.T) {
	repo := NewSeedRepository()
	if _, err := repo.CreateBranch("feature", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	for _, change := range []struct {
		branch string
		title  string
	}{
		{branch: "feature", title: "source title"},
		{branch: "main", title: "target title"},
	} {
		if _, err := repo.StageMutationBatch(StageMutationRequest{
			Branch: change.branch,
			Operations: []MutationOperation{{
				Action: "update", Entity: "node", ID: SeedNodeID, Title: change.title,
			}},
		}); err != nil {
			t.Fatalf("StageMutationBatch %s: %v", change.branch, err)
		}
		if _, err := repo.CommitStagedMutations(change.branch); err != nil {
			t.Fatalf("CommitStagedMutations %s: %v", change.branch, err)
		}
	}

	preview, err := repo.PreviewMerge("feature", "main")
	if err != nil {
		t.Fatalf("PreviewMerge: %v", err)
	}
	if preview.Clean || len(preview.Conflicts) != 1 {
		t.Fatalf("preview = %#v, want one conflict", preview)
	}
	if _, err := repo.ApplyMergePreview("feature", "main", "merge-1", preview.ID, "", ""); !errors.Is(err, ErrMergeConflicted) {
		t.Fatalf("ApplyMergePreview error = %v, want ErrMergeConflicted", err)
	}
	if err := repo.ResolveConflictedMerge(ResolveConflictedMergeRequest{
		TargetBranch: "main", TransactionID: "merge-1", PreviewID: preview.ID,
		Selections: []MergeResolutionSelection{{ConflictID: preview.Conflicts[0].ConflictID, Choice: "source"}},
	}); err != nil {
		t.Fatalf("ResolveConflictedMerge: %v", err)
	}
	status, err := repo.InspectMergeTransaction("main", "merge-1")
	if err != nil {
		t.Fatalf("InspectMergeTransaction: %v", err)
	}
	if !status.Resolved || !status.Restaged {
		t.Fatalf("status = %#v, want resolved and restaged", status)
	}
	merged, err := repo.FinalizeMergeTransaction("main", "merge-1")
	if err != nil {
		t.Fatalf("FinalizeMergeTransaction: %v", err)
	}
	node, err := repo.ResolvePinned(merged, SeedNodeID)
	if err != nil {
		t.Fatalf("ResolvePinned: %v", err)
	}
	if node.Node.Title != "source title" {
		t.Fatalf("merged title = %q, want source title", node.Node.Title)
	}
}
