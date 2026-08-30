package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
)

func TestFilterCLIAndToolReturnEquivalentJSON(t *testing.T) {
	repo := retrievalCommandRepository(t)
	tool := resolve.NewResolveTool(repo)
	rows, bytesLimit := 10, 100_000
	timeout := time.Second
	budget := resolve.QueryBudgetRequest{
		MaxRows: &rows, MaxResponseBytes: &bytesLimit, Timeout: &timeout,
	}

	var output bytes.Buffer
	command := NewFilterCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"--branch", "main", "--label", "Seed", "--max-rows", "10", "--max-response-bytes", "100000", "--timeout", "1s"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute filter: %v", err)
	}
	var actual resolve.FilterResult
	if err := json.Unmarshal(output.Bytes(), &actual); err != nil {
		t.Fatalf("decode filter: %v", err)
	}
	if actual.Completion.ResponseBytes != output.Len() {
		t.Fatalf("filter response bytes = %d, want %d", actual.Completion.ResponseBytes, output.Len())
	}

	want, err := tool.SPLFilter(context.Background(), resolve.FilterRequest{
		Selector: resolve.SnapshotSelector{Branch: "main"}, Labels: []string{"Seed"}, Budget: budget,
	})
	if err != nil {
		t.Fatalf("SPLFilter: %v", err)
	}
	actual.Completion = resolve.QueryCompletionMetadata{}
	want.Completion = resolve.QueryCompletionMetadata{}
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("CLI filter result %#v, tool result %#v", actual, want)
	}
}

func TestFilterCLIEnforcesBranchAndProjectionConstraints(t *testing.T) {
	repo := retrievalCommandRepository(t)
	tool := resolve.NewResolveTool(repo)
	historical, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("pin branch: %v", err)
	}
	if _, err := repo.AdvanceBranch("main"); err != nil {
		t.Fatalf("advance branch: %v", err)
	}

	var output bytes.Buffer
	filter := NewFilterCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
	filter.SetOut(&output)
	filter.SetArgs([]string{"--branch", "main", "--commit", string(historical), "--label", "Seed"})
	cliErr := filter.Execute()
	commit := string(historical)
	_, toolErr := tool.SPLFilter(context.Background(), resolve.FilterRequest{
		Selector: resolve.SnapshotSelector{Branch: "main", Commit: &commit}, Labels: []string{"Seed"},
	})
	if !errors.Is(cliErr, repository.ErrHistoricalProjectionUnsupported) || !errors.Is(toolErr, repository.ErrHistoricalProjectionUnsupported) {
		t.Fatalf("CLI/tool errors = %v/%v, want historical projection constraint", cliErr, toolErr)
	}
	if output.Len() != 0 {
		t.Fatalf("CLI emitted output on projection error: %q", output.String())
	}
}
