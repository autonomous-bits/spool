package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
)

func TestMergeCLIPreviewAndApplyCleanMerge(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := repository.InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	if _, err := repo.CreateBranch("feature", repository.BranchSource{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch feature: %v", err)
	}

	if _, err := repo.StageMutationBatch(repository.StageMutationRequest{
		Branch: "feature",
		Operations: []repository.MutationOperation{
			{Action: "add", Entity: "node", ID: "feature-node", Title: "Feature Node"},
		},
	}); err != nil {
		t.Fatalf("stage feature: %v", err)
	}
	if _, err := repo.CommitStagedMutations("feature"); err != nil {
		t.Fatalf("commit feature: %v", err)
	}

	repoProvider := func() (*repository.Repository, error) { return repo, nil }

	var previewOutput bytes.Buffer
	previewCmd := newMergePreviewCommand(repoProvider)
	previewCmd.SetOut(&previewOutput)
	previewCmd.SetArgs([]string{"--source", "feature", "--target", "main"})
	if err := previewCmd.Execute(); err != nil {
		t.Fatalf("execute merge preview: %v", err)
	}

	var previewResult repository.MergePreview
	if err := json.Unmarshal(previewOutput.Bytes(), &previewResult); err != nil {
		t.Fatalf("decode preview result: %v", err)
	}
	if !previewResult.Clean || previewResult.ID == "" {
		t.Fatalf("preview clean = %v, ID = %s", previewResult.Clean, previewResult.ID)
	}

	var applyOutput bytes.Buffer
	applyCmd := newMergeApplyCommand(repoProvider)
	applyCmd.SetOut(&applyOutput)
	applyCmd.SetArgs([]string{
		"--source", "feature",
		"--target", "main",
		"--transaction", "tx-clean-1",
		"--preview", string(previewResult.ID),
		"--author", "alice",
		"--message", "Merge feature into main",
	})
	if err := applyCmd.Execute(); err != nil {
		t.Fatalf("execute merge apply: %v", err)
	}

	var applyResult struct {
		Commit repository.ObjectID `json:"commit"`
	}
	if err := json.Unmarshal(applyOutput.Bytes(), &applyResult); err != nil {
		t.Fatalf("decode apply result: %v", err)
	}
	if applyResult.Commit == "" {
		t.Fatal("empty merge commit")
	}
}

func TestMergeCLIConflictedWorkflowAndAbort(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := repository.InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	if _, err := repo.CreateBranch("feature", repository.BranchSource{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	// Make conflicting edits on feature and main
	if _, err := repo.StageMutationBatch(repository.StageMutationRequest{
		Branch: "feature",
		Operations: []repository.MutationOperation{
			{Action: "update", Entity: "node", ID: repository.SeedNodeID, Title: "Feature Title"},
		},
	}); err != nil {
		t.Fatalf("stage feature: %v", err)
	}
	if _, err := repo.CommitStagedMutations("feature"); err != nil {
		t.Fatalf("commit feature: %v", err)
	}

	if _, err := repo.StageMutationBatch(repository.StageMutationRequest{
		Branch: "main",
		Operations: []repository.MutationOperation{
			{Action: "update", Entity: "node", ID: repository.SeedNodeID, Title: "Main Title"},
		},
	}); err != nil {
		t.Fatalf("stage main: %v", err)
	}
	if _, err := repo.CommitStagedMutations("main"); err != nil {
		t.Fatalf("commit main: %v", err)
	}

	repoProvider := func() (*repository.Repository, error) { return repo, nil }

	// 1. Preview conflicted merge
	var previewOutput bytes.Buffer
	previewCmd := newMergePreviewCommand(repoProvider)
	previewCmd.SetOut(&previewOutput)
	previewCmd.SetArgs([]string{"--source", "feature", "--target", "main"})
	if err := previewCmd.Execute(); err != nil {
		t.Fatalf("execute merge preview: %v", err)
	}
	var previewResult repository.MergePreview
	if err := json.Unmarshal(previewOutput.Bytes(), &previewResult); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if previewResult.Clean {
		t.Fatal("preview clean = true, want false")
	}

	// 2. Apply conflicted merge (should persist conflict state and emit conflict details)
	var applyOutput bytes.Buffer
	applyCmd := newMergeApplyCommand(repoProvider)
	applyCmd.SetOut(&applyOutput)
	applyCmd.SetArgs([]string{
		"--source", "feature",
		"--target", "main",
		"--transaction", "tx-conflict-1",
		"--preview", string(previewResult.ID),
		"--author", "alice",
		"--message", "Merge conflicted",
	})
	if err := applyCmd.Execute(); err != nil {
		t.Fatalf("apply conflicted merge: %v", err)
	}

	// 3. Inspect conflicts
	var conflictsOutput bytes.Buffer
	conflictsCmd := newMergeConflictsCommand(repoProvider)
	conflictsCmd.SetOut(&conflictsOutput)
	conflictsCmd.SetArgs([]string{"--target", "main", "--transaction", "tx-conflict-1"})
	if err := conflictsCmd.Execute(); err != nil {
		t.Fatalf("inspect conflicts: %v", err)
	}
	var conflictResult repository.MergeTransactionStatus
	if err := json.Unmarshal(conflictsOutput.Bytes(), &conflictResult); err != nil {
		t.Fatalf("decode conflicts: %v", err)
	}
	if len(conflictResult.Preview.Conflicts) == 0 {
		t.Fatal("expected conflicts in inspection result")
	}

	// 4. Resolve conflicts
	selections := []repository.MergeResolutionSelection{
		{ConflictID: conflictResult.Preview.Conflicts[0].ConflictID, Choice: "source"},
	}
	selectionsData, err := json.Marshal(selections)
	if err != nil {
		t.Fatalf("marshal selections: %v", err)
	}
	selectionsPath := filepath.Join(t.TempDir(), "selections.json")
	if err := os.WriteFile(selectionsPath, selectionsData, 0o600); err != nil {
		t.Fatalf("write selections: %v", err)
	}

	var resolveOutput bytes.Buffer
	resolveCmd := newMergeResolveCommand(repoProvider)
	resolveCmd.SetOut(&resolveOutput)
	resolveCmd.SetArgs([]string{
		"--target", "main",
		"--transaction", "tx-conflict-1",
		"--preview", string(previewResult.ID),
		"--selections", selectionsPath,
	})
	if err := resolveCmd.Execute(); err != nil {
		t.Fatalf("resolve merge: %v", err)
	}

	// 5. Finalize merge
	var finalizeOutput bytes.Buffer
	finalizeCmd := newMergeFinalizeCommand(repoProvider)
	finalizeCmd.SetOut(&finalizeOutput)
	finalizeCmd.SetArgs([]string{"--target", "main", "--transaction", "tx-conflict-1"})
	if err := finalizeCmd.Execute(); err != nil {
		t.Fatalf("finalize merge: %v", err)
	}
	var finalizeResult struct {
		Commit repository.ObjectID `json:"commit"`
	}
	if err := json.Unmarshal(finalizeOutput.Bytes(), &finalizeResult); err != nil {
		t.Fatalf("decode finalize result: %v", err)
	}
	if finalizeResult.Commit == "" {
		t.Fatal("empty finalize commit")
	}

	// 6. Test Abort on another transaction
	if _, err := repo.CreateBranch("feature2", repository.BranchSource{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch feature2: %v", err)
	}
	if _, err := repo.StageMutationBatch(repository.StageMutationRequest{
		Branch: "feature2",
		Operations: []repository.MutationOperation{
			{Action: "update", Entity: "node", ID: repository.SeedNodeID, Title: "Feature2 Title"},
		},
	}); err != nil {
		t.Fatalf("stage feature2: %v", err)
	}
	if _, err := repo.CommitStagedMutations("feature2"); err != nil {
		t.Fatalf("commit feature2: %v", err)
	}
	if _, err := repo.StageMutationBatch(repository.StageMutationRequest{
		Branch: "main",
		Operations: []repository.MutationOperation{
			{Action: "update", Entity: "node", ID: repository.SeedNodeID, Title: "Main Title 2"},
		},
	}); err != nil {
		t.Fatalf("stage main 2: %v", err)
	}
	if _, err := repo.CommitStagedMutations("main"); err != nil {
		t.Fatalf("commit main 2: %v", err)
	}

	prev2, err := repo.PreviewMerge("feature2", "main")
	if err != nil {
		t.Fatalf("preview2: %v", err)
	}
	_, _ = repo.ApplyMergePreview("feature2", "main", "tx-abort-1", prev2.ID, "", "")

	var abortOutput bytes.Buffer
	abortCmd := newMergeAbortCommand(repoProvider)
	abortCmd.SetOut(&abortOutput)
	abortCmd.SetArgs([]string{"--target", "main", "--transaction", "tx-abort-1"})
	if err := abortCmd.Execute(); err != nil {
		t.Fatalf("abort merge: %v", err)
	}
	var abortResult struct {
		Aborted bool `json:"aborted"`
	}
	if err := json.Unmarshal(abortOutput.Bytes(), &abortResult); err != nil {
		t.Fatalf("decode abort: %v", err)
	}
	if !abortResult.Aborted {
		t.Fatal("expected aborted = true")
	}
}
