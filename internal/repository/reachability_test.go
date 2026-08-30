package repository

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository/branch"
)

func TestScanRetentionTraversesSeedGraph(t *testing.T) {
	repo := newTestSeedRepository(t)

	scan, err := repo.ScanRetention()
	if err != nil {
		t.Fatalf("ScanRetention: %v", err)
	}
	if scan.RootCount != 1 || scan.ReachableObjects != 6 {
		t.Fatalf("scan counts = roots=%d objects=%d, want 1 and 6", scan.RootCount, scan.ReachableObjects)
	}
	for _, id := range []ObjectID{
		repo.branches["main"],
		repo.commits[repo.branches["main"]].Snapshot,
		repo.snapshots[repo.commits[repo.branches["main"]].Snapshot].SchemaRoot,
	} {
		if _, found := scan.Objects[id]; !found {
			t.Fatalf("scan omitted reachable object %s", id)
		}
	}
}

func TestScanRetentionTraversesProllyEntityObjects(t *testing.T) {
	repo := newTestSeedRepository(t)
	if _, err := repo.StageMutationBatch(StageMutationRequest{
		Branch: "main",
		Operations: []MutationOperation{
			{Action: "add", Entity: "node", ID: "22222222-2222-4222-8222-222222222222", Title: "second"},
			{Action: "add", Entity: "edge", ID: "seed-to-second", Source: SeedNodeID, Target: "22222222-2222-4222-8222-222222222222"},
		},
	}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	commitID, err := repo.CommitStagedMutations("main")
	if err != nil {
		t.Fatalf("CommitStagedMutations: %v", err)
	}
	snapshot := repo.snapshots[repo.commits[commitID.Commit].Snapshot]
	entries, err := repo.loadProllyTreeLocked(snapshot.EdgeRoot)
	if err != nil || len(entries) != 1 {
		t.Fatalf("load edge tree = %#v, %v", entries, err)
	}

	scan, err := repo.ScanRetention()
	if err != nil {
		t.Fatalf("ScanRetention: %v", err)
	}
	if _, retained := scan.Objects[entries[0].Value]; !retained {
		t.Fatal("scan omitted edge entity referenced by a Prolly leaf")
	}
}

func TestScanRetentionRetainsDeletedBranchReflogCommit(t *testing.T) {
	repo, err := InitializeRepository(t.TempDir())
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	closeTestRepository(t, repo)
	if _, err := repo.CreateBranch("feature", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	featureHead, err := repo.AdvanceBranch("feature")
	if err != nil {
		t.Fatalf("AdvanceBranch: %v", err)
	}
	if _, err := repo.DeleteBranch("feature"); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}

	scan, err := repo.ScanRetention()
	if err != nil {
		t.Fatalf("ScanRetention: %v", err)
	}
	if _, stillRetained := scan.Objects[featureHead]; !stillRetained {
		t.Fatal("deleted branch commit was not retained from its reflog")
	}
}

func TestScanRetentionRetainsResolvedMergeSnapshot(t *testing.T) {
	stateDir, stagedSnapshot, _ := resolvedMergeTransactionFixture(t)
	repo, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository: %v", err)
	}
	closeTestRepository(t, repo)

	scan, err := repo.ScanRetention()
	if err != nil {
		t.Fatalf("ScanRetention: %v", err)
	}
	if _, retained := scan.Objects[stagedSnapshot]; !retained {
		t.Fatal("resolved merge snapshot was not retained")
	}
}

func TestScanRetentionRejectsMalformedReflogAndMissingObjects(t *testing.T) {
	t.Run("malformed reflog", func(t *testing.T) {
		stateDir := t.TempDir()
		repo, err := InitializeRepository(stateDir)
		if err != nil {
			t.Fatalf("InitializeRepository: %v", err)
		}
		closeTestRepository(t, repo)
		path := filepath.Join(stateDir, "logs", "refs", "heads", "main")
		if err := os.WriteFile(path, []byte("not a reflog\n"), 0o600); err != nil {
			t.Fatalf("write reflog: %v", err)
		}
		if _, err := repo.ScanRetention(); !errors.Is(err, ErrGCCorrupt) {
			t.Fatalf("ScanRetention error = %v, want ErrGCCorrupt", err)
		}
	})

	t.Run("missing graph object", func(t *testing.T) {
		repo := newTestSeedRepository(t)
		missing := ObjectID("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
		id, err := repo.objectStore.put("commit", commit{Snapshot: missing})
		if err != nil {
			t.Fatalf("store malformed commit: %v", err)
		}
		repo.branches = map[string]ObjectID{"main": id}
		if _, err := repo.ScanRetention(); !errors.Is(err, ErrGCCorrupt) {
			t.Fatalf("ScanRetention error = %v, want ErrGCCorrupt", err)
		}
	})

	t.Run("unknown object content", func(t *testing.T) {
		repo := newTestSeedRepository(t)
		id, err := repo.objectStore.put("unknown-content", map[string]string{"key": "value"})
		if err != nil {
			t.Fatalf("store unknown object: %v", err)
		}
		repo.branches = map[string]ObjectID{"main": id}
		if _, err := repo.ScanRetention(); !errors.Is(err, ErrGCCorrupt) {
			t.Fatalf("ScanRetention error = %v, want ErrGCCorrupt", err)
		}
	})

	t.Run("cached durable object replaced", func(t *testing.T) {
		stateDir := t.TempDir()
		repo, err := InitializeRepository(stateDir)
		if err != nil {
			t.Fatalf("InitializeRepository: %v", err)
		}
		closeTestRepository(t, repo)
		head := repo.branches["main"]
		path := filepath.Join(stateDir, "objects", "loose", string(head[:2]), string(head[2:]))
		if err := os.WriteFile(path, []byte("corrupt"), 0o600); err != nil {
			t.Fatalf("corrupt loose object: %v", err)
		}
		if _, err := repo.ScanRetention(); !errors.Is(err, ErrGCCorrupt) {
			t.Fatalf("ScanRetention error = %v, want ErrGCCorrupt", err)
		}
	})
}
