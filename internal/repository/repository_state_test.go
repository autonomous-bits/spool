package repository

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository/branch"
)

func closeTestRepository(t *testing.T, repo *Repository) {
	t.Helper()
	t.Cleanup(func() {
		if err := repo.Close(); err != nil {
			t.Errorf("Close repository: %v", err)
		}
	})
}

func TestMergeStateRecoveryRetainsValidOwnerGatedTransaction(t *testing.T) {
	stateDir := t.TempDir()
	beforeRestart, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}

	base, source, target := createDivergedBranchHeads(beforeRestart)
	binding := MergePreviewBinding{MergeBase: base, SourceCommit: source, TargetCommit: target}
	if err := beforeRestart.ApplyConflictedBoundMerge("feature", "main", "owner", binding); !errors.Is(err, ErrMergeConflicted) {
		t.Fatalf("ApplyConflictedBoundMerge: %v, want ErrMergeConflicted", err)
	}
	if err := beforeRestart.Close(); err != nil {
		t.Fatalf("Close before restart: %v", err)
	}

	afterRestart, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("reopen NewSeedRepositoryWithMergeState: %v", err)
	}
	if got := afterRestart.mergeLeases["main"]; got != "owner" {
		t.Fatalf("recovered lease owner = %q, want owner", got)
	}
	if _, err := afterRestart.FinalizeMergeTransaction("main", "other"); !errors.Is(err, ErrMergeOperationNotOwner) {
		t.Fatalf("non-owner FinalizeMergeTransaction error = %v, want ErrMergeOperationNotOwner", err)
	}
	stagedSnapshot := afterRestart.commits[target].Snapshot
	if err := afterRestart.ResolveMergeTransaction("main", "owner", stagedSnapshot); err != nil {
		t.Fatalf("ResolveMergeTransaction: %v", err)
	}
	if err := afterRestart.RestageMergeTransaction("main", "owner"); err != nil {
		t.Fatalf("RestageMergeTransaction: %v", err)
	}
	merged, err := afterRestart.FinalizeMergeTransaction("main", "owner")
	if err != nil {
		t.Fatalf("FinalizeMergeTransaction: %v", err)
	}
	if _, err := os.Stat(afterRestart.mergeStatePath("main")); !os.IsNotExist(err) {
		t.Fatalf("state file remains after finalization: %v", err)
	}
	if err := afterRestart.Close(); err != nil {
		t.Fatalf("Close after finalization: %v", err)
	}
	finalRestart, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("reopen after finalization: %v", err)
	}
	closeTestRepository(t, finalRestart)
	if got := finalRestart.branches["main"]; got != merged {
		t.Fatalf("durable main head = %q, want %q", got, merged)
	}
}

func TestInitializeRepositoryPersistsMainAsActiveBranch(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	if result, err := repo.Initialization(); err != nil || result != (Initialization{DefaultBranch: "main", ActiveBranch: "main"}) {
		t.Fatalf("initialization = %#v, %v", result, err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close initialized repository: %v", err)
	}

	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository: %v", err)
	}
	closeTestRepository(t, reopened)
	if result, err := reopened.Initialization(); err != nil || result != (Initialization{DefaultBranch: "main", ActiveBranch: "main"}) {
		t.Fatalf("reopened initialization = %#v, %v", result, err)
	}
}

func TestInitializeRepositoryRejectsExistingRepositoryWithoutChangingDurableState(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("close initialized repository: %v", err)
	}

	statePath := filepath.Join(stateDir, "repository.json")
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read existing repository state: %v", err)
	}
	if _, err := InitializeRepository(stateDir); !errors.Is(err, ErrRepositoryAlreadyInitialized) {
		t.Fatalf("InitializeRepository error = %v, want ErrRepositoryAlreadyInitialized", err)
	}
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read preserved repository state: %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("InitializeRepository changed existing durable state")
	}
}

func TestOpenRepositoryRejectsNewTargetWithoutCreatingState(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), ".spl")
	if _, err := OpenRepository(stateDir); !errors.Is(err, ErrRepositoryNotInitialized) {
		t.Fatalf("OpenRepository error = %v, want ErrRepositoryNotInitialized", err)
	}

	if _, err := os.Stat(stateDir); !os.IsNotExist(err) {
		t.Fatalf("OpenRepository created state directory: %v", err)
	}
}

func FuzzPersistedRepositoryValidation(f *testing.F) {
	f.Add([]byte(`{"defaultBranch":"main","activeBranch":"main","branches":{}}`))
	f.Add([]byte(`{"branches":[]}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var state persistedRepository
		if err := json.Unmarshal(data, &state); err == nil {
			_ = state.valid()
		}
	})
}

func TestOpenRepositoryAcceptsLegacyRecordsWithoutEnrichedFields(t *testing.T) {
	stateDir := t.TempDir()
	objects := make(map[ObjectID][]byte)
	storeLegacy := func(objectType string, value any) ObjectID {
		t.Helper()
		encoded, err := canonicalCBOR.Marshal(value)
		if err != nil {
			t.Fatalf("marshal %s: %v", objectType, err)
		}
		id := legacyPersistedObjectID(objectType, encoded)
		objects[id] = encoded
		return id
	}

	node := legacyNode{ID: SeedNodeID, Title: "EDG walking skeleton"}
	nodeID := storeLegacy("node", node)
	nodeRoot := storeLegacy("prolly-node-root", []ObjectID{nodeID})
	edge := legacyEdge{ID: "edge-1", Source: SeedNodeID, Target: SeedNodeID}
	edgeID := storeLegacy("edge", edge)
	edgeRoot := storeLegacy("prolly-edge-root", []ObjectID{edgeID})
	outAdjRoot := storeLegacy("prolly-out-adjacency-root", []ObjectID{})
	inAdjRoot := storeLegacy("prolly-in-adjacency-root", []ObjectID{})
	schemaRoot := storeLegacy("schema-root", map[string]string{"version": "v1"})
	snapshot := graphSnapshot{
		NodeRoot: nodeRoot, EdgeRoot: edgeRoot, OutAdjRoot: outAdjRoot,
		InAdjRoot: inAdjRoot, SchemaRoot: schemaRoot,
	}
	snapshotID := storeLegacy("graph-snapshot", snapshot)
	seedCommit := commit{Snapshot: snapshotID, Message: "seed resolve snapshot", Author: defaultCommitAuthor}
	commitID := storeLegacy("commit", seedCommit)
	state := persistedRepository{
		DefaultBranch: "main",
		ActiveBranch:  "main",
		Branches:      map[string]ObjectID{"main": commitID},
		Commits:       map[ObjectID]commit{commitID: seedCommit},
		Snapshots:     map[ObjectID]graphSnapshot{snapshotID: snapshot},
		Projections:   map[ObjectID]map[string]Node{nodeRoot: {SeedNodeID: {ID: SeedNodeID, Title: "EDG walking skeleton"}}},
		Objects:       objects,
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal legacy repository state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "repository.json"), data, 0o600); err != nil {
		t.Fatalf("write legacy repository state: %v", err)
	}

	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository legacy state: %v", err)
	}
	closeTestRepository(t, reopened)
	if got := reopened.snapshots[snapshotID].SchemaRoot; got != schemaRoot {
		t.Fatalf("legacy SchemaRoot = %q, want %q", got, schemaRoot)
	}
	resolution, err := reopened.ResolvePinned(commitID, SeedNodeID)
	if err != nil {
		t.Fatalf("ResolvePinned legacy node: %v", err)
	}
	if !resolution.Node.Equal(Node{ID: SeedNodeID, Title: "EDG walking skeleton"}) {
		t.Fatalf("legacy node = %#v", resolution.Node)
	}
	if got := reopened.edgeProjections[snapshotID][edge.ID]; !got.Equal(Edge{ID: edge.ID, Source: SeedNodeID, Target: SeedNodeID}) {
		t.Fatalf("legacy edge = %#v", got)
	}
}

func TestMergeStateRecoveryDiscardsTornRecordWithoutMovingRef(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	closeTestRepository(t, repo)

	initialTarget := repo.branches["main"]
	if err := os.WriteFile(repo.mergeStatePath("main"), []byte(`{"version":1`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, ".merge-state-interrupted"), []byte("partial"), 0o600); err != nil {
		t.Fatalf("WriteFile temp: %v", err)
	}
	if err := repo.RecoverMergeTransactions(); err != nil {
		t.Fatalf("RecoverMergeTransactions: %v", err)
	}
	if got := repo.branches["main"]; got != initialTarget {
		t.Fatalf("main head = %q, want unchanged %q", got, initialTarget)
	}
	if _, active := repo.mergeLeases["main"]; active {
		t.Fatal("torn record left a lease")
	}
	if _, active := repo.mergeTransactions["main"]; active {
		t.Fatal("torn record left a transaction")
	}
	if _, err := os.Stat(repo.mergeStatePath("main")); !os.IsNotExist(err) {
		t.Fatalf("torn state file remains: %v", err)
	}
}

func TestMergeStateStartupRejectsCorruptRepositoryBeforeServingRefs(t *testing.T) {
	stateDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stateDir, "repository.json"), []byte(`{"branches":{}}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := NewSeedRepositoryWithMergeState(stateDir); err == nil {
		t.Fatal("NewSeedRepositoryWithMergeState accepted corrupt repository state")
	}
}

func TestMergeStateStartupRejectsRepositoryWithDanglingBranch(t *testing.T) {
	stateDir := t.TempDir()
	state := `{"branches":{"main":"missing"},"commits":{},"snapshots":{},"projections":{},"objects":{}}`
	if err := os.WriteFile(filepath.Join(stateDir, "repository.json"), []byte(state), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := NewSeedRepositoryWithMergeState(stateDir); err == nil {
		t.Fatal("NewSeedRepositoryWithMergeState accepted a dangling branch head")
	}
}

func TestMergeStateStartupRejectsProjectionThatDiffersFromCanonicalNode(t *testing.T) {
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
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	statePath := filepath.Join(stateDir, "repository.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var state persistedRepository
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for snapshotID, projection := range state.Projections {
		for nodeID, node := range projection {
			node.Title = "tampered"
			projection[nodeID] = node
			state.Projections[snapshotID] = projection
			break
		}
		break
	}
	data, err = json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := NewSeedRepositoryWithMergeState(stateDir); err == nil {
		t.Fatal("NewSeedRepositoryWithMergeState accepted a tampered projection")
	}
}

func TestCleanMergePersistsTargetRefAcrossRestart(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	base, source, target := createDivergedBranchHeads(repo)
	merged, err := repo.ApplyCleanBoundMerge("feature", "main", "owner", MergePreviewBinding{
		MergeBase: base, SourceCommit: source, TargetCommit: target,
	})
	if err != nil {
		t.Fatalf("ApplyCleanBoundMerge: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close before restart: %v", err)
	}
	reopened, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("reopen NewSeedRepositoryWithMergeState: %v", err)
	}
	closeTestRepository(t, reopened)
	if got := reopened.branches["main"]; got != merged {
		t.Fatalf("durable main head = %q, want %q", got, merged)
	}
}

func TestBranchAdvancePersistsAcrossRestart(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}

	advanced, err := repo.AdvanceBranch("main")
	if err != nil {
		t.Fatalf("AdvanceBranch: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close before restart: %v", err)
	}
	reopened, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("reopen NewSeedRepositoryWithMergeState: %v", err)
	}
	closeTestRepository(t, reopened)
	if got := reopened.branches["main"]; got != advanced {
		t.Fatalf("durable main head = %q, want %q", got, advanced)
	}
}

func TestBranchCreationPersistsAcrossRestart(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}

	mainHead := repo.branches["main"]
	created, err := repo.CreateBranch("feature", branch.Source{Branch: "main"})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if created.Commit != string(mainHead) {
		t.Fatalf("created branch head = %q, want %q", created.Commit, mainHead)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close before restart: %v", err)
	}

	reopened, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("reopen NewSeedRepositoryWithMergeState: %v", err)
	}
	closeTestRepository(t, reopened)
	if got := reopened.branches["feature"]; got != mainHead {
		t.Fatalf("durable feature head = %q, want %q", got, mainHead)
	}
}

func TestBranchDeletionPersistsAcrossRestart(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}

	if _, err := repo.CreateBranch("feature", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if _, err := repo.DeleteBranch("feature"); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close before restart: %v", err)
	}

	reopened, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("reopen NewSeedRepositoryWithMergeState: %v", err)
	}
	closeTestRepository(t, reopened)
	if _, exists := reopened.branches["feature"]; exists {
		t.Fatal("deleted branch restored after restart")
	}
}

func TestBranchSwitchPersistsAcrossRestart(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	if _, err := repo.CreateBranch("feature", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if _, err := repo.SwitchBranch("feature"); err != nil {
		t.Fatalf("SwitchBranch: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close before restart: %v", err)
	}

	reopened, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("reopen NewSeedRepositoryWithMergeState: %v", err)
	}
	closeTestRepository(t, reopened)
	if got := reopened.activeBranch; got != "feature" {
		t.Fatalf("durable active branch = %q, want feature", got)
	}
}

func TestMissingBranchSwitchDoesNotChangeDurableState(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	closeTestRepository(t, repo)

	statePath := filepath.Join(stateDir, "repository.json")
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read repository state before switch: %v", err)
	}
	originalActiveBranch := repo.activeBranch
	persistCalls := 0
	repo.persistRepositoryFn = func() error {
		persistCalls++
		return nil
	}

	if _, err := repo.SwitchBranch("missing"); !errors.Is(err, branch.ErrNotFound) {
		t.Fatalf("SwitchBranch error = %v, want ErrNotFound", err)
	}
	if got := repo.activeBranch; got != originalActiveBranch {
		t.Fatalf("active branch = %q, want unchanged %q", got, originalActiveBranch)
	}
	if persistCalls != 0 {
		t.Fatalf("missing branch switch attempted %d durable writes, want 0", persistCalls)
	}

	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read repository state after switch: %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("missing branch switch changed durable repository state")
	}
}

func TestActiveBranchSwitchDoesNotChangeDurableState(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	closeTestRepository(t, repo)

	statePath := filepath.Join(stateDir, "repository.json")
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read repository state before switch: %v", err)
	}
	persistCalls := 0
	repo.persistRepositoryFn = func() error {
		persistCalls++
		return nil
	}

	result, err := repo.SwitchBranch("main")
	if err != nil {
		t.Fatalf("SwitchBranch: %v", err)
	}
	if result != (branch.SwitchResult{ActiveBranch: "main"}) {
		t.Fatalf("SwitchBranch result = %#v", result)
	}
	if persistCalls != 0 {
		t.Fatalf("active branch switch attempted %d durable writes, want 0", persistCalls)
	}

	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read repository state after switch: %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("active branch switch changed durable repository state")
	}
}

func TestMergeStatePersistenceFailureLeavesLiveStateUnchanged(t *testing.T) {
	repo, err := NewSeedRepositoryWithMergeState(t.TempDir())
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	closeTestRepository(t, repo)
	base, source, target := createDivergedBranchHeads(repo)
	repo.persistStateFn = func(string, string, *mergeTransaction) error {
		return errors.New("injected state write failure")
	}

	err = repo.ApplyConflictedBoundMerge("feature", "main", "owner", MergePreviewBinding{
		MergeBase: base, SourceCommit: source, TargetCommit: target,
	})
	if err == nil {
		t.Fatal("ApplyConflictedBoundMerge succeeded despite state write failure")
	}
	if got := repo.branches["main"]; got != target {
		t.Fatalf("main head = %q, want unchanged %q", got, target)
	}
	if _, active := repo.mergeLeases["main"]; active {
		t.Fatal("failed persistence created a lease")
	}
	if _, active := repo.mergeTransactions["main"]; active {
		t.Fatal("failed persistence created a transaction")
	}
}

func TestMergeStatePathsDoNotCollideForCaseVariantBranches(t *testing.T) {
	repo := NewSeedRepository()
	repo.mergeStateDir = t.TempDir()
	if got, want := repo.mergeStatePath("Feature"), repo.mergeStatePath("feature"); got == want {
		t.Fatalf("case-variant state paths collide: %q", got)
	}
}

func TestWriteDurableStateFileReplacesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatalf("write existing state: %v", err)
	}
	if err := writeDurableStateFile(path, []byte("new")); err != nil {
		t.Fatalf("writeDurableStateFile: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read replaced state: %v", err)
	}
	if got, want := string(data), "new"; got != want {
		t.Fatalf("replaced state = %q, want %q", got, want)
	}
}

func TestDurableRepositoryLockRejectsOtherRepositoryInstance(t *testing.T) {
	stateDir := t.TempDir()
	first, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	closeTestRepository(t, first)
	if _, err := NewSeedRepositoryWithMergeState(stateDir); !errors.Is(err, ErrMergeRepositoryLocked) {
		t.Fatalf("second NewSeedRepositoryWithMergeState error = %v, want ErrMergeRepositoryLocked", err)
	}
}

func TestClosedStateBackedRepositoryRejectsMutations(t *testing.T) {
	repo, err := NewSeedRepositoryWithMergeState(t.TempDir())
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := repo.AdvanceBranch("main"); !errors.Is(err, ErrMergeRepositoryClosed) {
		t.Fatalf("AdvanceBranch after Close error = %v, want ErrMergeRepositoryClosed", err)
	}
}

func TestCanonicalRootsReconstructIndependentlyOfDerivedProjection(t *testing.T) {
	repo := NewSeedRepository()
	snapshotID := repo.commits[repo.branches["main"]].Snapshot
	snapshot := repo.snapshots[snapshotID]
	state := repo.persistedRepositoryLocked()

	if !state.validStoredObject(snapshotID, "graph-snapshot", snapshot) {
		t.Fatal("seed graph snapshot ID does not verify its canonical roots")
	}

	canonical, ok := state.canonicalNodeProjection(snapshot.NodeRoot)
	if !ok {
		t.Fatal("reconstruct canonical seed projection")
	}
	if got, want := canonical[SeedNodeID], (Node{ID: SeedNodeID, Title: "EDG walking skeleton"}); !got.Equal(want) {
		t.Fatalf("canonical node = %#v, want %#v", got, want)
	}

	nodeIDs, ok := state.canonicalRoot(snapshot.NodeRoot, "prolly-node-root")
	if !ok {
		t.Fatal("decode canonical node root")
	}
	nonEmptyRoots := snapshot
	nonEmptyRoots.EdgeRoot = repo.store("prolly-edge-root", nodeIDs)
	nonEmptyRoots.OutAdjRoot = repo.store("prolly-out-adjacency-root", nodeIDs)
	nonEmptyRoots.InAdjRoot = repo.store("prolly-in-adjacency-root", nodeIDs)
	if !state.validSnapshotRoots(nonEmptyRoots) {
		t.Fatal("snapshot root validation rejected non-empty canonical roots")
	}

	for _, root := range []ObjectID{
		snapshot.NodeRoot,
		snapshot.EdgeRoot,
		snapshot.OutAdjRoot,
		snapshot.InAdjRoot,
		snapshot.SchemaRoot,
	} {
		t.Run(string(root), func(t *testing.T) {
			tampered := state
			tampered.Objects = maps.Clone(state.Objects)
			tampered.Objects[root] = []byte("tampered")
			if root == snapshot.NodeRoot {
				if _, ok := tampered.canonicalNodeProjection(snapshot.NodeRoot); ok {
					t.Fatal("canonical node reconstruction accepted a tampered node root")
				}
			} else if tampered.validSnapshotRoots(snapshot) {
				t.Fatal("snapshot root validation accepted a tampered root")
			}
		})
	}

	state.Objects["unreferenced"] = []byte("not a canonical object")
	if reconstructed, ok := state.canonicalNodeProjection(snapshot.NodeRoot); !ok || !reflect.DeepEqual(reconstructed, canonical) {
		t.Fatal("canonical reconstruction depended on an unreferenced stored object")
	}

	repo.projections[snapshot.NodeRoot][SeedNodeID] = Node{ID: SeedNodeID, Title: "derived projection change"}
	if reconstructed, ok := state.canonicalNodeProjection(snapshot.NodeRoot); !ok || !reflect.DeepEqual(reconstructed, canonical) {
		t.Fatal("derived projection changed canonical reconstruction")
	}
}

func TestStateBackedRepositoryRejectsPersistedDerivedProjectionMutation(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	snapshotID := repo.commits[repo.branches["main"]].Snapshot
	repo.projections[repo.snapshots[snapshotID].NodeRoot][SeedNodeID] = Node{ID: SeedNodeID, Title: "derived projection change"}
	if _, err := repo.AdvanceBranch("main"); err != nil {
		t.Fatalf("AdvanceBranch: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := NewSeedRepositoryWithMergeState(stateDir); err == nil {
		t.Fatal("NewSeedRepositoryWithMergeState accepted a persisted derived projection mutation")
	}
}

func TestLegacyStateReconstructsCanonicalEdgeProjection(t *testing.T) {
	repo := NewSeedRepository()
	originalCommit := repo.branches["main"]
	originalSnapshot := repo.commits[originalCommit].Snapshot
	edge := Edge{ID: "edge-1", Source: SeedNodeID, Target: SeedNodeID}
	edgeID := repo.store("edge", edge)
	snapshot := repo.snapshots[originalSnapshot]
	snapshot.EdgeRoot = repo.store("prolly-edge-root", []ObjectID{edgeID})
	snapshotID := repo.store("graph-snapshot", snapshot)
	repo.snapshots[snapshotID] = snapshot
	repo.edgeProjections[snapshotID] = map[string]Edge{edge.ID: edge}
	commitID := repo.store("commit", commit{Snapshot: snapshotID, Parents: []ObjectID{originalCommit}, Message: "edge fixture"})
	repo.commits[commitID] = commit{Snapshot: snapshotID, Parents: []ObjectID{originalCommit}, Message: "edge fixture"}
	repo.branches["main"] = commitID

	state := repo.persistedRepositoryLocked()
	state.EdgeProjections = nil
	if !state.valid() {
		t.Fatal("legacy state with canonical edge root is invalid")
	}
	reopened := NewSeedRepository()
	reopened.restorePersistedRepositoryLocked(state)
	if got := reopened.edgeProjections[snapshotID][edge.ID]; !got.Equal(edge) {
		t.Fatalf("reconstructed edge = %#v, want %#v", got, edge)
	}
}
