package repository

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository/branch"
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

func TestFsckRepositoryRetainsReflogOnlyObjects(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	if _, err := repo.CreateBranch("feature", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	historical, err := repo.AdvanceBranch("feature")
	if err != nil {
		t.Fatalf("AdvanceBranch: %v", err)
	}
	if _, err := repo.DeleteBranch("feature"); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	result, err := FsckRepository(stateDir)
	if err != nil {
		t.Fatalf("FsckRepository: %v, result = %#v", err, result)
	}
	if !result.Valid || result.Commits != 2 || hasFsckInformation(result, "unreachable-loose-object", historical) {
		t.Fatalf("result = %#v, want reflog-only commit reachable", result)
	}
}

func TestFsckRepositoryReportsReflogCorruption(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(t *testing.T, stateDir string)
		code   string
		path   string
	}{
		{
			name: "missing directory",
			mutate: func(t *testing.T, stateDir string) {
				t.Helper()
				if err := os.RemoveAll(filepath.Join(stateDir, "logs")); err != nil {
					t.Fatalf("remove reflog directory: %v", err)
				}
			},
			code: "missing-reflog-directory",
			path: "logs",
		},
		{
			name: "malformed entry",
			mutate: func(t *testing.T, stateDir string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(stateDir, "logs", "refs", "heads", "main"), []byte("not a\n"), 0o600); err != nil {
					t.Fatalf("write reflog: %v", err)
				}
			},
			code: "invalid-reflog-entry",
			path: "logs/refs/heads/main",
		},
		{
			name: "invalid object",
			mutate: func(t *testing.T, stateDir string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(stateDir, "logs", "refs", "heads", "main"), []byte("invalid  update\n"), 0o600); err != nil {
					t.Fatalf("write reflog: %v", err)
				}
			},
			code: "invalid-reflog-object-id",
			path: "logs/refs/heads/main",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			stateDir := t.TempDir()
			repo, err := InitializeRepository(stateDir)
			if err != nil {
				t.Fatalf("InitializeRepository: %v", err)
			}
			if err := repo.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			test.mutate(t, stateDir)

			result, err := FsckRepository(stateDir)
			if !errors.Is(err, ErrFsckCorrupt) {
				t.Fatalf("FsckRepository error = %v, want ErrFsckCorrupt", err)
			}
			if !hasFsckDiagnosticAtPath(result, test.code, test.path) {
				t.Fatalf("result = %#v, want %s at %s", result, test.code, test.path)
			}
			again, againErr := FsckRepository(stateDir)
			if !errors.Is(againErr, ErrFsckCorrupt) || !reflect.DeepEqual(result, again) {
				t.Fatalf("second FsckRepository result = %#v, %v; want deterministic %#v", again, againErr, result)
			}
		})
	}
}

func TestFsckRepositoryAcceptsPackedObjectsAndReportsLooseGCCandidates(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	orphan, err := repo.objectStore.put("node", Node{ID: "orphan", Title: "orphan"})
	if err != nil {
		t.Fatalf("store unreachable object: %v", err)
	}
	head, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}
	if _, err := repo.GC(GCOptions{}); err != nil {
		t.Fatalf("GC: %v", err)
	}
	headPath, err := repo.objectStore.path(head)
	if err != nil {
		t.Fatalf("head path: %v", err)
	}
	if _, err := os.Stat(headPath); !os.IsNotExist(err) {
		t.Fatalf("reachable loose object remains after GC: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	result, err := FsckRepository(stateDir)
	if err != nil {
		t.Fatalf("FsckRepository: %v, result = %#v", err, result)
	}
	if !result.Valid || len(result.Diagnostics) != 0 || !hasFsckInformation(result, "unreachable-loose-object", orphan) {
		t.Fatalf("result = %#v, want valid packed report with orphan candidate", result)
	}
}

func TestFsckRepositoryReportsMissingLooseDirectoryWithActivePack(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	if _, err := repo.GC(GCOptions{}); err != nil {
		t.Fatalf("GC: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(stateDir, "objects", "loose")); err != nil {
		t.Fatalf("remove loose object directory: %v", err)
	}

	result, err := FsckRepository(stateDir)
	if !errors.Is(err, ErrFsckCorrupt) {
		t.Fatalf("FsckRepository error = %v, want ErrFsckCorrupt", err)
	}
	if !hasFsckDiagnostic(result, "missing-loose-objects") {
		t.Fatalf("result = %#v, want missing loose objects diagnostic", result)
	}
}

func TestFsckRepositoryReportsCorruptActivePack(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	if _, err := repo.GC(GCOptions{}); err != nil {
		t.Fatalf("GC: %v", err)
	}
	manifest, err := repo.objectStore.readPackManifest()
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if len(manifest.Packs) != 1 {
		t.Fatalf("active packs = %d, want 1", len(manifest.Packs))
	}
	packPath, err := repo.objectStore.packPath(manifest.Packs[0].ID)
	if err != nil {
		t.Fatalf("pack path: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := os.WriteFile(packPath, []byte("corrupt"), 0o600); err != nil {
		t.Fatalf("corrupt active pack: %v", err)
	}

	result, err := FsckRepository(stateDir)
	if !errors.Is(err, ErrFsckCorrupt) {
		t.Fatalf("FsckRepository error = %v, want ErrFsckCorrupt", err)
	}
	if result.Valid || !hasFsckDiagnostic(result, "invalid-pack") {
		t.Fatalf("result = %#v, want invalid active pack diagnostic", result)
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

func hasFsckDiagnosticAtPath(result FsckResult, code, path string) bool {
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == code && diagnostic.Path == path {
			return true
		}
	}
	return false
}

func hasFsckInformation(result FsckResult, code string, object ObjectID) bool {
	for _, diagnostic := range result.Informational {
		if diagnostic.Code == code && diagnostic.Object == object {
			return true
		}
	}
	return false
}
