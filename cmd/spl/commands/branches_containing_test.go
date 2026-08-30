package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
)

func TestBranchesContainingCLIAndToolReturnEquivalentContracts(t *testing.T) {
	repo := repository.NewSeedRepository()
	for _, name := range []string{"feature", "review"} {
		if _, err := repo.CreateBranch(name, repository.BranchSource{Branch: "main"}); err != nil {
			t.Fatalf("create %s branch: %v", name, err)
		}
	}
	tool := resolve.NewResolveTool(repo)
	var output bytes.Buffer
	command := NewBranchesContainingCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
	command.SetOut(&output)
	command.SetArgs([]string{
		"--entity-id", repository.SeedNodeID,
		"--max-rows", "1", "--max-response-bytes", "5000", "--timeout", "1s",
	})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute containment: %v", err)
	}
	var cliResult resolve.BranchesContainingResult
	if err := json.Unmarshal(output.Bytes(), &cliResult); err != nil {
		t.Fatalf("decode containment: %v", err)
	}
	if cliResult.Completion.ResponseBytes != output.Len() {
		t.Fatalf("CLI containment bytes = %d, want responseBytes %d", output.Len(), cliResult.Completion.ResponseBytes)
	}
	rows, responseBytes := 1, 5000
	timeout := time.Second
	toolResult, err := tool.SPLBranchesContainingPage(context.Background(), resolve.BranchesContainingRequest{
		Selector: resolve.ContainmentSelector{EntityID: repository.SeedNodeID},
		Budget:   resolve.QueryBudgetRequest{MaxRows: &rows, MaxResponseBytes: &responseBytes, Timeout: &timeout},
	})
	if err != nil {
		t.Fatalf("SPLBranchesContaining: %v", err)
	}
	if !reflect.DeepEqual(comparableBranchesContainingResult(cliResult), comparableBranchesContainingResult(toolResult)) ||
		!reflect.DeepEqual(cliResult.Branches, []string{"feature"}) {
		t.Fatalf("CLI containment %#v, tool containment %#v", cliResult, toolResult)
	}
	if cliResult.ContinuationToken == "" {
		t.Fatal("CLI containment did not return a continuation token")
	}

	output.Reset()
	command = NewBranchesContainingCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
	command.SetOut(&output)
	command.SetArgs([]string{
		"--entity-id", repository.SeedNodeID,
		"--max-rows", "1", "--max-response-bytes", "5000", "--timeout", "1s",
		"--continuation", cliResult.ContinuationToken,
	})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute continued containment: %v", err)
	}
	var continuedCLIResult resolve.BranchesContainingResult
	if err := json.Unmarshal(output.Bytes(), &continuedCLIResult); err != nil {
		t.Fatalf("decode continued containment: %v", err)
	}
	toolResult, err = tool.SPLBranchesContainingPage(context.Background(), resolve.BranchesContainingRequest{
		Selector:          resolve.ContainmentSelector{EntityID: repository.SeedNodeID},
		ContinuationToken: cliResult.ContinuationToken,
		Budget:            resolve.QueryBudgetRequest{MaxRows: &rows, MaxResponseBytes: &responseBytes, Timeout: &timeout},
	})
	if err != nil {
		t.Fatalf("continued SPLBranchesContainingPage: %v", err)
	}
	if !reflect.DeepEqual(comparableBranchesContainingResult(continuedCLIResult), comparableBranchesContainingResult(toolResult)) {
		t.Fatalf("continued CLI containment %#v, tool containment %#v", continuedCLIResult, toolResult)
	}
}

func comparableBranchesContainingResult(result resolve.BranchesContainingResult) resolve.BranchesContainingResult {
	result.Completion = resolve.QueryCompletionMetadata{}
	return result
}
