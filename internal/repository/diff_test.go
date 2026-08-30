package repository

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/autonomous-bits/spool/internal/repository/branch"
)

func TestDiffReturnsCanonicalDirectionalChangesFiltersAndContext(t *testing.T) {
	repo := newTestSeedRepository(t)
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
		Base: base, Target: target.Commit,
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
		Base: base, Target: target.Commit,
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
		Base: base, Target: target.Commit,
		Filter: DiffFilter{NodeTitleSubstr: "Second"}, MaxRows: 10, MaxResponseBytes: 1 << 20,
	})
	if err != nil {
		t.Fatalf("title Diff: %v", err)
	}
	if len(titleOnly.Changes) != 1 || titleOnly.Changes[0].Entity != "node" {
		t.Fatalf("title filter included edges: %#v", titleOnly.Changes)
	}
}

func TestDiffRequiresExistingPinnedCommits(t *testing.T) {
	repo := newTestSeedRepository(t)
	if _, err := repo.CreateBranch("feature", branchSource("main")); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	feature, err := repo.AdvanceBranch("feature")
	if err != nil {
		t.Fatalf("advance feature: %v", err)
	}
	result, err := repo.Diff(DiffRequest{
		Base: repo.branches["main"], Target: feature,
		MaxRows: 10, MaxResponseBytes: 1 << 20,
	})
	if err != nil {
		t.Fatalf("Diff global commit: %v", err)
	}
	if result.TargetCommit != feature {
		t.Fatalf("target = %q, want %q", result.TargetCommit, feature)
	}
	if _, err := repo.Diff(DiffRequest{
		Base: "missing", Target: repo.branches["main"], MaxRows: 1, MaxResponseBytes: 1024,
	}); !errors.Is(err, ErrCommitNotFound) {
		t.Fatalf("missing pinned commit error = %v", err)
	}
}

func TestDiffPaginatesWithinRowsAndBudgetAndBindsContinuation(t *testing.T) {
	repo := newTestSeedRepository(t)
	base, _ := repo.PinBranch("main")
	if _, err := repo.StageMutationBatch(StageMutationRequest{Branch: "main", Operations: validMutationBatch()}); err != nil {
		t.Fatalf("stage: %v", err)
	}

	target, err := repo.CommitStagedMutations("main")
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	request := DiffRequest{
		Base: base, Target: target.Commit,
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
	request.MaxRows = 1
	request.IncludeOneHop = true
	if _, err := repo.Diff(request); !errors.Is(err, ErrInvalidContinuation) {
		t.Fatalf("one-hop mismatched continuation error = %v", err)
	}
	if _, err := repo.Diff(DiffRequest{
		Base: base, Target: target.Commit,
		MaxRows: 1, MaxResponseBytes: 1,
	}); !errors.Is(err, ErrResponseBudgetTooSmall) {
		t.Fatalf("small budget error = %v", err)
	}
	if _, err := repo.Diff(DiffRequest{
		Base: base, Target: target.Commit,
		MaxRows: 0, MaxResponseBytes: 1 << 20,
	}); !errors.Is(err, ErrInvalidDiffBudget) {
		t.Fatalf("zero row budget error = %v", err)
	}
}

func TestDiffContextStopsAtOneHop(t *testing.T) {
	repo := newTestSeedRepository(t)
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
		Base: base, Target: target.Commit,
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

func TestDiffContextHonorsCanceledContext(t *testing.T) {
	repo := newTestSeedRepository(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := repo.DiffContext(ctx, DiffRequest{
		Base: repo.branches["main"], Target: repo.branches["main"], MaxRows: 1, MaxResponseBytes: 1024,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("DiffContext error = %v, want context.Canceled", err)
	}
}

func TestDiffContextReturnsDeterministicPrefixWhenDeadlineFiresDuringPaging(t *testing.T) {
	repo := newTestSeedRepository(t)
	base, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch base: %v", err)
	}
	if _, err := repo.StageMutationBatch(StageMutationRequest{Branch: "main", Operations: validMutationBatch()}); err != nil {
		t.Fatalf("stage: %v", err)
	}
	target, err := repo.CommitStagedMutations("main")
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	request := DiffRequest{Base: base, Target: target.Commit, MaxRows: 2, MaxResponseBytes: 1 << 20}

	for checks := 1; checks < 100; checks++ {
		ctx := &deadlineAfterChecks{remaining: checks}
		result, err := repo.DiffContext(ctx, request)
		if !errors.Is(err, context.DeadlineExceeded) || len(result.Changes) == 0 {
			continue
		}
		if result.ContinuationToken == "" || len(result.Changes) >= 2 {
			t.Fatalf("deadline page = %#v, want a strict prefix with continuation", result)
		}
		return
	}
	t.Fatal("could not trigger a deadline after a deterministic diff prefix")
}

func TestDiffContextReturnsPrefixWhenDeadlineFiresDuringChangeScan(t *testing.T) {
	const additions = 32
	repo := newTestSeedRepository(t)
	base, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch base: %v", err)
	}
	operations := make([]MutationOperation, 0, additions)
	for i := 0; i < additions; i++ {
		id := fmt.Sprintf("node-%02d", i)
		operations = append(operations, MutationOperation{Action: "add", Entity: "node", ID: id, Title: id})
	}
	if _, err := repo.StageMutationBatch(StageMutationRequest{Branch: "main", Operations: operations}); err != nil {
		t.Fatalf("stage additions: %v", err)
	}
	target, err := repo.CommitStagedMutations("main")
	if err != nil {
		t.Fatalf("commit additions: %v", err)
	}
	request := DiffRequest{
		Base: base, Target: target.Commit, MaxRows: additions, MaxResponseBytes: 1 << 20,
	}
	result, err := repo.DiffContext(&deadlineAfterChecks{remaining: additions + 5}, request)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("DiffContext deadline error = %v, want context.DeadlineExceeded", err)
	}
	if len(result.Changes) != 1 || result.Changes[0].ID != "node-00" || result.ContinuationToken == "" {
		t.Fatalf("deadline diff prefix = %#v, want node-00 with continuation", result)
	}
	request.ContinuationToken = result.ContinuationToken
	continued, err := repo.DiffContext(context.Background(), request)
	if err != nil || len(continued.Changes) == 0 || continued.Changes[0].ID != "node-01" {
		t.Fatalf("continued deadline diff = %#v/%v, want node-01 prefix", continued, err)
	}
}

type deadlineAfterChecks struct {
	remaining int
}

func (c *deadlineAfterChecks) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *deadlineAfterChecks) Done() <-chan struct{}       { return nil }
func (c *deadlineAfterChecks) Err() error {
	c.remaining--
	if c.remaining < 0 {
		return context.DeadlineExceeded
	}
	return nil
}
func (c *deadlineAfterChecks) Value(any) any { return nil }

func TestDiffTitleFilterIncludesMatchingRemovedNodes(t *testing.T) {
	repo := newTestSeedRepository(t)
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
		Base: base, Target: target.Commit,
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
