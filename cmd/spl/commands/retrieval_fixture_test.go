package commands

import (
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
)

func retrievalCommandRepository(t *testing.T) *repository.Repository {
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
			{Action: "add", Entity: "node", ID: "seed-a", Title: "evidence alpha", Labels: []string{"Seed"}},
			{Action: "add", Entity: "node", ID: "seed-b", Title: "evidence beta", Labels: []string{"Seed"}},
			{Action: "add", Entity: "node", ID: "related", Title: "related"},
			{Action: "add", Entity: "edge", ID: "related-edge", Source: "seed-a", Target: "related", Type: "RELATED"},
		},
	}); err != nil {
		t.Fatalf("stage retrieval nodes: %v", err)
	}
	if _, err := repo.CommitStagedMutations("main"); err != nil {
		t.Fatalf("commit retrieval nodes: %v", err)
	}
	return repo
}
