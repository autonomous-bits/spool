package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
)

func TestHistoryCLIAndToolReturnEquivalentContracts(t *testing.T) {
	repo := newTestSeedRepository(t)
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
	command.SetArgs([]string{
		"--branch", "main", "--entity-id", repository.SeedNodeID,
		"--max-rows", "1", "--max-response-bytes", "5000", "--timeout", "1s",
	})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute history: %v", err)
	}
	var cliResult resolve.HistoryResult
	if err := json.Unmarshal(output.Bytes(), &cliResult); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if cliResult.Completion.ResponseBytes != output.Len() {
		t.Fatalf("CLI history bytes = %d, want responseBytes %d", output.Len(), cliResult.Completion.ResponseBytes)
	}
	rows, responseBytes := 1, 5000
	timeout := time.Second
	toolResult, err := tool.SPLHistory(context.Background(), resolve.HistoryRequest{
		Selector: snapshotSelector("main", ""), EntityID: repository.SeedNodeID,
		Budget: resolve.QueryBudgetRequest{MaxRows: &rows, MaxResponseBytes: &responseBytes, Timeout: &timeout},
	})
	if err != nil {
		t.Fatalf("SPLHistory: %v", err)
	}
	if !reflect.DeepEqual(comparableHistoryResult(cliResult), comparableHistoryResult(toolResult)) {
		t.Fatalf("CLI history %#v, tool history %#v", cliResult, toolResult)
	}
	if cliResult.ContinuationToken == "" {
		t.Fatal("CLI history did not return a continuation token")
	}

	output.Reset()
	command = NewHistoryCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
	command.SetOut(&output)
	command.SetArgs([]string{
		"--branch", "main", "--entity-id", repository.SeedNodeID,
		"--max-rows", "1", "--max-response-bytes", "5000", "--timeout", "1s",
		"--continuation", cliResult.ContinuationToken,
	})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute continued history: %v", err)
	}
	var continuedCLIResult resolve.HistoryResult
	if err := json.Unmarshal(output.Bytes(), &continuedCLIResult); err != nil {
		t.Fatalf("decode continued history: %v", err)
	}
	toolResult, err = tool.SPLHistory(context.Background(), resolve.HistoryRequest{
		Selector:          snapshotSelector("main", ""),
		EntityID:          repository.SeedNodeID,
		ContinuationToken: cliResult.ContinuationToken,
		Budget:            resolve.QueryBudgetRequest{MaxRows: &rows, MaxResponseBytes: &responseBytes, Timeout: &timeout},
	})
	if err != nil {
		t.Fatalf("continued SPLHistory: %v", err)
	}
	if !reflect.DeepEqual(comparableHistoryResult(continuedCLIResult), comparableHistoryResult(toolResult)) {
		t.Fatalf("continued CLI history %#v, tool history %#v", continuedCLIResult, toolResult)
	}
}

func TestHistoryCLIRequiresBranchSelector(t *testing.T) {
	command := NewHistoryCommand(func() (*resolve.ResolveTool, error) {
		return resolve.NewResolveTool(newTestSeedRepository(t)), nil
	})
	command.SetArgs([]string{"--entity-id", repository.SeedNodeID})

	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), `required flag(s) "branch" not set`) {
		t.Fatalf("history error = %v, want missing branch", err)
	}
}

func TestHistoryCLIAndToolRejectUnreachableCommitWithSameCategory(t *testing.T) {
	repo := newTestSeedRepository(t)
	if _, err := repo.CreateBranch("feature", repository.BranchSource{Branch: "main"}); err != nil {
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
	_, mcpErr := tool.SPLHistory(context.Background(), resolve.HistoryRequest{
		Selector: resolve.SnapshotSelector{Branch: "main", Commit: &commit}, EntityID: repository.SeedNodeID,
	})
	if !errors.Is(cliErr, resolve.ErrUnsupportedCommit) || !errors.Is(mcpErr, resolve.ErrUnsupportedCommit) {
		t.Fatalf("CLI/tool errors = %v/%v, want ErrUnsupportedCommit", cliErr, mcpErr)
	}
	if output.Len() != 0 {
		t.Fatalf("CLI wrote output for error: %q", output.String())
	}
}

func comparableHistoryResult(result resolve.HistoryResult) resolve.HistoryResult {
	result.Completion = resolve.QueryCompletionMetadata{}
	return result
}
