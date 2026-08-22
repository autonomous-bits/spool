package resolve

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/autonomous-bits/spool/internal/contextual"
	"github.com/autonomous-bits/spool/internal/repository"
)

func TestSPLRetrievalUsesPublicProvenanceAndNarrowedBudget(t *testing.T) {
	repo := retrievalAdapterRepository(t)
	configured := QueryBudget{
		MaxRows: 1, MaxResponseBytes: 100_000, MaxDepth: 2, MaxVisited: 2, Timeout: time.Second,
	}
	tool := NewResolveToolWithOptions(repo, Options{QueryBudget: &configured})
	rows, visited := 10, 10

	search, err := tool.SPLSearch(context.Background(), SearchRequest{
		Selector: SnapshotSelector{Branch: "main"}, Query: "evidence",
		Budget: QueryBudgetRequest{MaxRows: &rows, MaxVisited: &visited},
	})
	if err != nil {
		t.Fatalf("SPLSearch: %v", err)
	}
	if search.Budget != configured || len(search.Matches) != 1 || search.Snapshot.Branch != "main" ||
		search.Snapshot.Commit == "" || search.Snapshot.Root == "" || search.Projection.State != "ready" {
		t.Fatalf("search result = %#v", search)
	}

	expanded, err := tool.SPLSearchExpand(context.Background(), SearchExpandRequest{
		Selector: SnapshotSelector{Branch: "main"}, Seeds: contextual.SeedSelector{Query: "evidence"},
		SeedLimit: 1, Direction: contextual.DirectionOut, EdgeTypes: []string{"RELATED"},
		Budget: QueryBudgetRequest{MaxRows: &rows, MaxVisited: &visited},
	})
	if err != nil {
		t.Fatalf("SPLSearchExpand: %v", err)
	}
	if expanded.Snapshot != search.Snapshot || expanded.Projection != search.Projection ||
		expanded.Budget != configured || len(expanded.Evidence) != 1 || expanded.Completion.ResponseBytes == 0 {
		t.Fatalf("search expand result = %#v", expanded)
	}
}

func TestSPLRetrievalRejectsHistoricalProjection(t *testing.T) {
	repo := retrievalAdapterRepository(t)
	historical, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}
	if _, err := repo.AdvanceBranch("main"); err != nil {
		t.Fatalf("AdvanceBranch: %v", err)
	}
	commit := string(historical)
	_, err = NewResolveTool(repo).SPLContext(context.Background(), ContextRequest{
		Selector: SnapshotSelector{Branch: "main", Commit: &commit},
		Seeds:    contextual.SeedSelector{Query: "evidence"}, Direction: contextual.DirectionOut,
	})
	if !errors.Is(err, repository.ErrHistoricalProjectionUnsupported) {
		t.Fatalf("SPLContext error = %v, want historical projection constraint", err)
	}
}

func retrievalAdapterRepository(t *testing.T) *repository.Repository {
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
	if _, err := repo.StageMutationBatch(repository.StageMutationRequest{
		Branch: "main",
		Operations: []repository.MutationOperation{
			{Action: "add", Entity: "node", ID: "evidence-a", Title: "evidence alpha"},
			{Action: "add", Entity: "node", ID: "evidence-b", Title: "evidence beta"},
			{Action: "add", Entity: "node", ID: "related", Title: "related"},
			{Action: "add", Entity: "edge", ID: "related-edge", Source: "evidence-a", Target: "related", Type: "RELATED"},
		},
	}); err != nil {
		t.Fatalf("stage retrieval nodes: %v", err)
	}
	if _, err := repo.CommitStagedMutations("main"); err != nil {
		t.Fatalf("commit retrieval nodes: %v", err)
	}
	return repo
}
