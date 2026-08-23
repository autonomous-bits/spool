package resolve

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/autonomous-bits/spool/internal/repository"
)

const (
	defaultStressNodeCount = 1_000
	defaultStressEdgeCount = 5_000
)

type retrievalStressConfig struct {
	nodeCount int
	edgeCount int
}

func retrievalStressConfigForTest(t *testing.T) retrievalStressConfig {
	t.Helper()
	if os.Getenv("SPOOL_STRESS") != "1" {
		t.Skip("stress tests disabled; set SPOOL_STRESS=1 to enable")
	}
	return retrievalStressConfig{
		nodeCount: stressPositiveIntEnv(t, "SPOOL_STRESS_NODES", defaultStressNodeCount),
		edgeCount: stressPositiveIntEnv(t, "SPOOL_STRESS_EDGES", defaultStressEdgeCount),
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

func TestStressRetrievalQueries(t *testing.T) {
	config := retrievalStressConfigForTest(t)
	recorder := repository.NewPerformanceRecorder()
	endFixture := recorder.Measure("retrieval_fixture_construction")
	repo, head := retrievalStressRepository(t, config)
	endFixture()
	pageSize := minInt(127, config.nodeCount)
	budget := QueryBudget{
		MaxRows:          pageSize,
		MaxResponseBytes: 1 << 20,
		MaxDepth:         1,
		MaxVisited:       minInt(256, config.nodeCount+1),
		Timeout:          time.Minute,
	}
	tool := NewResolveToolWithOptions(repo, Options{QueryBudget: &budget})
	expectCapacityExhausted := 1+minInt(config.nodeCount, config.edgeCount) > budget.MaxVisited

	searchIDs := make([]string, 0, config.nodeCount)
	searchToken := ""
	for {
		endQuery := recorder.Measure("retrieval_query_execution_search")
		result, err := tool.SPLSearch(context.Background(), SearchRequest{
			Selector:          SnapshotSelector{Branch: "main"},
			Query:             "querycorpus",
			ContinuationToken: searchToken,
		})
		endQuery()
		if err != nil {
			t.Fatalf("SPLSearch page %d: %v", len(searchIDs)/pageSize, err)
		}
		assertStressQueryMetadata(t, result.Snapshot, result.Projection, result.Budget, result.Completion, result)
		for _, match := range result.Matches {
			searchIDs = append(searchIDs, match.Node.ID)
		}
		searchToken = result.ContinuationToken
		if searchToken == "" {
			break
		}
	}
	assertStressQueryIDs(t, "search", searchIDs, config.nodeCount)

	filterIDs := make([]string, 0, config.nodeCount)
	filterToken := ""
	for {
		endQuery := recorder.Measure("retrieval_query_execution_filter")
		result, err := tool.SPLFilter(context.Background(), FilterRequest{
			Selector:          SnapshotSelector{Branch: "main"},
			Labels:            []string{"Task"},
			ContinuationToken: filterToken,
		})
		endQuery()
		if err != nil {
			t.Fatalf("SPLFilter page %d: %v", len(filterIDs)/pageSize, err)
		}
		assertStressQueryMetadata(t, result.Snapshot, result.Projection, result.Budget, result.Completion, result)
		for _, node := range result.Nodes {
			filterIDs = append(filterIDs, node.ID)
		}
		filterToken = result.ContinuationToken
		if filterToken == "" {
			break
		}
	}
	assertStressQueryIDs(t, "filter", filterIDs, config.nodeCount)

	endSearchExpand := recorder.Measure("retrieval_query_execution_search_expand")
	expanded, err := tool.SPLSearchExpand(context.Background(), SearchExpandRequest{
		Selector:  SnapshotSelector{Branch: "main"},
		Seeds:     SeedSelector{Query: "queryroot"},
		Direction: DirectionOut,
		EdgeTypes: []string{"STRESS_LINK"},
	})
	endSearchExpand()
	if err != nil {
		t.Fatalf("SPLSearchExpand: %v", err)
	}
	assertStressQueryMetadata(t, expanded.Snapshot, expanded.Projection, expanded.Budget, expanded.Completion, expanded)
	assertStressContextResult(t, "search-expand", expanded, budget, expectCapacityExhausted)

	endContext := recorder.Measure("retrieval_query_execution_context")
	contextResult, err := tool.SPLContext(context.Background(), ContextRequest{
		Selector:  SnapshotSelector{Branch: "main"},
		Seeds:     SeedSelector{Labels: []string{"Seed"}},
		Direction: DirectionOut,
		EdgeTypes: []string{"STRESS_LINK"},
	})
	endContext()
	if err != nil {
		t.Fatalf("SPLContext: %v", err)
	}
	assertStressQueryMetadata(t, contextResult.Snapshot, contextResult.Projection, contextResult.Budget, contextResult.Completion, contextResult)
	assertStressContextResult(t, "context", contextResult, budget, expectCapacityExhausted)

	if expanded.Snapshot.Commit != string(head) || contextResult.Snapshot.Commit != string(head) {
		t.Fatalf("retrieval commit provenance = %q/%q, want %q", expanded.Snapshot.Commit, contextResult.Snapshot.Commit, head)
	}
	assertRetrievalPerformancePhases(t, recorder, []string{
		"retrieval_fixture_construction",
		"retrieval_query_execution_search",
		"retrieval_query_execution_filter",
		"retrieval_query_execution_search_expand",
		"retrieval_query_execution_context",
	})
	reportRetrievalPerformance(t, recorder)
}

func assertRetrievalPerformancePhases(t *testing.T, recorder *repository.PerformanceRecorder, expected []string) {
	t.Helper()
	actual := make(map[string]repository.PerformancePhase)
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

func reportRetrievalPerformance(t *testing.T, recorder *repository.PerformanceRecorder) {
	t.Helper()
	encoded, err := json.Marshal(struct {
		Phases []repository.PerformancePhase `json:"phases"`
	}{Phases: recorder.Phases()})
	if err != nil {
		t.Fatalf("marshal performance report: %v", err)
	}
	t.Logf("performance diagnostics: %s", encoded)
}

func retrievalStressRepository(t testing.TB, config retrievalStressConfig) (*repository.Repository, repository.ObjectID) {
	t.Helper()
	stateDir := t.TempDir()
	repo, err := repository.InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	operations := make([]repository.MutationOperation, 0, config.nodeCount+config.edgeCount+1)
	operations = append(operations, repository.MutationOperation{
		Action: "update", Entity: "node", ID: repository.SeedNodeID, Title: "queryroot",
		Labels: []string{"Seed"},
		Properties: map[string]repository.PropertyValue{
			"search": repository.StringPropertyValue("queryroot"),
			"rank":   repository.IntegerPropertyValue(-1),
		},
	})
	for index := 0; index < config.nodeCount; index++ {
		operations = append(operations, repository.MutationOperation{
			Action: "add", Entity: "node", ID: stressQueryNodeID(index), Title: fmt.Sprintf("query corpus node %06d", index),
			Labels: []string{"Task"},
			Properties: map[string]repository.PropertyValue{
				"search": repository.StringPropertyValue("querycorpus"),
				"rank":   repository.IntegerPropertyValue(int64(index % 10)),
			},
		})
	}
	for index := 0; index < config.edgeCount; index++ {
		source, target := repository.SeedNodeID, stressQueryNodeID(index%config.nodeCount)
		if index >= config.nodeCount {
			source = stressQueryNodeID(index % config.nodeCount)
			target = stressQueryNodeID((index*17 + 1) % config.nodeCount)
		}
		operations = append(operations, repository.MutationOperation{
			Action: "add", Entity: "edge", ID: stressQueryEdgeID(index),
			Source: source, Target: target, Type: "STRESS_LINK",
		})
	}
	schema := []byte(`
version = 2
[[node]]
label = "Seed"
[[node.property]]
key = "search"
required = true
indexed = true
types = ["string"]
[[node.property]]
key = "rank"
required = true
indexed = true
types = ["integer"]
[[node]]
label = "Task"
[[node.property]]
key = "search"
required = true
indexed = true
types = ["string"]
[[node.property]]
key = "rank"
required = true
indexed = true
types = ["integer"]
[[edge]]
type = "STRESS_LINK"
source_labels = ["Seed", "Task"]
target_labels = ["Task"]
`)
	if _, err := repo.StageSchemaMigration(repository.SchemaMigrationRequest{
		Branch: "main", SchemaTOML: schema, Operations: operations,
	}); err != nil {
		t.Fatalf("StageSchemaMigration: %v", err)
	}
	committed, err := repo.CommitStagedMutationBatch(repository.CommitStagedMutationRequest{Branch: "main"})
	if err != nil {
		t.Fatalf("CommitStagedMutationBatch: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close before reopen: %v", err)
	}
	reopened, err := repository.OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository: %v", err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return reopened, committed.Commit
}

func assertStressQueryMetadata(t *testing.T, snapshot SnapshotMetadata, projection ProjectionMetadata, budget QueryBudget, completion QueryCompletionMetadata, result any) {
	t.Helper()
	if snapshot.Branch != "main" || snapshot.Commit == "" || snapshot.Root == "" || projection.State != "ready" {
		t.Fatalf("query metadata = %#v/%#v", snapshot, projection)
	}
	if completion.ResponseBytes == 0 || completion.ResponseBytes > budget.MaxResponseBytes {
		t.Fatalf("query completion = %#v, budget = %#v", completion, budget)
	}
	encoded, err := json.Marshal(result)
	if err != nil || len(encoded) != completion.ResponseBytes {
		t.Fatalf("query response bytes = %d/%d, error = %v", len(encoded), completion.ResponseBytes, err)
	}
}

func assertStressQueryIDs(t *testing.T, query string, ids []string, want int) {
	t.Helper()
	if len(ids) != want {
		t.Fatalf("%s match count = %d, want %d", query, len(ids), want)
	}
	seen := make(map[string]struct{}, len(ids))
	for index, id := range ids {
		if _, exists := seen[id]; exists {
			t.Fatalf("%s duplicate ID %q", query, id)
		}
		seen[id] = struct{}{}
		if wantID := stressQueryNodeID(index); id != wantID {
			t.Fatalf("%s ID %d = %q, want %q", query, index, id, wantID)
		}
	}
}

func assertStressContextResult(t *testing.T, query string, result SearchExpandResult, budget QueryBudget, expectCapacityExhausted bool) {
	t.Helper()
	if len(result.Evidence) != 1 || result.Evidence[0].Node.ID != repository.SeedNodeID {
		t.Fatalf("%s evidence = %#v, want root seed", query, result.Evidence)
	}
	if len(result.Nodes) == 0 || result.Nodes[0].Node.ID != repository.SeedNodeID {
		t.Fatalf("%s nodes = %#v, want root first", query, result.Nodes)
	}
	if len(result.Nodes) > budget.MaxRows || result.Completion.Visited > budget.MaxVisited {
		t.Fatalf("%s bounds = %#v, budget = %#v", query, result.Completion, budget)
	}
	if expectCapacityExhausted && !result.CapacityExhausted {
		t.Fatalf("%s did not report exhausted traversal capacity: %#v", query, result)
	}
	for _, path := range result.Paths {
		if path.NodeID == repository.SeedNodeID {
			continue
		}
		if len(path.NodeIDs) != 2 || path.NodeIDs[0] != repository.SeedNodeID || len(path.EdgeIDs) != 1 {
			t.Fatalf("%s non-root path = %#v", query, path)
		}
	}
}

func stressQueryNodeID(index int) string {
	return fmt.Sprintf("stress-query-node-%06d", index)
}

func stressQueryEdgeID(index int) string {
	return fmt.Sprintf("stress-query-edge-%020d", index)
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
