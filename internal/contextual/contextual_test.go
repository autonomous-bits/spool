package contextual

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/autonomous-bits/spool/internal/repository"
)

func TestSearchExpandUsesRankedSeedsAndDeterministicShortestPaths(t *testing.T) {
	repo, head := contextualRepository(t)
	service := NewService(repo)

	result, err := service.SearchExpand(context.Background(), SearchExpandRequest{
		Branch: "main", Seeds: SeedSelector{Query: "evidence"},
		Direction: DirectionOut, EdgeTypes: []string{"ALLOWED"},
		Budget: testBudget(10, 10),
	})
	if err != nil {
		t.Fatalf("SearchExpand: %v", err)
	}
	if result.Snapshot.Commit != head || result.Projection.State != "ready" || !result.Completion.Complete {
		t.Fatalf("metadata = %#v", result)
	}
	if got, want := evidenceIDs(result.Evidence), []string{"seed-a", "seed-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("evidence order = %#v, want %#v", got, want)
	}
	if got, want := contextIDs(result.Nodes), []string{"seed-a", "seed-b", "node-c", "node-d"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expanded nodes = %#v, want %#v", got, want)
	}
	if got, want := edgeIDs(result.Edges), []string{"edge-a-c", "edge-a-d", "edge-b-c", "edge-cycle"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("context edges = %#v, want %#v", got, want)
	}
	if path := pathFor(result.Paths, "node-d"); !reflect.DeepEqual(path.NodeIDs, []string{"seed-a", "node-d"}) ||
		!reflect.DeepEqual(path.EdgeIDs, []string{"edge-a-d"}) {
		t.Fatalf("node-d shortest supporting path = %#v", path)
	}
	if result.Completion.Visited != 4 || result.Completion.ReturnedRows != 4 {
		t.Fatalf("completion counts = %#v", result.Completion)
	}
	data, err := json.Marshal(result)
	if err != nil || len(data) != result.Completion.ResponseBytes {
		t.Fatalf("response byte accounting = %d/%d (%v)", len(data), result.Completion.ResponseBytes, err)
	}
}

func TestContextHonorsDirectionFiltersAndBounds(t *testing.T) {
	repo, _ := contextualRepository(t)
	service := NewService(repo)

	result, err := service.Context(context.Background(), ContextRequest{
		Branch: "main", Seeds: SeedSelector{Labels: []string{"Seed"}},
		Direction: DirectionBoth, EdgeTypes: []string{"ALLOWED"},
		Budget: testBudget(2, 3),
	})
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if got, want := contextIDs(result.Nodes), []string{"seed-a", "node-c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("bounded context nodes = %#v, want %#v", got, want)
	}
	if !result.CapacityExhausted || !result.Completion.Truncated || result.Completion.Complete {
		t.Fatalf("bounded completion = %#v", result)
	}
	if got, want := edgeIDs(result.Edges), []string{"edge-a-c", "edge-cycle"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("bounded context edges = %#v, want %#v", got, want)
	}
}

func TestSearchExpandCapsSeedLimitAtRowBudget(t *testing.T) {
	repo, _ := contextualRepository(t)
	service := NewService(repo)

	result, err := service.SearchExpand(context.Background(), SearchExpandRequest{
		Branch: "main", Seeds: SeedSelector{Query: "evidence"}, SeedLimit: 10,
		Direction: DirectionOut, Budget: testBudget(1, 10),
	})
	if err != nil {
		t.Fatalf("SearchExpand: %v", err)
	}
	if got, want := evidenceIDs(result.Evidence), []string{"seed-a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("evidence = %#v, want %#v", got, want)
	}
	if len(result.Nodes) != 1 || !result.Completion.Truncated || result.Completion.Complete {
		t.Fatalf("bounded result = %#v", result)
	}
}

func TestSearchExpandSurfacesProjectionAndBudgetConstraints(t *testing.T) {
	repo, head := contextualRepository(t)
	service := NewService(repo)
	historical := head
	if _, err := repo.StageMutationBatch(repository.StageMutationRequest{
		Branch: "main", Operations: []repository.MutationOperation{{Action: "add", Entity: "node", ID: "later", Title: "later"}},
	}); err != nil {
		t.Fatalf("stage later commit: %v", err)
	}
	if _, err := repo.CommitStagedMutations("main"); err != nil {
		t.Fatalf("commit later: %v", err)
	}

	_, err := service.SearchExpand(context.Background(), SearchExpandRequest{
		Branch: "main", Commit: &historical, Seeds: SeedSelector{Query: "evidence"},
		Direction: DirectionOut, Budget: testBudget(10, 10),
	})
	if !errors.Is(err, repository.ErrHistoricalProjectionUnsupported) {
		t.Fatalf("historical request error = %v, want projection constraint", err)
	}
	_, err = service.SearchExpand(context.Background(), SearchExpandRequest{
		Branch: "main", Seeds: SeedSelector{Query: "evidence"}, Direction: DirectionOut,
		Budget: QueryBudget{MaxRows: 1, MaxVisited: 1, MaxDepth: 0, MaxResponseBytes: 1, Timeout: time.Second},
	})
	if !errors.Is(err, repository.ErrResponseBudgetTooSmall) {
		t.Fatalf("seed response budget error = %v", err)
	}
}

func contextualRepository(t *testing.T) (*repository.Repository, repository.ObjectID) {
	t.Helper()
	repo, err := repository.InitializeRepository(t.TempDir())
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	t.Cleanup(func() {
		if err := repo.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	operations := []repository.MutationOperation{
		{Action: "add", Entity: "node", ID: "seed-a", Title: "evidence alpha", Labels: []string{"Seed"}},
		{Action: "add", Entity: "node", ID: "seed-b", Title: "evidence beta", Labels: []string{"Other"}},
		{Action: "add", Entity: "node", ID: "node-c", Title: "connected"},
		{Action: "add", Entity: "node", ID: "node-d", Title: "alternate"},
		{Action: "add", Entity: "node", ID: "node-in", Title: "inbound only"},
		{Action: "add", Entity: "edge", ID: "edge-a-c", Source: "seed-a", Target: "node-c", Type: "ALLOWED"},
		{Action: "add", Entity: "edge", ID: "edge-b-c", Source: "seed-b", Target: "node-c", Type: "ALLOWED"},
		{Action: "add", Entity: "edge", ID: "edge-a-d", Source: "seed-a", Target: "node-d", Type: "ALLOWED"},
		{Action: "add", Entity: "edge", ID: "edge-denied", Source: "node-c", Target: "node-d", Type: "DENIED"},
		{Action: "add", Entity: "edge", ID: "edge-in", Source: "node-in", Target: "seed-a", Type: "ALLOWED"},
		{Action: "add", Entity: "edge", ID: "edge-cycle", Source: "node-c", Target: "seed-a", Type: "ALLOWED"},
	}
	if _, err := repo.StageMutationBatch(repository.StageMutationRequest{Branch: "main", Operations: operations}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	committed, err := repo.CommitStagedMutations("main")
	if err != nil {
		t.Fatalf("CommitStagedMutations: %v", err)
	}
	return repo, committed.Commit
}

func testBudget(rows, visited int) QueryBudget {
	return QueryBudget{MaxRows: rows, MaxVisited: visited, MaxDepth: 3, MaxResponseBytes: 1 << 20, Timeout: time.Second}
}

func evidenceIDs(evidence []Evidence) []string {
	ids := make([]string, len(evidence))
	for i, item := range evidence {
		ids[i] = item.Node.ID
	}
	return ids
}

func contextIDs(nodes []ContextNode) []string {
	ids := make([]string, len(nodes))
	for i, node := range nodes {
		ids[i] = node.Node.ID
	}
	return ids
}

func edgeIDs(edges []repository.Edge) []string {
	ids := make([]string, len(edges))
	for i, edge := range edges {
		ids[i] = edge.ID
	}
	return ids
}

func pathFor(paths []SupportingPath, nodeID string) SupportingPath {
	for _, path := range paths {
		if path.NodeID == nodeID {
			return path
		}
	}
	return SupportingPath{}
}
