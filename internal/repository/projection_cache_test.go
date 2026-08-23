package repository

import (
	"errors"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository/branch"
)

func TestEnsureSnapshotProjectionRejectsMissingStoreForStaleCacheMarker(t *testing.T) {
	source := NewSeedRepository()
	snapshotID := source.commits[source.branches["main"]].Snapshot
	validator := &Repository{
		snapshots:             map[ObjectID]graphSnapshot{snapshotID: source.snapshots[snapshotID]},
		projections:           make(map[ObjectID]map[string]Node),
		edgeProjections:       make(map[ObjectID]map[string]Edge),
		materializedSnapshots: map[ObjectID]struct{}{snapshotID: {}},
	}

	if err := validator.ensureSnapshotProjectionLocked(snapshotID); !errors.Is(err, ErrProjectionUnavailable) {
		t.Fatalf("ensure snapshot error = %v, want ErrProjectionUnavailable", err)
	}
}

func TestEnsureSnapshotProjectionRebuildsStaleCacheMarker(t *testing.T) {
	repo := NewSeedRepository()
	snapshotID := repo.commits[repo.branches["main"]].Snapshot
	snapshot := repo.snapshots[snapshotID]
	delete(repo.projections, snapshot.NodeRoot)
	delete(repo.edgeProjections, snapshotID)

	if err := repo.ensureSnapshotProjectionLocked(snapshotID); err != nil {
		t.Fatalf("ensure snapshot: %v", err)
	}
	if _, exists := repo.projections[snapshot.NodeRoot]; !exists {
		t.Fatal("stale marker prevented node projection reconstruction")
	}
	if _, exists := repo.edgeProjections[snapshotID]; !exists {
		t.Fatal("stale marker prevented edge projection reconstruction")
	}
}

func commitSeedTitle(t *testing.T, repo *Repository, title string) ObjectID {
	t.Helper()
	if _, err := repo.StageMutationBatch(StageMutationRequest{
		Branch: "main",
		Operations: []MutationOperation{{
			Action: "update", Entity: "node", ID: SeedNodeID, Title: title,
		}},
	}); err != nil {
		t.Fatalf("StageMutationBatch(%q): %v", title, err)
	}
	result, err := repo.CommitStagedMutations("main")
	if err != nil {
		t.Fatalf("CommitStagedMutations(%q): %v", title, err)
	}
	return result.Commit
}

func TestOpenRepositoryDefersHistoricalProjectionMaterialization(t *testing.T) {
	repo, err := InitializeRepository(t.TempDir())
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	stateDir := repo.mergeStateDir
	commits := []ObjectID{repo.branches["main"]}
	for index := range 3 {
		commits = append(commits, commitSeedTitle(t, repo, "version "+string(rune('a'+index))))
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository: %v", err)
	}
	defer closeTestRepository(t, reopened)
	if got := len(reopened.edgeProjections); got != 1 {
		t.Fatalf("materialized snapshots after open = %d, want active head only", got)
	}
	historicalSnapshot := reopened.commits[commits[1]].Snapshot
	if _, exists := reopened.edgeProjections[historicalSnapshot]; exists {
		t.Fatal("open materialized a historical snapshot")
	}

	if _, err := reopened.ResolvePinned(commits[1], SeedNodeID); err != nil {
		t.Fatalf("ResolvePinned historical commit: %v", err)
	}
	if _, exists := reopened.edgeProjections[historicalSnapshot]; !exists {
		t.Fatal("historical query did not materialize its snapshot")
	}
}

func TestHistoricalProjectionLRUEvictsAfterEightWhileBranchHeadsRemainPinned(t *testing.T) {
	repo := NewSeedRepository()
	if _, err := repo.CreateBranch("pinned", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	commits := []ObjectID{repo.branches["main"]}
	for index := range 10 {
		commits = append(commits, commitSeedTitle(t, repo, "version "+string(rune('a'+index))))
	}

	for _, commitID := range commits[1:10] {
		if _, err := repo.ResolvePinned(commitID, SeedNodeID); err != nil {
			t.Fatalf("ResolvePinned(%s): %v", commitID, err)
		}
	}
	if got := len(repo.historicalProjectionLRU); got != historicalProjectionCacheCapacity {
		t.Fatalf("historical cache entries = %d, want %d", got, historicalProjectionCacheCapacity)
	}
	evicted := repo.commits[commits[1]].Snapshot
	if _, exists := repo.edgeProjections[evicted]; exists {
		t.Fatal("least recently used historical snapshot was retained")
	}
	for _, commitID := range []ObjectID{repo.branches["pinned"], repo.branches["main"]} {
		snapshotID := repo.commits[commitID].Snapshot
		if _, exists := repo.edgeProjections[snapshotID]; !exists {
			t.Fatalf("branch head snapshot %s was evicted", snapshotID)
		}
	}
}
