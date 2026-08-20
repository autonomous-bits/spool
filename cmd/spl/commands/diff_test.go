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

func TestDiffCLIAndMCPReturnEquivalentPayloads(t *testing.T) {
	repo := repository.NewSeedRepository()
	base, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("pin base: %v", err)
	}

	if _, err := repo.StageMutationBatch(repository.StageMutationRequest{
		Branch:     "main",
		Operations: []repository.MutationOperation{{Action: "add", Entity: "node", ID: "node-2", Title: "Second node"}},
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
		"--node-id", "node-2", "--max-rows", "10",
	})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute CLI: %v", err)
	}
	var cliResult resolve.DiffResult
	if err := json.Unmarshal(output.Bytes(), &cliResult); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}
	rows := 10
	mcpResult, err := tool.EDGDiff(context.Background(), resolve.DiffRequest{
		Base: snapshotSelector("main", string(base)), Target: snapshotSelector("main", string(target.Commit)),
		Filter: repository.DiffFilter{NodeIDs: []string{"node-2"}},
		Budget: resolve.QueryBudgetRequest{MaxRows: &rows},
	})
	if err != nil {
		t.Fatalf("EDGDiff: %v", err)
	}
	if !reflect.DeepEqual(cliResult, mcpResult) {
		t.Fatalf("CLI result %#v does not match MCP result %#v", cliResult, mcpResult)
	}
}

func TestDiffCLIAndMCPReturnEquivalentErrors(t *testing.T) {
	tool := resolve.NewResolveTool(repository.NewSeedRepository())
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
			_, mcpErr := tool.EDGDiff(context.Background(), testCase.request)
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
		return resolve.NewResolveTool(repository.NewSeedRepository()), nil
	})
	command.SetArgs([]string{"--base-branch", "main"})

	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), `required flag(s) "target-branch" not set`) {
		t.Fatalf("diff error = %v, want missing target-branch", err)
	}
}

func TestDiffCLIAndMCPRejectUnreachableCommitWithSameCategory(t *testing.T) {
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
	command := NewDiffCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
	command.SetOut(&output)
	command.SetArgs([]string{
		"--base-branch", "main", "--target-branch", "main", "--target-commit", commit,
	})

	cliErr := command.Execute()
	_, mcpErr := tool.EDGDiff(context.Background(), resolve.DiffRequest{
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
