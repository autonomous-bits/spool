package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
)

func TestHistoryCLIAndToolReturnEquivalentContracts(t *testing.T) {
	repo := repository.NewSeedRepository()
	if _, err := repo.StageMutationBatch(repository.StageMutationRequest{
		Branch: "main", Operations: []repository.MutationOperation{{Action: "update", Entity: "node", ID: repository.SeedNodeID, Title: "Updated"}},
	}); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if _, err := repo.CommitStagedMutationBatch(repository.CommitStagedMutationRequest{Branch: "main", Author: "alice", Message: "update"}); err != nil {
		t.Fatalf("commit: %v", err)
	}
	tool := resolve.NewResolveTool(repo)
	var output bytes.Buffer
	command := NewHistoryCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"--branch", "main", "--entity-id", repository.SeedNodeID})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute history: %v", err)
	}
	var cliResult repository.HistoryResult
	if err := json.Unmarshal(output.Bytes(), &cliResult); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	toolResult, err := tool.EDGHistory(context.Background(), resolve.HistoryRequest{
		Selector: repository.DiffSelector{Branch: "main"}, EntityID: repository.SeedNodeID,
	})
	if err != nil {
		t.Fatalf("EDGHistory: %v", err)
	}
	if !reflect.DeepEqual(cliResult, toolResult) {
		t.Fatalf("CLI history %#v, tool history %#v", cliResult, toolResult)
	}
}

func TestBranchesContainingCLIAndToolReturnEquivalentContracts(t *testing.T) {
	repo := repository.NewSeedRepository()
	tool := resolve.NewResolveTool(repo)
	var output bytes.Buffer
	command := NewBranchesContainingCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"--entity-id", repository.SeedNodeID})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute containment: %v", err)
	}
	var cliResult repository.BranchContainmentResult
	if err := json.Unmarshal(output.Bytes(), &cliResult); err != nil {
		t.Fatalf("decode containment: %v", err)
	}
	toolResult, err := tool.EDGBranchesContaining(context.Background(), resolve.ContainmentSelector{EntityID: repository.SeedNodeID})
	if err != nil {
		t.Fatalf("EDGBranchesContaining: %v", err)
	}
	if !reflect.DeepEqual(cliResult, toolResult) || !reflect.DeepEqual(cliResult.Branches, []string{"main"}) {
		t.Fatalf("CLI containment %#v, tool containment %#v", cliResult, toolResult)
	}
}
