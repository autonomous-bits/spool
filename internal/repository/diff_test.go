package repository

import (
	"errors"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository/branch"
)

func TestDiffReturnsCanonicalDirectionalChangesFiltersAndContext(t *testing.T) {
	repo := NewSeedRepository()
	base, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("pin base: %v", err)
	}
	if _, err := repo.StageMutationBatch(StageMutationRequest{Branch: "main", Operations: validMutationBatch()}); err != nil {
		t.Fatalf("stage: %v", err)
	}
	target, err := repo.CommitStagedMutations("main")
	if err != nil {
		t.Fatalf("commit: %v", err)
	}

	result, err := repo.Diff(DiffRequest{
		Base: DiffSelector{Commit: string(base)}, Target: DiffSelector{Commit: string(target.Commit)},
		MaxRows: 10, MaxResponseBytes: 1 << 20, IncludeOneHop: true,
	})
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if result.BaseCommit != base || result.TargetCommit != target.Commit {
		t.Fatalf("commits = %#v", result)
	}
	if len(result.Changes) != 2 ||
		result.Changes[0].Entity != "node" || result.Changes[0].Change != "added" || result.Changes[0].ID != "node-2" ||
		result.Changes[1].Entity != "edge" || result.Changes[1].Change != "added" || result.Changes[1].ID != "edge-1" {
		t.Fatalf("changes = %#v", result.Changes)
	}
	if len(result.Context) != 1 || result.Context[0].Entity != "node" || result.Context[0].ID != SeedNodeID {
		t.Fatalf("context = %#v", result.Context)
	}

	filtered, err := repo.Diff(DiffRequest{
		Base: DiffSelector{Commit: string(base)}, Target: DiffSelector{Commit: string(target.Commit)},
		Filter:  DiffFilter{NodeIDs: []string{"node-2"}, NodeTitleSubstr: "Second"},
		MaxRows: 10, MaxResponseBytes: 1 << 20,
	})
	if err != nil {
		t.Fatalf("filtered Diff: %v", err)
	}
	if len(filtered.Changes) != 1 || filtered.Changes[0].ID != "node-2" {
		t.Fatalf("filtered changes = %#v", filtered.Changes)
	}
	titleOnly, err := repo.Diff(DiffRequest{
		Base: DiffSelector{Commit: string(base)}, Target: DiffSelector{Commit: string(target.Commit)},
		Filter: DiffFilter{NodeTitleSubstr: "Second"}, MaxRows: 10, MaxResponseBytes: 1 << 20,
	})
	if err != nil {
		t.Fatalf("title Diff: %v", err)
	}
	if len(titleOnly.Changes) != 1 || titleOnly.Changes[0].Entity != "node" {
		t.Fatalf("title filter included edges: %#v", titleOnly.Changes)
	}
}

func TestDiffPinsBranchResolvesGlobalCommitsAndValidatesSelectors(t *testing.T) {
	repo := NewSeedRepository()
	if _, err := repo.CreateBranch("feature", branchSource("main")); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	feature, err := repo.AdvanceBranch("feature")
	if err != nil {
		t.Fatalf("advance feature: %v", err)
	}
	result, err := repo.Diff(DiffRequest{
		Base: DiffSelector{Branch: "main"}, Target: DiffSelector{Commit: string(feature)},
		MaxRows: 10, MaxResponseBytes: 1 << 20,
	})
	if err != nil {
		t.Fatalf("Diff global commit: %v", err)
	}
	if result.TargetCommit != feature {
		t.Fatalf("target = %q, want %q", result.TargetCommit, feature)
	}
	for _, selector := range []DiffSelector{{}, {Branch: "main", Commit: string(feature)}} {
		_, err := repo.Diff(DiffRequest{Base: selector, Target: DiffSelector{Branch: "main"}, MaxRows: 1, MaxResponseBytes: 1024})
		if !errors.Is(err, ErrInvalidDiffSelector) {
			t.Fatalf("selector %#v error = %v", selector, err)
		}
	}
}

func TestDiffPaginatesWithinRowsAndBudgetAndBindsContinuation(t *testing.T) {
	repo := NewSeedRepository()
	base, _ := repo.PinBranch("main")
	if _, err := repo.StageMutationBatch(StageMutationRequest{Branch: "main", Operations: validMutationBatch()}); err != nil {
		t.Fatalf("stage: %v", err)
	}

	target, err := repo.CommitStagedMutations("main")
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	request := DiffRequest{
		Base: DiffSelector{Commit: string(base)}, Target: DiffSelector{Commit: string(target.Commit)},
		MaxRows: 1, MaxResponseBytes: 1 << 20,
	}
	first, err := repo.Diff(request)
	if err != nil {
		t.Fatalf("first Diff: %v", err)
	}
	if len(first.Changes) != 1 || first.ContinuationToken == "" {
		t.Fatalf("first page = %#v", first)
	}
	request.ContinuationToken = first.ContinuationToken
	second, err := repo.Diff(request)
	if err != nil {
		t.Fatalf("second Diff: %v", err)
	}
	if len(second.Changes) != 1 || second.ContinuationToken != "" {
		t.Fatalf("second page = %#v", second)
	}
	request.MaxRows = 2
	if _, err := repo.Diff(request); !errors.Is(err, ErrInvalidContinuation) {
		t.Fatalf("mismatched continuation error = %v", err)
	}
	if _, err := repo.Diff(DiffRequest{
		Base: DiffSelector{Commit: string(base)}, Target: DiffSelector{Commit: string(target.Commit)},
		MaxRows: 1, MaxResponseBytes: 1,
	}); !errors.Is(err, ErrResponseBudgetTooSmall) {
		t.Fatalf("small budget error = %v", err)
	}
	if _, err := repo.Diff(DiffRequest{
		Base: DiffSelector{Commit: string(base)}, Target: DiffSelector{Commit: string(target.Commit)},
		MaxRows: 0, MaxResponseBytes: 1 << 20,
	}); !errors.Is(err, ErrInvalidDiffBudget) {
		t.Fatalf("zero row budget error = %v", err)
	}
}

func TestDiffContextStopsAtOneHop(t *testing.T) {
	repo := NewSeedRepository()
	operations := []MutationOperation{
		{Action: "add", Entity: "node", ID: "node-2", Title: "Second"},
		{Action: "add", Entity: "node", ID: "node-3", Title: "Third"},
		{Action: "add", Entity: "edge", ID: "edge-1", Source: SeedNodeID, Target: "node-2"},
		{Action: "add", Entity: "edge", ID: "edge-2", Source: "node-2", Target: "node-3"},
	}
	if _, err := repo.StageMutationBatch(StageMutationRequest{Branch: "main", Operations: operations}); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if _, err := repo.CommitStagedMutations("main"); err != nil {
		t.Fatalf("commit graph: %v", err)
	}
	base, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("pin base: %v", err)
	}
	if _, err := repo.StageMutationBatch(StageMutationRequest{Branch: "main", Operations: []MutationOperation{
		{Action: "update", Entity: "node", ID: SeedNodeID, Title: "Updated seed"},
	}}); err != nil {
		t.Fatalf("stage update: %v", err)
	}
	target, err := repo.CommitStagedMutations("main")
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	result, err := repo.Diff(DiffRequest{
		Base: DiffSelector{Commit: string(base)}, Target: DiffSelector{Commit: string(target.Commit)},
		Filter: DiffFilter{NodeIDs: []string{SeedNodeID}}, IncludeOneHop: true,
		MaxRows: 10, MaxResponseBytes: 1 << 20,
	})
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	for _, context := range result.Context {
		if context.ID == "node-3" || context.ID == "edge-2" {
			t.Fatalf("second-hop context = %#v", result.Context)
		}
	}
}

func TestDiffTitleFilterIncludesMatchingRemovedNodes(t *testing.T) {
	repo := NewSeedRepository()
	base, _ := repo.PinBranch("main")
	if _, err := repo.StageMutationBatch(StageMutationRequest{Branch: "main", Operations: []MutationOperation{
		{Action: "delete", Entity: "node", ID: SeedNodeID},
	}}); err != nil {
		t.Fatalf("stage delete: %v", err)
	}
	target, err := repo.CommitStagedMutations("main")
	if err != nil {
		t.Fatalf("commit delete: %v", err)
	}
	result, err := repo.Diff(DiffRequest{
		Base: DiffSelector{Commit: string(base)}, Target: DiffSelector{Commit: string(target.Commit)},
		Filter: DiffFilter{NodeTitleSubstr: "walking skeleton"}, MaxRows: 10, MaxResponseBytes: 1 << 20,
	})
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(result.Changes) != 1 || result.Changes[0].Change != "removed" || result.Changes[0].ID != SeedNodeID {
		t.Fatalf("filtered removal = %#v", result.Changes)
	}
}

func branchSource(name string) branch.Source { return branch.Source{Branch: name} }
