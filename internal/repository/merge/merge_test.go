package merge

import (
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/repository/branch"
)

func TestServiceAppliesCleanMergeThroughRepositoryContract(t *testing.T) {
	repo := repository.NewSeedRepository()
	if _, err := repo.CreateBranch("feature", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch feature: %v", err)
	}
	source, err := repo.AdvanceBranch("feature")
	if err != nil {
		t.Fatalf("AdvanceBranch feature: %v", err)
	}
	target, err := repo.AdvanceBranch("main")
	if err != nil {
		t.Fatalf("AdvanceBranch main: %v", err)
	}

	merged, err := NewService(repo).ApplyClean("feature", "main", "test-transaction", PreviewBinding{
		MergeBase: source, SourceCommit: source, TargetCommit: target,
	})
	if err != nil {
		t.Fatalf("ApplyClean: %v", err)
	}
	head, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch merged main: %v", err)
	}
	if head != merged {
		t.Fatalf("main head = %q, want merged commit %q", head, merged)
	}
}
