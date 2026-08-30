package repository

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestCherryPickMissingArguments(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "repo")
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	defer func() { _ = repo.Close() }()

	_, err = repo.CherryPick(CherryPickRequest{
		Commit:       "",
		TargetBranch: "main",
	})
	if !errors.Is(err, ErrCherryPickCommitRequired) {
		t.Fatalf("expected ErrCherryPickCommitRequired, got %v", err)
	}

	_, err = repo.CherryPick(CherryPickRequest{
		Commit:       "some-commit",
		TargetBranch: "",
	})
	if !errors.Is(err, ErrCherryPickTargetBranchRequired) {
		t.Fatalf("expected ErrCherryPickTargetBranchRequired, got %v", err)
	}
}

func TestCherryPickTargetBranchNotFound(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "repo")
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	defer func() { _ = repo.Close() }()

	_, err = repo.CherryPick(CherryPickRequest{
		Commit:       string(repo.branches["main"]),
		TargetBranch: "non-existent",
	})
	if !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("expected ErrBranchNotFound, got %v", err)
	}
}

func TestCherryPickSourceCommitNotFound(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "repo")
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	defer func() { _ = repo.Close() }()

	_, err = repo.CherryPick(CherryPickRequest{
		Commit:       "0000000000000000000000000000000000000000000000000000000000000000",
		TargetBranch: "main",
	})
	if !errors.Is(err, ErrCommitNotFound) {
		t.Fatalf("expected ErrCommitNotFound, got %v", err)
	}
}

func TestCherryPickWithUncommittedStagedChangesFails(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "repo")
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	defer func() { _ = repo.Close() }()

	_, err = repo.StageMutationBatch(StageMutationRequest{
		Branch: "main",
		Operations: []MutationOperation{
			{Action: "add", Entity: "node", ID: "temp-1", Title: "Temp", Labels: []string{"Architecture"}},
		},
	})
	if err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}

	_, err = repo.CherryPick(CherryPickRequest{
		Commit:       string(repo.branches["main"]),
		TargetBranch: "main",
	})
	if !errors.Is(err, ErrUncommittedStagedChanges) {
		t.Fatalf("expected ErrUncommittedStagedChanges, got %v", err)
	}
}

func TestCherryPickSuccessfulTransplantationAndProvenance(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "repo")
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	defer func() { _ = repo.Close() }()

	// Create feature branch
	_, err = repo.CreateBranch("feature", BranchSource{Branch: "main"})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	// Commit 1 on feature: add node-1
	_, err = repo.StageMutationBatch(StageMutationRequest{
		Branch: "feature",
		Operations: []MutationOperation{
			{Action: "add", Entity: "node", ID: "feature-node-1", Title: "Feature Node 1", Labels: []string{"Product", "Requirement"}},
		},
	})
	if err != nil {
		t.Fatalf("StageMutationBatch feature-1: %v", err)
	}
	c1Res, err := repo.CommitStagedMutationBatch(CommitStagedMutationRequest{
		Branch:  "feature",
		Author:  "alice@example.com",
		Message: "Add feature requirement 1",
	})
	if err != nil {
		t.Fatalf("CommitStagedMutationBatch feature-1: %v", err)
	}

	// Commit 2 on feature: add node-2 (unrelated to what we will cherry-pick)
	_, err = repo.StageMutationBatch(StageMutationRequest{
		Branch: "feature",
		Operations: []MutationOperation{
			{Action: "add", Entity: "node", ID: "feature-node-2", Title: "Feature Node 2", Labels: []string{"Product", "Requirement"}},
		},
	})
	if err != nil {
		t.Fatalf("StageMutationBatch feature-2: %v", err)
	}
	_, err = repo.CommitStagedMutationBatch(CommitStagedMutationRequest{
		Branch:  "feature",
		Author:  "bob@example.com",
		Message: "Add feature requirement 2",
	})
	if err != nil {
		t.Fatalf("CommitStagedMutationBatch feature-2: %v", err)
	}

	// Also make an independent commit on main
	_, err = repo.StageMutationBatch(StageMutationRequest{
		Branch: "main",
		Operations: []MutationOperation{
			{Action: "add", Entity: "node", ID: "main-node-1", Title: "Main Node 1", Labels: []string{"Architecture", "Component"}},
		},
	})
	if err != nil {
		t.Fatalf("StageMutationBatch main: %v", err)
	}
	mainCommitBefore, err := repo.CommitStagedMutationBatch(CommitStagedMutationRequest{
		Branch:  "main",
		Author:  "charlie@example.com",
		Message: "Add main component 1",
	})
	if err != nil {
		t.Fatalf("CommitStagedMutationBatch main: %v", err)
	}

	// 1. Test Dry-Run simulation on main
	dryRunRes, err := repo.CherryPick(CherryPickRequest{
		Commit:       string(c1Res.Commit),
		TargetBranch: "main",
		DryRun:       true,
	})
	if err != nil {
		t.Fatalf("CherryPick DryRun: %v", err)
	}
	if !dryRunRes.DryRun {
		t.Fatal("expected DryRun = true")
	}
	if len(dryRunRes.Changes) != 1 || dryRunRes.Changes[0].ID != "feature-node-1" || dryRunRes.Changes[0].Change != "added" {
		t.Fatalf("unexpected dry-run changes: %+v", dryRunRes.Changes)
	}
	if len(dryRunRes.Conflicts) != 0 || len(dryRunRes.Violations) != 0 {
		t.Fatalf("expected no conflicts or violations, got conflicts=%v violations=%v", dryRunRes.Conflicts, dryRunRes.Violations)
	}
	if repo.branches["main"] != mainCommitBefore.Commit {
		t.Fatalf("expected main branch head unchanged after dry run, was %q, now %q", mainCommitBefore.Commit, repo.branches["main"])
	}

	// 2. Test Non-Dry-Run execution on main
	applyRes, err := repo.CherryPick(CherryPickRequest{
		Commit:       string(c1Res.Commit),
		TargetBranch: "main",
		DryRun:       false,
	})
	if err != nil {
		t.Fatalf("CherryPick Apply: %v", err)
	}
	if applyRes.DryRun {
		t.Fatal("expected DryRun = false")
	}
	if len(applyRes.Changes) != 1 || applyRes.Changes[0].ID != "feature-node-1" {
		t.Fatalf("unexpected applied changes: %+v", applyRes.Changes)
	}
	if applyRes.Commit == "" || applyRes.Commit == string(mainCommitBefore.Commit) {
		t.Fatalf("expected new commit hash, got %q", applyRes.Commit)
	}

	// Verify main HEAD is updated
	newMainHead := repo.branches["main"]
	if string(newMainHead) != applyRes.Commit {
		t.Fatalf("expected main HEAD %q, got %q", applyRes.Commit, newMainHead)
	}

	// Verify commit DAG: single parent on target branch (linear DAG advancement, NOT a merge)
	transplantedCommit := repo.commits[newMainHead]
	if len(transplantedCommit.Parents) != 1 || transplantedCommit.Parents[0] != mainCommitBefore.Commit {
		t.Fatalf("expected single parent %q, got %v", mainCommitBefore.Commit, transplantedCommit.Parents)
	}

	// Verify provenance in commit metadata
	if transplantedCommit.Author != "alice@example.com" {
		t.Fatalf("expected author %q, got %q", "alice@example.com", transplantedCommit.Author)
	}
	if !strings.Contains(transplantedCommit.Message, "Add feature requirement 1") || !strings.Contains(transplantedCommit.Message, string(c1Res.Commit)) {
		t.Fatalf("expected provenance metadata in commit message, got: %q", transplantedCommit.Message)
	}

	// Verify snapshot contents on main: contains seed, main-node-1, feature-node-1, but NOT feature-node-2
	snapshotID := transplantedCommit.Snapshot
	nodes := repo.projections[repo.snapshots[snapshotID].NodeRoot]
	if _, exists := nodes["feature-node-1"]; !exists {
		t.Fatal("feature-node-1 should exist on main")
	}
	if _, exists := nodes["main-node-1"]; !exists {
		t.Fatal("main-node-1 should exist on main")
	}
	if _, exists := nodes["feature-node-2"]; exists {
		t.Fatal("feature-node-2 should NOT exist on main (was isolated to commit 2)")
	}
}

func TestCherryPickReferentialIntegrityPreflightGating(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "repo")
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	defer func() { _ = repo.Close() }()

	_, err = repo.CreateBranch("feature", BranchSource{Branch: "main"})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	// Step 1 on feature: add feature-node-A
	_, err = repo.StageMutationBatch(StageMutationRequest{
		Branch: "feature",
		Operations: []MutationOperation{
			{Action: "add", Entity: "node", ID: "feature-node-A", Title: "Node A", Labels: []string{"Product"}},
		},
	})
	if err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	_, err = repo.CommitStagedMutationBatch(CommitStagedMutationRequest{
		Branch:  "feature",
		Message: "Add node A",
	})
	if err != nil {
		t.Fatalf("CommitStagedMutationBatch: %v", err)
	}

	// Step 2 on feature: add edge from SeedNodeID to feature-node-A
	_, err = repo.StageMutationBatch(StageMutationRequest{
		Branch: "feature",
		Operations: []MutationOperation{
			{Action: "add", Entity: "edge", ID: "edge-seed-to-A", Source: SeedNodeID, Target: "feature-node-A", Type: "REL"},
		},
	})
	if err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	edgeCommitRes, err := repo.CommitStagedMutationBatch(CommitStagedMutationRequest{
		Branch:  "feature",
		Message: "Add edge to node A",
	})
	if err != nil {
		t.Fatalf("CommitStagedMutationBatch: %v", err)
	}

	mainHeadBefore := repo.branches["main"]

	// Cherry-picking Step 2 onto main without Step 1 would create a dangling edge because feature-node-A does not exist on main!
	res, err := repo.CherryPick(CherryPickRequest{
		Commit:       string(edgeCommitRes.Commit),
		TargetBranch: "main",
		DryRun:       false,
	})
	if !errors.Is(err, ErrCherryPickConflicts) {
		t.Fatalf("expected ErrCherryPickConflicts for referential integrity violation, got %v", err)
	}
	if len(res.Conflicts) == 0 {
		t.Fatalf("expected structural conflicts for missing target endpoint, got 0: %+v", res)
	}
	foundEndpointConflict := false
	for _, c := range res.Conflicts {
		if c.Entity == "edge" && c.ID == "edge-seed-to-A" {
			foundEndpointConflict = true
		}
	}
	if !foundEndpointConflict {
		t.Fatalf("expected conflict on edge-seed-to-A, got: %+v", res.Conflicts)
	}

	// Verify target branch remains completely unmodified
	if repo.branches["main"] != mainHeadBefore {
		t.Fatalf("target branch head was mutated despite conflict! before=%q, after=%q", mainHeadBefore, repo.branches["main"])
	}
}

func TestCherryPickPropertyCollisionConflict(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "repo")
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	defer func() { _ = repo.Close() }()

	_, err = repo.CreateBranch("feature", BranchSource{Branch: "main"})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	// Update seed node title on feature
	_, err = repo.StageMutationBatch(StageMutationRequest{
		Branch: "feature",
		Operations: []MutationOperation{
			{Action: "update", Entity: "node", ID: SeedNodeID, Title: "Feature Title", Labels: []string{"Root"}},
		},
	})
	if err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	featureCommit, err := repo.CommitStagedMutationBatch(CommitStagedMutationRequest{
		Branch:  "feature",
		Message: "Update title on feature",
	})
	if err != nil {
		t.Fatalf("CommitStagedMutationBatch: %v", err)
	}

	// Update seed node title differently on main
	_, err = repo.StageMutationBatch(StageMutationRequest{
		Branch: "main",
		Operations: []MutationOperation{
			{Action: "update", Entity: "node", ID: SeedNodeID, Title: "Main Divergent Title", Labels: []string{"Root"}},
		},
	})
	if err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	mainHeadBefore, err := repo.CommitStagedMutationBatch(CommitStagedMutationRequest{
		Branch:  "main",
		Message: "Update title on main",
	})
	if err != nil {
		t.Fatalf("CommitStagedMutationBatch: %v", err)
	}

	// Cherry-pick feature commit onto main -> title conflict
	res, err := repo.CherryPick(CherryPickRequest{
		Commit:       string(featureCommit.Commit),
		TargetBranch: "main",
		DryRun:       false,
	})
	if !errors.Is(err, ErrCherryPickConflicts) {
		t.Fatalf("expected ErrCherryPickConflicts, got %v", err)
	}
	if len(res.Conflicts) == 0 {
		t.Fatalf("expected title conflict, got %+v", res)
	}

	// Verify main branch remains untouched
	if repo.branches["main"] != mainHeadBefore.Commit {
		t.Fatalf("target branch head was mutated despite collision! before=%q, after=%q", mainHeadBefore.Commit, repo.branches["main"])
	}
}

func TestCherryPickIdempotentNoOp(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "repo")
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	defer func() { _ = repo.Close() }()

	_, err = repo.CreateBranch("feature", BranchSource{Branch: "main"})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	_, err = repo.StageMutationBatch(StageMutationRequest{
		Branch: "feature",
		Operations: []MutationOperation{
			{Action: "add", Entity: "node", ID: "shared-node", Title: "Shared Node", Labels: []string{"Component"}},
		},
	})
	if err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	featureCommit, err := repo.CommitStagedMutationBatch(CommitStagedMutationRequest{
		Branch:  "feature",
		Message: "Add shared node",
	})
	if err != nil {
		t.Fatalf("CommitStagedMutationBatch: %v", err)
	}

	// Apply 1st time
	res1, err := repo.CherryPick(CherryPickRequest{
		Commit:       string(featureCommit.Commit),
		TargetBranch: "main",
	})
	if err != nil {
		t.Fatalf("CherryPick 1: %v", err)
	}
	if len(res1.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(res1.Changes))
	}

	// Apply 2nd time (already applied): should be clean no-op with 0 changes
	res2, err := repo.CherryPick(CherryPickRequest{
		Commit:       string(featureCommit.Commit),
		TargetBranch: "main",
	})
	if err != nil {
		t.Fatalf("CherryPick 2 (idempotent): %v", err)
	}
	if len(res2.Changes) != 0 {
		t.Fatalf("expected 0 changes for already applied cherry-pick, got %+v", res2.Changes)
	}
	if res2.Commit != res1.Commit {
		t.Fatalf("expected commit to remain %q, got %q", res1.Commit, res2.Commit)
	}
}
