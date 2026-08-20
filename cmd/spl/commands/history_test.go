package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/repository/branch"
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
	var cliResult resolve.HistoryResult
	if err := json.Unmarshal(output.Bytes(), &cliResult); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	toolResult, err := tool.EDGHistory(context.Background(), resolve.HistoryRequest{
		Selector: snapshotSelector("main", ""), EntityID: repository.SeedNodeID,
	})
	if err != nil {
		t.Fatalf("EDGHistory: %v", err)
	}
	if !reflect.DeepEqual(cliResult, toolResult) {
		t.Fatalf("CLI history %#v, tool history %#v", cliResult, toolResult)
	}
}

func TestHistoryCLIRequiresBranchSelector(t *testing.T) {
	command := NewHistoryCommand(func() (*resolve.ResolveTool, error) {
		return resolve.NewResolveTool(repository.NewSeedRepository()), nil
	})
	command.SetArgs([]string{"--entity-id", repository.SeedNodeID})

	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), `required flag(s) "branch" not set`) {
		t.Fatalf("history error = %v, want missing branch", err)
	}
}

func TestHistoryCLIAndToolRejectUnreachableCommitWithSameCategory(t *testing.T) {
	repo := repository.NewSeedRepository()
	if _, err := repo.CreateBranch("feature", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	feature, err := repo.AdvanceBranch("feature")
	if err != nil {
		t.Fatalf("AdvanceBranch feature: %v", err)
	}
	tool := resolve.NewResolveTool(repo)
	commit := string(feature)
	var output bytes.Buffer
	command := NewHistoryCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"--branch", "main", "--commit", commit, "--entity-id", repository.SeedNodeID})

	cliErr := command.Execute()
	_, mcpErr := tool.EDGHistory(context.Background(), resolve.HistoryRequest{
		Selector: resolve.SnapshotSelector{Branch: "main", Commit: &commit}, EntityID: repository.SeedNodeID,
	})
	if !errors.Is(cliErr, resolve.ErrUnsupportedCommit) || !errors.Is(mcpErr, resolve.ErrUnsupportedCommit) {
		t.Fatalf("CLI/tool errors = %v/%v, want ErrUnsupportedCommit", cliErr, mcpErr)
	}
	if output.Len() != 0 {
		t.Fatalf("CLI wrote output for error: %q", output.String())
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
