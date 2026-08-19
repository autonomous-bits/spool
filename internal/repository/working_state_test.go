package repository

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func validMutationBatch() []MutationOperation {
	return []MutationOperation{
		{Action: "add", Entity: "node", ID: "node-2", Title: "Second node"},
		{Action: "add", Entity: "edge", ID: "edge-1", Source: SeedNodeID, Target: "node-2"},
	}
}

func TestStageMutationBatchStagesValidBatchAtomically(t *testing.T) {
	repo := NewSeedRepository()

	result, err := repo.StageMutationBatch(StageMutationRequest{Branch: "main", Operations: validMutationBatch()})
	if err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	staged := repo.stagedMutations["main"]
	if result.Branch != "main" || result.BaseCommit != repo.branches["main"] || result.Operations != len(validMutationBatch()) {
		t.Fatalf("result = %#v", result)
	}
	if staged.BaseCommit != repo.branches["main"] || !reflect.DeepEqual(staged.Operations, validMutationBatch()) {
		t.Fatalf("staged set = %#v", staged)
	}
}

func TestStagedEnrichedMutationsNormalizeCommitAndPersist(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	baseSnapshot := repo.snapshots[repo.commits[repo.branches["main"]].Snapshot]
	operations := []MutationOperation{
		{
			Action: "add", Entity: "node", ID: "node-2", Title: "Second node",
			Labels: []string{"Requirement", "Decision", "Requirement"},
			Properties: map[string]PropertyValue{
				"priority": {Kind: PropertyInteger, Integer: 3, String: "discard"},
			},
		},
		{
			Action: "add", Entity: "edge", ID: "edge-1", Source: SeedNodeID, Target: "node-2",
			Type: "DEPENDS_ON",
			Properties: map[string]PropertyValue{
				"weight": {Kind: PropertyFloat, Float: math.Copysign(0, -1)},
			},
		},
	}
	if _, err := repo.StageMutationBatch(StageMutationRequest{Branch: "main", Operations: operations}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	operations[0].Labels[0] = "changed"
	operations[0].Properties["priority"] = StringPropertyValue("changed")

	staged := repo.stagedMutations["main"].Operations
	if got, want := staged[0].Labels, []string{"Decision", "Requirement"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("staged labels = %#v, want %#v", got, want)
	}
	if got := staged[0].Properties["priority"]; !got.Equal(IntegerPropertyValue(3)) {
		t.Fatalf("staged node property = %#v", got)
	}
	if got := staged[1].Properties["weight"]; got.Kind != PropertyFloat || math.Signbit(got.Float) {
		t.Fatalf("staged edge property = %#v", got)
	}

	result, err := repo.CommitStagedMutations("main")
	if err != nil {
		t.Fatalf("CommitStagedMutations: %v", err)
	}
	snapshot := repo.snapshots[repo.commits[result.Commit].Snapshot]
	if snapshot.SchemaRoot != baseSnapshot.SchemaRoot {
		t.Fatalf("committed SchemaRoot = %q, want %q", snapshot.SchemaRoot, baseSnapshot.SchemaRoot)
	}
	if got := repo.projections[snapshot.NodeRoot]["node-2"]; !got.Equal(Node{
		ID: "node-2", Title: "Second node", Labels: []string{"Decision", "Requirement"},
		Properties: map[string]PropertyValue{"priority": IntegerPropertyValue(3)},
	}) {
		t.Fatalf("committed node = %#v", got)
	}
	if got := repo.edgeProjections[repo.commits[result.Commit].Snapshot]["edge-1"]; !got.Equal(Edge{
		ID: "edge-1", Source: SeedNodeID, Target: "node-2", Type: "DEPENDS_ON",
		Properties: map[string]PropertyValue{"weight": FloatPropertyValue(0)},
	}) {
		t.Fatalf("committed edge = %#v", got)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository: %v", err)
	}
	closeTestRepository(t, reopened)
	reopenedSnapshot := reopened.snapshots[reopened.commits[result.Commit].Snapshot]
	if !reopened.projections[reopenedSnapshot.NodeRoot]["node-2"].Equal(repo.projections[snapshot.NodeRoot]["node-2"]) {
		t.Fatal("reopened node lost enriched values")
	}
	if !reopened.edgeProjections[reopened.commits[result.Commit].Snapshot]["edge-1"].Equal(repo.edgeProjections[repo.commits[result.Commit].Snapshot]["edge-1"]) {
		t.Fatal("reopened edge lost enriched values")
	}
}

func TestStageMutationBatchRejectsInvalidEnrichedPropertyBeforeStaging(t *testing.T) {
	repo := NewSeedRepository()
	_, err := repo.StageMutationBatch(StageMutationRequest{
		Branch: "main",
		Operations: []MutationOperation{{
			Action: "add", Entity: "node", ID: "node-2", Title: "Second node",
			Properties: map[string]PropertyValue{"invalid": FloatPropertyValue(math.NaN())},
		}},
	})
	if !errors.Is(err, ErrInvalidMutationBatch) {
		t.Fatalf("StageMutationBatch error = %v, want ErrInvalidMutationBatch", err)
	}
	if _, exists := repo.stagedMutations["main"]; exists {
		t.Fatal("invalid enriched mutation was staged")
	}
}

func TestStageMutationBatchPreservesExplicitEmptyEnrichedFields(t *testing.T) {
	repo := NewSeedRepository()
	if _, err := repo.StageMutationBatch(StageMutationRequest{
		Branch: "main",
		Operations: []MutationOperation{{
			Action: "update", Entity: "node", ID: SeedNodeID, Title: "Updated",
			Labels: []string{}, Properties: map[string]PropertyValue{},
		}},
	}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	staged := repo.stagedMutations["main"].Operations[0]
	if staged.Labels == nil || staged.Properties == nil {
		t.Fatalf("staged empty fields = %#v, want explicit empty values", staged)
	}
	result, err := repo.CommitStagedMutations("main")
	if err != nil {
		t.Fatalf("CommitStagedMutations: %v", err)
	}
	snapshot := repo.snapshots[repo.commits[result.Commit].Snapshot]
	node := repo.projections[snapshot.NodeRoot][SeedNodeID]
	if node.Labels == nil || node.Properties == nil || len(node.Labels) != 0 || len(node.Properties) != 0 {
		t.Fatalf("committed empty fields = %#v", node)
	}
}

func TestCommitStagedMutationsReturnsInvalidPropertyErrorWithoutPanicking(t *testing.T) {
	repo := NewSeedRepository()
	if _, err := repo.StageMutationBatch(StageMutationRequest{
		Branch:     "main",
		Operations: []MutationOperation{{Action: "update", Entity: "node", ID: SeedNodeID, Title: "Updated"}},
	}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	head := repo.branches["main"]
	snapshot := repo.snapshots[repo.commits[head].Snapshot]
	node := repo.projections[snapshot.NodeRoot][SeedNodeID]
	node.Properties = map[string]PropertyValue{"invalid": FloatPropertyValue(math.NaN())}
	repo.projections[snapshot.NodeRoot][SeedNodeID] = node

	if _, err := repo.CommitStagedMutations("main"); !errors.Is(err, ErrInvalidPropertyValue) {
		t.Fatalf("CommitStagedMutations error = %v, want ErrInvalidPropertyValue", err)
	}
	if repo.branches["main"] != head {
		t.Fatalf("branch head = %q, want unchanged %q", repo.branches["main"], head)
	}
}

func TestBranchStagingStatusReportsSharedBranchDelta(t *testing.T) {
	repo := NewSeedRepository()
	if _, err := repo.StageMutationBatch(StageMutationRequest{Branch: "main", Operations: validMutationBatch()}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}

	status, err := repo.BranchStagingStatus("main")
	if err != nil {
		t.Fatalf("BranchStagingStatus: %v", err)
	}
	if status.Branch != "main" || status.BaseCommit != repo.branches["main"] || status.Operations != len(validMutationBatch()) {
		t.Fatalf("status = %#v", status)
	}
}

func TestBranchStagingStatusReportsEmptyAndRejectsMissingBranch(t *testing.T) {
	repo := NewSeedRepository()

	status, err := repo.BranchStagingStatus("main")
	if err != nil {
		t.Fatalf("BranchStagingStatus: %v", err)
	}
	if status != (BranchStagingStatus{Branch: "main"}) {
		t.Fatalf("status = %#v", status)
	}
	if _, err := repo.BranchStagingStatus("missing"); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("BranchStagingStatus error = %v, want ErrBranchNotFound", err)
	}
}

func TestStageMutationBatchRejectsInvalidBatchesWithoutReplacingStagedSet(t *testing.T) {
	repo := NewSeedRepository()
	if _, err := repo.StageMutationBatch(StageMutationRequest{Branch: "main", Operations: validMutationBatch()}); err != nil {
		t.Fatalf("stage valid batch: %v", err)
	}
	before := repo.stagedMutations["main"]

	for _, testCase := range []struct {
		name       string
		operations []MutationOperation
		want       error
	}{
		{
			name:       "generic invalid",
			operations: []MutationOperation{{Action: "update", Entity: "node", ID: "missing", Title: "Missing"}},
			want:       ErrInvalidMutationBatch,
		},
		{
			name:       "missing endpoint",
			operations: []MutationOperation{{Action: "add", Entity: "edge", ID: "edge-2", Source: SeedNodeID, Target: "missing"}},
			want:       ErrMissingEdgeEndpoint,
		},
		{
			name: "missing endpoint masks generic invalid",
			operations: []MutationOperation{
				{Action: "update", Entity: "node", ID: "missing", Title: "Missing"},
				{Action: "add", Entity: "edge", ID: "edge-2", Source: SeedNodeID, Target: "missing"},
			},
			want: ErrMissingEdgeEndpoint,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := repo.StageMutationBatch(StageMutationRequest{Branch: "main", Operations: testCase.operations}); !errors.Is(err, testCase.want) {
				t.Fatalf("StageMutationBatch error = %v, want %v", err, testCase.want)
			}
			if got := repo.stagedMutations["main"]; !reflect.DeepEqual(got, before) {
				t.Fatalf("staged set = %#v, want preserved %#v", got, before)
			}
		})
	}
}

func TestStageMutationBatchAcceptsEdgeEndpointAddedLaterInSameBatch(t *testing.T) {
	repo := NewSeedRepository()
	operations := []MutationOperation{
		{Action: "add", Entity: "edge", ID: "edge-1", Source: SeedNodeID, Target: "node-2"},
		{Action: "add", Entity: "node", ID: "node-2", Title: "Second node"},
	}

	if _, err := repo.StageMutationBatch(StageMutationRequest{Branch: "main", Operations: operations}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
}

func TestStageMutationBatchValidatesExistingEdgeUpdatesAndDeletes(t *testing.T) {
	repo := NewSeedRepository()
	snapshotID := repo.commits[repo.branches["main"]].Snapshot
	repo.edgeProjections[snapshotID]["edge-1"] = Edge{ID: "edge-1", Source: SeedNodeID, Target: SeedNodeID}

	for _, operation := range []MutationOperation{
		{Action: "update", Entity: "edge", ID: "edge-1", Source: SeedNodeID, Target: SeedNodeID},
		{Action: "delete", Entity: "edge", ID: "edge-1"},
	} {
		if _, err := repo.StageMutationBatch(StageMutationRequest{Branch: "main", Operations: []MutationOperation{operation}}); err != nil {
			t.Fatalf("StageMutationBatch(%s edge): %v", operation.Action, err)
		}
	}
}

func TestStageMutationBatchRejectsEdgesReferencingDeletedNodes(t *testing.T) {
	repo := NewSeedRepository()
	operations := []MutationOperation{
		{Action: "delete", Entity: "node", ID: SeedNodeID},
		{Action: "add", Entity: "node", ID: "node-2", Title: "Second node"},
		{Action: "add", Entity: "edge", ID: "edge-1", Source: SeedNodeID, Target: "node-2"},
	}

	if _, err := repo.StageMutationBatch(StageMutationRequest{Branch: "main", Operations: operations}); !errors.Is(err, ErrMissingEdgeEndpoint) {
		t.Fatalf("StageMutationBatch error = %v, want ErrMissingEdgeEndpoint", err)
	}
}

func TestStageMutationBatchRejectsDeletingNodeReferencedByUnchangedEdge(t *testing.T) {
	repo := NewSeedRepository()
	snapshotID := repo.commits[repo.branches["main"]].Snapshot
	repo.edgeProjections[snapshotID]["edge-1"] = Edge{ID: "edge-1", Source: SeedNodeID, Target: SeedNodeID}

	if _, err := repo.StageMutationBatch(StageMutationRequest{Branch: "main", Operations: []MutationOperation{{Action: "delete", Entity: "node", ID: SeedNodeID}}}); !errors.Is(err, ErrMissingEdgeEndpoint) {
		t.Fatalf("StageMutationBatch error = %v, want ErrMissingEdgeEndpoint", err)
	}
}

func TestStageMutationBatchPersistsAcrossRestart(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	if _, err := repo.StageMutationBatch(StageMutationRequest{Branch: "main", Operations: validMutationBatch()}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("reopen repository: %v", err)
	}
	closeTestRepository(t, reopened)
	if got := reopened.stagedMutations["main"]; !reflect.DeepEqual(got.Operations, validMutationBatch()) {
		t.Fatalf("staged set = %#v", got)
	}
	status, err := reopened.BranchStagingStatus("main")
	if err != nil {
		t.Fatalf("BranchStagingStatus: %v", err)
	}
	if status.Operations != len(validMutationBatch()) {
		t.Fatalf("staged operation count = %d, want %d", status.Operations, len(validMutationBatch()))
	}
}

func TestOpenRepositoryAcceptsStateWithoutStagedMutations(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}

	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	path := filepath.Join(stateDir, "repository.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read repository state: %v", err)
	}
	var state map[string]any
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("decode repository state: %v", err)
	}
	delete(state, "stagedMutations")
	data, err = json.Marshal(state)
	if err != nil {
		t.Fatalf("encode repository state: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write old repository state: %v", err)
	}

	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository: %v", err)
	}
	closeTestRepository(t, reopened)
	if len(reopened.stagedMutations) != 0 {
		t.Fatalf("staged mutations = %#v, want empty", reopened.stagedMutations)
	}
}

func TestOpenRepositoryAcceptsHistoricalStagingOutsideNewIngestionLimits(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	if _, err := repo.StageMutationBatch(StageMutationRequest{
		Branch:     "main",
		Operations: []MutationOperation{{Action: "add", Entity: "node", ID: "node-2", Title: "Second node"}},
	}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	path := filepath.Join(stateDir, "repository.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read repository state: %v", err)
	}
	var state persistedRepository
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("decode repository state: %v", err)
	}
	staged := state.StagedMutations["main"]
	staged.Operations[0].Labels = []string{"legacy/label"}
	staged.Operations[0].Properties = map[string]PropertyValue{
		"legacy/key": StringPropertyValue("value"),
	}
	state.StagedMutations["main"] = staged
	data, err = json.Marshal(state)
	if err != nil {
		t.Fatalf("encode legacy repository state: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write legacy repository state: %v", err)
	}

	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository: %v", err)
	}
	closeTestRepository(t, reopened)
	if got := reopened.stagedMutations["main"].Operations[0].Labels; !reflect.DeepEqual(got, []string{"legacy/label"}) {
		t.Fatalf("reopened staged labels = %#v, want legacy label", got)
	}
}

func TestCommitStagedMutationsAdvancesBranchAndClearsStaging(t *testing.T) {
	repo := NewSeedRepository()
	base := repo.branches["main"]
	if _, err := repo.StageMutationBatch(StageMutationRequest{Branch: "main", Operations: validMutationBatch()}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	result, err := repo.CommitStagedMutations("main")
	if err != nil {
		t.Fatalf("CommitStagedMutations: %v", err)
	}
	if result.Branch != "main" || result.Commit == base || repo.branches["main"] != result.Commit {
		t.Fatalf("result/head = %#v/%q, want advanced main", result, repo.branches["main"])
	}
	if _, exists := repo.stagedMutations["main"]; exists {
		t.Fatal("staged mutations remain after commit")
	}
	committed := repo.commits[result.Commit]
	if len(committed.Parents) != 1 || committed.Parents[0] != base {
		t.Fatalf("parents = %#v, want [%q]", committed.Parents, base)
	}
	if _, exists := repo.projections[repo.snapshots[committed.Snapshot].NodeRoot]["node-2"]; !exists {
		t.Fatal("committed node is absent")
	}
	if _, exists := repo.edgeProjections[committed.Snapshot]["edge-1"]; !exists {
		t.Fatal("committed edge is absent")
	}
}

func TestCommitStagedMutationsReusesNodeRootProjectionAcrossEdgeOnlySnapshots(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	originalCommit := repo.branches["main"]
	originalSnapshot := repo.commits[originalCommit].Snapshot
	originalRoot := repo.snapshots[originalSnapshot].NodeRoot

	if _, err := repo.StageMutationBatch(StageMutationRequest{
		Branch: "main",
		Operations: []MutationOperation{
			{Action: "add", Entity: "edge", ID: "edge-1", Source: SeedNodeID, Target: SeedNodeID},
		},
	}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	result, err := repo.CommitStagedMutations("main")
	if err != nil {
		t.Fatalf("CommitStagedMutations: %v", err)
	}
	if committedRoot := repo.snapshots[repo.commits[result.Commit].Snapshot].NodeRoot; committedRoot != originalRoot {
		t.Fatalf("committed NodeRoot = %q, want reused %q", committedRoot, originalRoot)
	}
	if len(repo.projections) != 1 {
		t.Fatalf("projection records = %d, want one shared NodeRoot record", len(repo.projections))
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository: %v", err)
	}
	closeTestRepository(t, reopened)
	if len(reopened.projections) != 1 {
		t.Fatalf("reopened projection records = %d, want one", len(reopened.projections))
	}
	if _, err := reopened.ResolvePinned(result.Commit, SeedNodeID); err != nil {
		t.Fatalf("ResolvePinned committed snapshot: %v", err)
	}
}

func TestCommitStagedMutationsRollsBackNewNodeRootProjectionOnPersistenceFailure(t *testing.T) {
	repo := NewSeedRepository()
	beforeSnapshots, beforeProjections := len(repo.snapshots), len(repo.projections)
	beforeHead := repo.branches["main"]
	if _, err := repo.StageMutationBatch(StageMutationRequest{
		Branch:     "main",
		Operations: []MutationOperation{{Action: "add", Entity: "node", ID: "node-2", Title: "Second node"}},
	}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	repo.persistRepositoryFn = func() error { return errors.New("injected persistence failure") }

	if _, err := repo.CommitStagedMutations("main"); err == nil {
		t.Fatal("CommitStagedMutations succeeded despite persistence failure")
	}
	if len(repo.snapshots) != beforeSnapshots || len(repo.projections) != beforeProjections || repo.branches["main"] != beforeHead {
		t.Fatal("failed commit left a snapshot, projection, or branch update behind")
	}
}

func TestCommitStagedMutationsRejectsStaleBaseWithoutMutation(t *testing.T) {
	repo := NewSeedRepository()
	if _, err := repo.StageMutationBatch(StageMutationRequest{Branch: "main", Operations: validMutationBatch()}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	if _, err := repo.AdvanceBranch("main"); err != nil {
		t.Fatalf("AdvanceBranch: %v", err)
	}
	head, staged := repo.branches["main"], repo.stagedMutations["main"]
	staged.Operations = append([]MutationOperation(nil), staged.Operations...)
	if _, err := repo.CommitStagedMutations("main"); !errors.Is(err, ErrStaleStagedBase) {
		t.Fatalf("CommitStagedMutations error = %v, want ErrStaleStagedBase", err)
	}
	if repo.branches["main"] != head || !reflect.DeepEqual(repo.stagedMutations["main"], staged) {
		t.Fatal("stale commit mutated branch or staging")
	}
}
