package repository

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository/branch"
)

func TestMutableControlFilesSurviveRestart(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	main, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch main: %v", err)
	}
	if _, err := repo.CreateBranch("feature", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if _, err := repo.SwitchBranch("feature"); err != nil {
		t.Fatalf("SwitchBranch: %v", err)
	}
	if _, err := repo.StageMutationBatch(StageMutationRequest{
		Branch:     "feature",
		Operations: []MutationOperation{{Action: "add", Entity: "node", ID: "22222222-2222-4222-8222-222222222222", Title: "durable"}},
	}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	for _, path := range []string{
		"config.toml", reflogRetentionInventoryFilename, "HEAD", filepath.Join("refs", "heads", "main"),
		filepath.Join("refs", "heads", "feature"), filepath.Join("staged", "feature.json"),
		filepath.Join("logs", "HEAD"), filepath.Join("logs", "refs", "heads", "feature"),
	} {
		if _, err := os.Stat(filepath.Join(stateDir, path)); err != nil {
			t.Fatalf("control file %s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(stateDir, "repository.json")); !os.IsNotExist(err) {
		t.Fatalf("repository.json exists: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(stateDir, "config.toml")); err != nil || !strings.Contains(string(data), "format_version = 1") || !strings.Contains(string(data), `default_branch = 'main'`) || !strings.Contains(string(data), "reflog_retention_inventory = true") {
		t.Fatalf("config.toml = %q, %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(stateDir, "HEAD")); err != nil || string(data) != "feature\n" {
		t.Fatalf("HEAD = %q, %v", data, err)
	}

	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository: %v", err)
	}
	defer closeTestRepository(t, reopened)
	if initialization, err := reopened.Initialization(); err != nil || initialization != (Initialization{DefaultBranch: "main", ActiveBranch: "feature"}) {
		t.Fatalf("reopened initialization = %#v, %v", initialization, err)
	}
	if head, err := reopened.PinBranch("main"); err != nil || head != main {
		t.Fatalf("reopened main = %q, %v; want %q", head, err, main)
	}
	if status, err := reopened.BranchStagingStatus("feature"); err != nil || status.Operations != 1 {
		t.Fatalf("reopened stage = %#v, %v", status, err)
	}
	committed, err := reopened.CommitStagedMutations("feature")
	if err != nil {
		t.Fatalf("CommitStagedMutations: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("close committed repository: %v", err)
	}

	final, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("open committed repository: %v", err)
	}
	defer closeTestRepository(t, final)
	if head, err := final.PinBranch("feature"); err != nil || head != committed.Commit {
		t.Fatalf("reopened feature = %q, %v; want %q", head, err, committed.Commit)
	}
	if status, err := final.BranchStagingStatus("feature"); err != nil || status.Operations != 0 {
		t.Fatalf("committed stage = %#v, %v", status, err)
	}
	if data, err := os.ReadFile(filepath.Join(stateDir, "logs", "refs", "heads", "feature")); err != nil || !strings.Contains(string(data), "commit") {
		t.Fatalf("feature reflog = %q, %v", data, err)
	}
}

func TestLegacyRepositoryJSONIsRejectedWithoutMutation(t *testing.T) {
	stateDir := t.TempDir()
	legacy := []byte(`{"defaultBranch":"main"}`)
	path := filepath.Join(stateDir, "repository.json")
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatalf("write legacy state: %v", err)
	}

	for _, open := range []func(string) (*Repository, error){OpenRepository, InitializeRepository, NewSeedRepositoryWithMergeState} {
		repo, err := open(stateDir)
		if repo != nil {
			_ = repo.Close()
			t.Fatalf("legacy open unexpectedly returned a repository")
		}
		if !errors.Is(err, ErrLegacyRepositoryState) {
			t.Fatalf("legacy open error = %v, want ErrLegacyRepositoryState", err)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read legacy state after rejection: %v", err)
	}
	if !bytes.Equal(after, legacy) {
		t.Fatal("legacy repository.json was mutated")
	}
	if _, err := os.Stat(filepath.Join(stateDir, "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("legacy rejection created config.toml: %v", err)
	}
}

func TestMergeRecordsUseDedicatedDirectoryAcrossRestart(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	base, source, target := createDivergedBranchHeads(repo)
	binding := MergePreviewBinding{MergeBase: base, SourceCommit: source, TargetCommit: target}
	if err := repo.ApplyConflictedBoundMerge("feature", "main", "owner", binding); !errors.Is(err, ErrMergeConflicted) {
		t.Fatalf("ApplyConflictedBoundMerge: %v", err)
	}
	mergePath := repo.mergeStatePath("main")
	if _, err := os.Stat(mergePath); err != nil {
		t.Fatalf("merge record %q: %v", mergePath, err)
	}
	data, err := os.ReadFile(mergePath)
	if err != nil {
		t.Fatalf("read merge record: %v", err)
	}
	var state persistedMergeTransaction
	if err := json.Unmarshal(data, &state); err != nil || !repo.validPersistedMergeTransactionLocked(state) {
		t.Fatalf("written merge record is invalid: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("close repository: %v", err)
	}
	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository: %v", err)
	}
	defer closeTestRepository(t, reopened)
	if got := reopened.mergeLeases["main"]; got != "owner" {
		t.Fatalf("recovered merge lease = %q, want owner", got)
	}
}

func TestInMemoryRepositoryNeverWritesControlFiles(t *testing.T) {
	t.Chdir(t.TempDir())
	repo := newTestSeedRepository(t)
	t.Cleanup(func() {
		if err := repo.Close(); err != nil {
			t.Errorf("close in-memory repository: %v", err)
		}
	})

	if _, err := repo.CreateBranch("feature", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if _, err := repo.SwitchBranch("feature"); err != nil {
		t.Fatalf("SwitchBranch: %v", err)
	}
	if _, err := repo.StageMutationBatch(StageMutationRequest{
		Branch: "feature",
		Operations: []MutationOperation{{
			Action: "add", Entity: "node", ID: "in-memory-node", Title: "in memory",
		}},
	}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	if _, err := repo.CommitStagedMutations("feature"); err != nil {
		t.Fatalf("CommitStagedMutations: %v", err)
	}
	if _, err := repo.CreateBranch("discard", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch discard: %v", err)
	}
	if _, err := repo.DeleteBranch("discard"); err != nil {
		t.Fatalf("DeleteBranch discard: %v", err)
	}

	for _, path := range []string{"config.toml", "HEAD", "refs", "staged", "logs", "merge", "objects"} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("in-memory repository wrote %q: %v", path, err)
		}
	}
}
