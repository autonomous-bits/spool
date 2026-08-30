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

func TestDiffCLIAndMCPReturnEquivalentPayloads(t *testing.T) {
	repo := newTestSeedRepository(t)
	base, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("pin base: %v", err)
	}

	if _, err := repo.StageMutationBatch(repository.StageMutationRequest{
		Branch: "main",
		Operations: []repository.MutationOperation{
			{Action: "add", Entity: "node", ID: "node-2", Title: "Second node"},
			{Action: "add", Entity: "node", ID: "node-3", Title: "Third node"},
		},
	}); err != nil {
		t.Fatalf("stage: %v", err)
	}

	target, err := repo.CommitStagedMutations("main")
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	tool := resolve.NewResolveTool(repo)
	var output bytes.Buffer
	command := NewDiffCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
	command.SetOut(&output)
	command.SetArgs([]string{
		"--base-branch", "main", "--base-commit", string(base),
		"--target-branch", "main", "--target-commit", string(target.Commit),
		"--max-rows", "1", "--max-response-bytes", "5000", "--timeout", "1s",
	})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute CLI: %v", err)
	}
	var cliResult resolve.DiffResult
	if err := json.Unmarshal(output.Bytes(), &cliResult); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}
	if cliResult.Completion.ResponseBytes != output.Len() {
		t.Fatalf("CLI diff bytes = %d, want responseBytes %d", output.Len(), cliResult.Completion.ResponseBytes)
	}
	rows, responseBytes := 1, 5000
	timeout := time.Second
	mcpResult, err := tool.SPLDiff(context.Background(), resolve.DiffRequest{
		Base: snapshotSelector("main", string(base)), Target: snapshotSelector("main", string(target.Commit)),
		Budget: resolve.QueryBudgetRequest{MaxRows: &rows, MaxResponseBytes: &responseBytes, Timeout: &timeout},
	})
	if err != nil {
		t.Fatalf("SPLDiff: %v", err)
	}
	if !reflect.DeepEqual(comparableDiffResult(cliResult), comparableDiffResult(mcpResult)) {
		t.Fatalf("CLI result %#v does not match MCP result %#v", cliResult, mcpResult)
	}
	if cliResult.ContinuationToken == "" {
		t.Fatal("CLI diff did not return a continuation token")
	}

	output.Reset()
	command = NewDiffCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
	command.SetOut(&output)
	command.SetArgs([]string{
		"--base-branch", "main", "--base-commit", string(base),
		"--target-branch", "main", "--target-commit", string(target.Commit),
		"--max-rows", "1", "--max-response-bytes", "5000", "--timeout", "1s",
		"--continuation", cliResult.ContinuationToken,
	})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute continued CLI diff: %v", err)
	}
	var continuedCLIResult resolve.DiffResult
	if err := json.Unmarshal(output.Bytes(), &continuedCLIResult); err != nil {
		t.Fatalf("decode continued CLI result: %v", err)
	}
	mcpResult, err = tool.SPLDiff(context.Background(), resolve.DiffRequest{
		Base: snapshotSelector("main", string(base)), Target: snapshotSelector("main", string(target.Commit)),
		ContinuationToken: cliResult.ContinuationToken,
		Budget:            resolve.QueryBudgetRequest{MaxRows: &rows, MaxResponseBytes: &responseBytes, Timeout: &timeout},
	})
	if err != nil {
		t.Fatalf("continued SPLDiff: %v", err)
	}
	if !reflect.DeepEqual(comparableDiffResult(continuedCLIResult), comparableDiffResult(mcpResult)) {
		t.Fatalf("continued CLI result %#v does not match MCP result %#v", continuedCLIResult, mcpResult)
	}
}

func comparableDiffResult(result resolve.DiffResult) resolve.DiffResult {
	result.Completion = resolve.QueryCompletionMetadata{}
	return result
}

func TestDiffCLIAndMCPReturnEquivalentErrors(t *testing.T) {
	tool := resolve.NewResolveTool(newTestSeedRepository(t))
	for _, testCase := range []struct {
		name    string
		args    []string
		request resolve.DiffRequest
		want    error
	}{
		{
			name: "missing branch",
			args: []string{"--base-branch", "", "--target-branch", "main"},
			request: resolve.DiffRequest{
				Base:   snapshotSelector("", ""),
				Target: snapshotSelector("main", ""),
			},
			want: resolve.ErrMissingBranch,
		},
		{
			name: "response budget",
			args: []string{"--base-branch", "main", "--target-branch", "main", "--max-response-bytes", "1"},
			request: resolve.DiffRequest{
				Base: snapshotSelector("main", ""), Target: snapshotSelector("main", ""),
				Budget: func() resolve.QueryBudgetRequest {
					bytes := 1
					return resolve.QueryBudgetRequest{MaxResponseBytes: &bytes}
				}(),
			},
			want: repository.ErrResponseBudgetTooSmall,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var output bytes.Buffer
			command := NewDiffCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
			command.SetOut(&output)
			command.SetArgs(testCase.args)
			cliErr := command.Execute()
			_, mcpErr := tool.SPLDiff(context.Background(), testCase.request)
			if !errors.Is(cliErr, testCase.want) || !errors.Is(mcpErr, testCase.want) {
				t.Fatalf("CLI/MCP errors = %v/%v, want %v", cliErr, mcpErr, testCase.want)
			}

			if output.Len() != 0 {
				t.Fatalf("CLI wrote output for error: %q", output.String())
			}
		})
	}
}

func TestDiffCLIRequiresBothBranchSelectors(t *testing.T) {
	command := NewDiffCommand(func() (*resolve.ResolveTool, error) {
		return resolve.NewResolveTool(newTestSeedRepository(t)), nil
	})
	command.SetArgs([]string{"--base-branch", "main"})

	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), `required flag(s) "target-branch" not set`) {
		t.Fatalf("diff error = %v, want missing target-branch", err)
	}
}

func TestDiffCLIAndMCPRejectUnreachableCommitWithSameCategory(t *testing.T) {
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
	command := NewDiffCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
	command.SetOut(&output)
	command.SetArgs([]string{
		"--base-branch", "main", "--target-branch", "main", "--target-commit", commit,
	})

	cliErr := command.Execute()
	_, mcpErr := tool.SPLDiff(context.Background(), resolve.DiffRequest{
		Base:   snapshotSelector("main", ""),
		Target: resolve.SnapshotSelector{Branch: "main", Commit: &commit},
	})
	if !errors.Is(cliErr, resolve.ErrUnsupportedCommit) || !errors.Is(mcpErr, resolve.ErrUnsupportedCommit) {
		t.Fatalf("CLI/MCP errors = %v/%v, want ErrUnsupportedCommit", cliErr, mcpErr)
	}
	if output.Len() != 0 {
		t.Fatalf("CLI wrote output for error: %q", output.String())
	}
}
