package repository

import (
	"errors"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository/branch"
)

func TestMergeNodeCombinesIndependentFields(t *testing.T) {
	base := Node{
		ID: "node", Title: "base", Labels: []string{"Base"},
		Properties: map[string]PropertyValue{"priority": IntegerPropertyValue(1)},
	}
	source := base.clone()
	source.Title = "source title"
	target := base.clone()
	target.Properties["priority"] = IntegerPropertyValue(2)
	target.Properties["owner"] = StringPropertyValue("target")

	var conflicts []MergeConflict
	merged, present := mergeNode("node", base, source, target, true, true, true, &conflicts)
	if !present {
		t.Fatal("merged node is absent")
	}
	if merged.Title != source.Title {
		t.Fatalf("title = %q, want %q", merged.Title, source.Title)
	}
	if !merged.Properties["priority"].Equal(target.Properties["priority"]) || !merged.Properties["owner"].Equal(target.Properties["owner"]) {
		t.Fatalf("properties = %#v, want target properties", merged.Properties)
	}
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %#v", conflicts)
	}
}

func TestMergeNodeReportsOverlappingPropertyConflict(t *testing.T) {
	base := Node{ID: "node", Title: "base", Properties: map[string]PropertyValue{"priority": IntegerPropertyValue(1)}}
	source, target := base.clone(), base.clone()
	source.Properties["priority"] = IntegerPropertyValue(2)
	target.Properties["priority"] = IntegerPropertyValue(3)

	var conflicts []MergeConflict
	_, _ = mergeNode("node", base, source, target, true, true, true, &conflicts)
	if len(conflicts) != 1 || conflicts[0].Category != "structural" || conflicts[0].Entity != "node" ||
		conflicts[0].ID != "node" || conflicts[0].Field != "properties.priority" {
		t.Fatalf("conflicts = %#v", conflicts)
	}
}

func TestPreviewMergeAppliesSourceGraphAndRejectsStalePreview(t *testing.T) {
	repo := newTestSeedRepository(t)
	if _, err := repo.CreateBranch("feature", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	if _, err := repo.StageMutationBatch(StageMutationRequest{
		Branch:     "feature",
		Operations: []MutationOperation{{Action: "add", Entity: "node", ID: "feature-node", Title: "Feature"}},
	}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	if _, err := repo.CommitStagedMutations("feature"); err != nil {
		t.Fatalf("CommitStagedMutations: %v", err)
	}

	preview, err := repo.PreviewMerge("feature", "main")
	if err != nil {
		t.Fatalf("PreviewMerge: %v", err)
	}
	if !preview.Clean || len(preview.Changes) != 1 || preview.Changes[0] != (MergeChange{Entity: "node", ID: "feature-node", Change: "added"}) {
		t.Fatalf("preview = %#v", preview)
	}
	if _, err := repo.AdvanceBranch("main"); err != nil {
		t.Fatalf("AdvanceBranch: %v", err)
	}
	if _, err := repo.ApplyMergePreview("feature", "main", "merge", preview.ID, "alice", "merge feature"); !errors.Is(err, ErrMergePreviewMismatch) {
		t.Fatalf("ApplyMergePreview stale error = %v, want ErrMergePreviewMismatch", err)
	}

	preview, err = repo.PreviewMerge("feature", "main")
	if err != nil {
		t.Fatalf("PreviewMerge refreshed: %v", err)
	}
	if _, err := repo.ApplyMergePreview("feature", "main", "merge", preview.ID, "alice", "merge feature"); err != nil {
		t.Fatalf("ApplyMergePreview: %v", err)
	}
	head, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}
	if _, err := repo.ResolvePinned(head, "feature-node"); err != nil {
		t.Fatalf("merged node missing: %v", err)
	}
}

func TestApplyMergePreviewPreservesActiveConflictTransactionLease(t *testing.T) {
	repo := newTestSeedRepository(t)
	base, source, target := createDivergedBranchHeads(repo)
	binding := MergePreviewBinding{MergeBase: base, SourceCommit: source, TargetCommit: target}
	if err := repo.ApplyConflictedBoundMerge("feature", "main", "owner", binding); !errors.Is(err, ErrMergeConflicted) {
		t.Fatalf("ApplyConflictedBoundMerge: %v", err)
	}

	preview, err := repo.PreviewMerge("feature", "main")
	if err != nil {
		t.Fatalf("PreviewMerge: %v", err)
	}
	if _, err := repo.ApplyMergePreview("feature", "main", "owner", preview.ID, "", ""); !errors.Is(err, ErrMergeTargetLeaseHeld) {
		t.Fatalf("ApplyMergePreview error = %v, want ErrMergeTargetLeaseHeld", err)
	}
	if _, err := repo.AdvanceBranch("main"); !errors.Is(err, ErrMergeTargetLeaseHeld) {
		t.Fatalf("AdvanceBranch error = %v, want ErrMergeTargetLeaseHeld", err)
	}
}
