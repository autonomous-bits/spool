package repository

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"testing"
)

const (
	defaultStressNodeCount   = 1_000
	defaultStressEdgeCount   = 5_000
	defaultStressCommitCount = 25
)

type stressConfig struct {
	nodeCount   int
	edgeCount   int
	commitCount int
}

func stressConfigForTest(t *testing.T) stressConfig {
	t.Helper()
	if os.Getenv("SPOOL_STRESS") != "1" {
		t.Skip("stress tests disabled; set SPOOL_STRESS=1 to enable")
	}
	return stressConfig{
		nodeCount:   stressPositiveIntEnv(t, "SPOOL_STRESS_NODES", defaultStressNodeCount),
		edgeCount:   stressPositiveIntEnv(t, "SPOOL_STRESS_EDGES", defaultStressEdgeCount),
		commitCount: stressPositiveIntEnv(t, "SPOOL_STRESS_COMMITS", defaultStressCommitCount),
	}
}

func stressPositiveIntEnv(t *testing.T, name string, defaultValue int) int {
	t.Helper()
	value, ok := os.LookupEnv(name)
	if !ok {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		t.Fatalf("invalid %s=%q: must be a positive integer", name, value)
	}
	return parsed
}

func stressGraphOperations(config stressConfig) []MutationOperation {
	operations := make([]MutationOperation, 0, config.nodeCount+config.edgeCount)
	for index := 0; index < config.nodeCount; index++ {
		operations = append(operations, MutationOperation{
			Action: "add",
			Entity: "node",
			ID:     stressNodeID(index),
			Title:  fmt.Sprintf("Stress node %06d", index),
		})
	}
	for index := 0; index < config.edgeCount; index++ {
		source := index % config.nodeCount
		target := (index*17 + 1) % config.nodeCount
		operations = append(operations, MutationOperation{
			Action: "add",
			Entity: "edge",
			ID:     stressEdgeID(index),
			Source: stressNodeID(source),
			Target: stressNodeID(target),
			Type:   "STRESS_LINK",
		})
	}
	return operations
}

func stressNodeID(index int) string {
	return fmt.Sprintf("stress-node-%06d", index)
}

func stressEdgeID(index int) string {
	return fmt.Sprintf("stress-edge-%020d", index)
}

func TestStressHarness(t *testing.T) {
	config := stressConfigForTest(t)
	operations := stressGraphOperations(config)
	if got, want := len(operations), config.nodeCount+config.edgeCount; got != want {
		t.Fatalf("stress operation count = %d, want %d", got, want)
	}
	if !reflect.DeepEqual(operations, stressGraphOperations(config)) {
		t.Fatal("stress graph operations are not deterministic")
	}
	for index, operation := range operations[:config.nodeCount] {
		if operation.ID != stressNodeID(index) {
			t.Fatalf("node %d ID = %q, want %q", index, operation.ID, stressNodeID(index))
		}
	}
	for index, operation := range operations[config.nodeCount:] {
		source := index % config.nodeCount
		target := (index*17 + 1) % config.nodeCount
		if operation.ID != stressEdgeID(index) ||
			operation.Source != stressNodeID(source) ||
			operation.Target != stressNodeID(target) {
			t.Fatalf("edge %d = %#v, want stable topology", index, operation)
		}
	}
	if stressEdgeID(999_999) >= stressEdgeID(1_000_000) {
		t.Fatal("edge IDs do not preserve numeric order across the six-digit boundary")
	}
}

func TestStressLargeGraphDurableLifecycle(t *testing.T) {
	config := stressConfigForTest(t)
	operations := stressGraphOperations(config)
	stateDir := t.TempDir()
	recorder := NewPerformanceRecorder()

	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	repo.performanceRecorder = recorder
	base, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch seed: %v", err)
	}
	staged, err := repo.StageMutationBatch(StageMutationRequest{
		Branch:     "main",
		Operations: operations,
	})
	if err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	if got, want := staged.Operations, len(operations); got != want {
		t.Fatalf("staged operations = %d, want %d", got, want)
	}
	committed, err := repo.CommitStagedMutations("main")
	if err != nil {
		t.Fatalf("CommitStagedMutations: %v", err)
	}
	head, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch head: %v", err)
	}
	if head != committed.Commit {
		t.Fatalf("head = %q, want committed %q", head, committed.Commit)
	}
	if result, err := repo.Fsck(); err != nil || !result.Valid || result.Commits != 2 {
		t.Fatalf("Fsck = %#v, %v; want valid repository with retained seed commit", result, err)
	}
	assertStressGraphReadable(t, repo, base, head, config)

	diff, err := repo.Diff(DiffRequest{
		Base:             base,
		Target:           head,
		MaxRows:          len(operations),
		MaxResponseBytes: max(1<<20, len(operations)*1024),
	})
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if got, want := len(diff.Changes), len(operations); got != want || diff.ContinuationToken != "" {
		t.Fatalf("diff changes = %d with continuation %q, want %d complete changes", got, diff.ContinuationToken, want)
	}
	if first, want := diff.Changes[0], stressNodeID(0); first.Entity != "node" || first.Change != "added" || first.ID != want {
		t.Fatalf("first diff change = %#v, want added node %q", first, want)
	}
	if last, want := diff.Changes[len(diff.Changes)-1], stressEdgeID(config.edgeCount-1); last.Entity != "edge" || last.Change != "added" || last.ID != want {
		t.Fatalf("last diff change = %#v, want added edge %q", last, want)
	}

	if err := repo.Close(); err != nil {
		t.Fatalf("Close before reopen: %v", err)
	}
	reopened, err := openRepositoryWithPerformanceRecorder(stateDir, recorder)
	if err != nil {
		t.Fatalf("OpenRepository: %v", err)
	}
	assertStressGraphReadable(t, reopened, base, head, config)
	if result, err := reopened.Fsck(); err != nil || !result.Valid {
		t.Fatalf("Fsck = %#v, %v; want valid durable repository", result, err)
	}
	packed, err := reopened.GC(GCOptions{})
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if packed.PackedObjects == 0 {
		t.Fatal("GC did not pack the large reachable graph")
	}
	headPath, err := reopened.objectStore.path(head)
	if err != nil {
		t.Fatalf("head loose path: %v", err)
	}
	if _, err := os.Stat(headPath); !os.IsNotExist(err) {
		t.Fatalf("packed head loose object still exists: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("Close after GC: %v", err)
	}

	endPackedReopen := recorder.Measure("packed_repository_reopen")
	packedReopened, err := openRepositoryWithPerformanceRecorder(stateDir, recorder)
	endPackedReopen()
	if err != nil {
		t.Fatalf("OpenRepository from packed storage: %v", err)
	}
	defer closeTestRepository(t, packedReopened)
	assertStressGraphReadable(t, packedReopened, base, head, config)
	assertStressPerformancePhases(t, recorder, []string{
		"mutation_normalization_candidate_construction",
		"immutable_object_encoding",
		"commit_pack_publication",
		"commit_ref_publication",
		"repository_reopen_control_state_loading",
		"projection_reconstruction",
		"gc_packing_publication",
		"packed_repository_reopen",
	})
	reportStressPerformance(t, recorder)
}

func assertStressPerformancePhases(t *testing.T, recorder *PerformanceRecorder, expected []string) {
	t.Helper()
	actual := make(map[string]PerformancePhase)
	for _, phase := range recorder.Phases() {
		actual[phase.Name] = phase
	}
	for _, name := range expected {
		phase, exists := actual[name]
		if !exists || phase.DurationNanos <= 0 {
			t.Fatalf("performance phase %q = %#v, want recorded duration", name, phase)
		}
	}
}

func reportStressPerformance(t *testing.T, recorder *PerformanceRecorder) {
	t.Helper()
	report, err := json.Marshal(struct {
		Phases []PerformancePhase `json:"phases"`
	}{Phases: recorder.Phases()})
	if err != nil {
		t.Fatalf("marshal performance report: %v", err)
	}
	t.Logf("performance diagnostics: %s", report)
}

func assertStressGraphReadable(t *testing.T, repo *Repository, base, head ObjectID, config stressConfig) {
	t.Helper()

	nodes, err := repo.PinnedNodes(head)
	if err != nil {
		t.Fatalf("PinnedNodes: %v", err)
	}
	if got, want := len(nodes), config.nodeCount+1; got != want {
		t.Fatalf("pinned node count = %d, want %d", got, want)
	}
	representativeNode := stressNodeID(config.nodeCount - 1)
	if got := findStressNode(nodes, representativeNode); !got.Equal(Node{
		ID: representativeNode, Title: fmt.Sprintf("Stress node %06d", config.nodeCount-1),
	}) {
		t.Fatalf("canonical pinned node %q = %#v", representativeNode, got)
	}

	edges, err := repo.PinnedEdges(head)
	if err != nil {
		t.Fatalf("PinnedEdges: %v", err)
	}
	if got, want := len(edges), config.edgeCount; got != want {
		t.Fatalf("pinned edge count = %d, want %d", got, want)
	}
	representativeEdgeIndex := config.edgeCount - 1
	representativeEdge := stressEdgeID(representativeEdgeIndex)
	if got := findStressEdge(edges, representativeEdge); !got.Equal(Edge{
		ID:     representativeEdge,
		Source: stressNodeID(representativeEdgeIndex % config.nodeCount),
		Target: stressNodeID((representativeEdgeIndex*17 + 1) % config.nodeCount),
		Type:   "STRESS_LINK",
	}) {
		t.Fatalf("canonical pinned edge %q = %#v", representativeEdge, got)
	}

	resolved, err := repo.ResolvePinned(head, representativeNode)
	if err != nil {
		t.Fatalf("ResolvePinned %q: %v", representativeNode, err)
	}
	if resolved.Commit != head || !resolved.Node.Equal(Node{
		ID: representativeNode, Title: fmt.Sprintf("Stress node %06d", config.nodeCount-1),
	}) {
		t.Fatalf("ResolvePinned %q = %#v", representativeNode, resolved)
	}
	if _, err := repo.ResolvePinned(base, SeedNodeID); err != nil {
		t.Fatalf("ResolvePinned retained seed commit: %v", err)
	}
}

func findStressNode(nodes []Node, id string) Node {
	for _, node := range nodes {
		if node.ID == id {
			return node
		}
	}
	return Node{}
}

func findStressEdge(edges []Edge, id string) Edge {
	for _, edge := range edges {
		if edge.ID == id {
			return edge
		}
	}
	return Edge{}
}

func TestStressDeepHistoryDurableLifecycle(t *testing.T) {
	config := stressConfigForTest(t)
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}

	seed, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch seed: %v", err)
	}
	expectedCommits := []ObjectID{seed}
	finalTitle := ""
	for index := 0; index < config.commitCount; index++ {
		finalTitle = fmt.Sprintf("Deep history revision %06d", index)
		if _, err := repo.StageMutationBatch(StageMutationRequest{
			Branch:     "main",
			Operations: []MutationOperation{{Action: "update", Entity: "node", ID: SeedNodeID, Title: finalTitle}},
		}); err != nil {
			t.Fatalf("stage revision %d: %v", index, err)
		}
		committed, err := repo.CommitStagedMutations("main")
		if err != nil {
			t.Fatalf("commit revision %d: %v", index, err)
		}
		expectedCommits = append(expectedCommits, committed.Commit)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository: %v", err)
	}
	defer closeTestRepository(t, reopened)

	head, err := reopened.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch reopened: %v", err)
	}
	if head != expectedCommits[len(expectedCommits)-1] {
		t.Fatalf("reopened head = %q, want %q", head, expectedCommits[len(expectedCommits)-1])
	}

	request := HistoryRequest{
		Commit:           head,
		EntityID:         SeedNodeID,
		MaxRows:          3,
		MaxResponseBytes: 1 << 20,
	}
	entries := make([]HistoryEntry, 0, len(expectedCommits))
	continuations := make(map[string]struct{})
	for {
		page, err := reopened.History(request)
		if err != nil {
			t.Fatalf("History continuation %q: %v", request.ContinuationToken, err)
		}
		if len(page.Entries) == 0 || len(page.Entries) > request.MaxRows {
			t.Fatalf("history page = %#v, want 1 through %d entries", page, request.MaxRows)
		}
		for _, entry := range page.Entries {
			expectedIndex := len(expectedCommits) - 1 - len(entries)
			if entry.Commit != expectedCommits[expectedIndex] {
				t.Fatalf("history entry %d commit = %q, want newest-first %q", len(entries), entry.Commit, expectedCommits[expectedIndex])
			}
			entries = append(entries, entry)
		}
		if page.ContinuationToken == "" {
			break
		}
		if _, seen := continuations[page.ContinuationToken]; seen {
			t.Fatalf("repeated history continuation %q", page.ContinuationToken)
		}
		continuations[page.ContinuationToken] = struct{}{}
		request.ContinuationToken = page.ContinuationToken
	}
	if len(entries) != len(expectedCommits) {
		t.Fatalf("history entries = %d, want complete coverage of %d commits", len(entries), len(expectedCommits))
	}
	uniqueCommits := make(map[ObjectID]struct{}, len(entries))
	for _, entry := range entries {
		if _, exists := uniqueCommits[entry.Commit]; exists {
			t.Fatalf("duplicate history commit %q", entry.Commit)
		}
		uniqueCommits[entry.Commit] = struct{}{}
	}

	resolved, err := reopened.ResolvePinned(head, SeedNodeID)
	if err != nil {
		t.Fatalf("ResolvePinned final head: %v", err)
	}
	if resolved.Node.Title != finalTitle {
		t.Fatalf("resolved title = %q, want %q", resolved.Node.Title, finalTitle)
	}

	result, err := reopened.Fsck()
	if err != nil {
		t.Fatalf("Fsck reopened repository: %v, result = %#v", err, result)
	}
	if !result.Valid || len(result.Diagnostics) != 0 || !reflect.DeepEqual(result.Branches, []string{"main"}) ||
		result.Commits != len(expectedCommits) || result.Snapshots != len(expectedCommits) {
		t.Fatalf("Fsck result = %#v, want valid durable chain with %d commits and snapshots", result, len(expectedCommits))
	}
}
