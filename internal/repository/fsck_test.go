package repository

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFsckRepositoryAcceptsValidDurableFixture(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	if _, err := repo.StageMutationBatch(StageMutationRequest{
		Branch:     "main",
		Operations: []MutationOperation{{Action: "add", Entity: "node", ID: "fixture-node", Title: "fixture"}},
	}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	result, err := FsckRepository(stateDir)
	if err != nil {
		t.Fatalf("FsckRepository: %v, result = %#v", err, result)
	}
	if !result.Valid || len(result.Diagnostics) != 0 || result.Branches[0] != "main" ||
		result.Commits != 1 || result.Snapshots != 1 || result.Objects == 0 {
		t.Fatalf("result = %#v, want valid seeded report", result)
	}
}

func TestFsckRepositoryReportsCorruptLooseObjectFixture(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	head, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	path := filepath.Join(stateDir, "objects", "loose", string(head[:2]), string(head[2:]))
	if err := os.WriteFile(path, []byte("not-cbor"), 0o600); err != nil {
		t.Fatalf("corrupt object: %v", err)
	}

	result, err := FsckRepository(stateDir)
	if !errors.Is(err, ErrFsckCorrupt) {
		t.Fatalf("FsckRepository error = %v, want ErrFsckCorrupt", err)
	}
	if result.Valid || !hasFsckDiagnostic(result, "invalid-object-envelope") {
		t.Fatalf("result = %#v, want invalid-object-envelope", result)
	}
	again, againErr := FsckRepository(stateDir)
	if !errors.Is(againErr, ErrFsckCorrupt) || !reflect.DeepEqual(result, again) {
		t.Fatalf("second FsckRepository result = %#v, %v; want deterministic %#v", again, againErr, result)
	}
}

func TestFsckChecksProllyOrderingAndMergeBindings(t *testing.T) {
	repo := NewSeedRepository()
	head, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}
	badRoot, err := repo.objectStore.put(prollyTreeLeafType, prollyTreeLeaf{Entries: []prollyTreeEntry{
		{Key: "z", Value: ObjectID("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")},
		{Key: "a", Value: ObjectID("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")},
	}})
	if err != nil {
		t.Fatalf("store bad leaf: %v", err)
	}
	snapshot := repo.snapshots[repo.commits[head].Snapshot]
	snapshot.NodeRoot = badRoot
	snapshotID, err := repo.objectStore.put("graph-snapshot", snapshot)
	if err != nil {
		t.Fatalf("store snapshot: %v", err)
	}
	repo.snapshots[snapshotID] = snapshot
	badCommit := repo.commits[head]
	badCommit.Snapshot = snapshotID
	badCommitID, err := repo.objectStore.put("commit", badCommit)
	if err != nil {
		t.Fatalf("store commit: %v", err)
	}
	repo.commits[badCommitID] = badCommit
	repo.branches["main"] = badCommitID

	result, err := repo.Fsck()
	if !errors.Is(err, ErrFsckCorrupt) {
		t.Fatalf("Fsck error = %v, want ErrFsckCorrupt", err)
	}
	if !hasFsckDiagnostic(result, "invalid-prolly-leaf") {
		t.Fatalf("result = %#v, want invalid-prolly-leaf", result)
	}
}

func TestFsckRepositoryReportsInvalidMergeState(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	data, err := json.Marshal(persistedMergeTransaction{Version: 1})
	if err != nil {
		t.Fatalf("marshal merge state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "merge", "main.json"), data, 0o600); err != nil {
		t.Fatalf("write merge state: %v", err)
	}

	result, err := FsckRepository(stateDir)
	if !errors.Is(err, ErrFsckCorrupt) {
		t.Fatalf("FsckRepository error = %v, want ErrFsckCorrupt", err)
	}
	if !hasFsckDiagnostic(result, "invalid-merge-binding") {
		t.Fatalf("result = %#v, want invalid-merge-binding", result)
	}
}

func TestFsckRepositoryAcceptsResolvedMergeSnapshotOutsideBranchHistory(t *testing.T) {
	stateDir, _, _ := resolvedMergeTransactionFixture(t)

	result, err := FsckRepository(stateDir)
	if err != nil {
		t.Fatalf("FsckRepository: %v, result = %#v", err, result)
	}
	if !result.Valid || len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want valid resolved merge transaction", result)
	}
}

func TestFsckRepositoryReportsCorruptResolvedMergeSnapshotBinding(t *testing.T) {
	stateDir, _, nodeRoot := resolvedMergeTransactionFixture(t)
	path := filepath.Join(stateDir, "objects", "loose", string(nodeRoot[:2]), string(nodeRoot[2:]))
	if err := os.WriteFile(path, []byte("corrupt"), 0o600); err != nil {
		t.Fatalf("corrupt resolved snapshot root: %v", err)
	}

	result, err := FsckRepository(stateDir)
	if !errors.Is(err, ErrFsckCorrupt) {
		t.Fatalf("FsckRepository error = %v, want ErrFsckCorrupt", err)
	}
	if !hasFsckDiagnostic(result, "invalid-merge-binding") || !hasFsckDiagnostic(result, "invalid-object-envelope") {
		t.Fatalf("result = %#v, want corrupt resolved merge binding diagnostics", result)
	}
}

func hasFsckDiagnostic(result FsckResult, code string) bool {
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
