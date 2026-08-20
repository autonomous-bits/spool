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
	"github.com/autonomous-bits/spool/internal/repository/branch"
	"github.com/autonomous-bits/spool/internal/resolve"
)

func TestResolveCLIAndMCPReturnEquivalentPayloads(t *testing.T) {
	repo := repository.NewSeedRepository()
	tool := resolve.NewResolveTool(repo)

	var output bytes.Buffer
	if err := runResolveCommand([]string{"--branch", "main", "--node", repository.SeedNodeID}, &output, tool); err != nil {
		t.Fatalf("run CLI: %v", err)
	}

	var cliResult resolve.ResolveResult
	if err := json.Unmarshal(output.Bytes(), &cliResult); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}
	if cliResult.Completion.ResponseBytes != output.Len() {
		t.Fatalf("CLI resolve bytes = %d, want responseBytes %d", output.Len(), cliResult.Completion.ResponseBytes)
	}
	mcpResult, err := tool.EDGResolve(context.Background(), resolve.ResolveRequest{
		Selector: resolve.SnapshotSelector{Branch: "main"},
		NodeID:   repository.SeedNodeID,
	})
	if err != nil {
		t.Fatalf("EDGResolve: %v", err)
	}
	if !reflect.DeepEqual(comparableResolveResult(cliResult), comparableResolveResult(mcpResult)) {
		t.Fatalf("CLI result %#v does not match MCP result %#v", cliResult, mcpResult)
	}
}

func TestResolveCLIRequiresBranch(t *testing.T) {
	repo := repository.NewSeedRepository()
	tool := resolve.NewResolveTool(repo)

	var output bytes.Buffer
	cliErr := runResolveCommand([]string{"--node", repository.SeedNodeID}, &output, tool)
	if cliErr == nil || !strings.Contains(cliErr.Error(), `required flag(s) "branch" not set`) {
		t.Fatalf("CLI error = %v, want required branch error", cliErr)
	}
	if output.Len() != 0 {
		t.Fatalf("CLI wrote success output for missing branch: %q", output.String())
	}
}

func TestResolveCLIAndMCPUseExplicitOlderCommit(t *testing.T) {
	repo := repository.NewSeedRepository()
	olderCommit, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}

	if _, err := repo.AdvanceBranch("main"); err != nil {
		t.Fatalf("AdvanceBranch: %v", err)
	}
	tool := resolve.NewResolveTool(repo)

	var output bytes.Buffer
	if err := runResolveCommand([]string{"--branch", "main", "--commit", string(olderCommit), "--node", repository.SeedNodeID}, &output, tool); err != nil {
		t.Fatalf("run CLI: %v", err)
	}
	var cliResult resolve.ResolveResult
	if err := json.Unmarshal(output.Bytes(), &cliResult); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}
	selected := string(olderCommit)
	mcpResult, err := tool.EDGResolve(context.Background(), resolve.ResolveRequest{
		Selector: resolve.SnapshotSelector{Branch: "main", Commit: &selected},
		NodeID:   repository.SeedNodeID,
	})
	if err != nil {
		t.Fatalf("EDGResolve: %v", err)
	}
	if !reflect.DeepEqual(comparableResolveResult(cliResult), comparableResolveResult(mcpResult)) || cliResult.Snapshot.Commit != selected {
		t.Fatalf("CLI/MCP results = %#v/%#v, want selected commit %q", cliResult, mcpResult, selected)
	}
}

func TestResolveCLIHONorsLowerBudgetAndCapsOverLimitRequests(t *testing.T) {
	configured := resolve.QueryBudget{
		MaxRows:          100,
		MaxResponseBytes: 1_000,
		MaxDepth:         10,
		MaxVisited:       500,
		Timeout:          time.Second,
	}
	tool := resolve.NewResolveToolWithOptions(repository.NewSeedRepository(), resolve.Options{QueryBudget: &configured})

	var output bytes.Buffer
	if err := runResolveCommand([]string{
		"--branch", "main",
		"--node", repository.SeedNodeID,
		"--max-rows", "25",
		"--max-response-bytes", "800",
		"--max-depth", "3",
		"--max-visited", "100",
		"--timeout", "100ms",
	}, &output, tool); err != nil {
		t.Fatalf("run CLI: %v", err)
	}
	var cliResult resolve.ResolveResult
	if err := json.Unmarshal(output.Bytes(), &cliResult); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}
	if got, want := cliResult.Budget, (resolve.QueryBudget{
		MaxRows: 25, MaxResponseBytes: 800, MaxDepth: 3,
		MaxVisited: 100, Timeout: 100 * time.Millisecond,
	}); got != want {
		t.Fatalf("CLI budget = %#v, want %#v", got, want)
	}

	rows, responseBytes, depth, visited := 25, 800, 3, 100
	timeout := 100 * time.Millisecond
	mcpResult, err := tool.EDGResolve(context.Background(), resolve.ResolveRequest{
		Selector: resolve.SnapshotSelector{Branch: "main"},
		NodeID:   repository.SeedNodeID,
		Budget: resolve.QueryBudgetRequest{
			MaxRows: &rows, MaxResponseBytes: &responseBytes, MaxDepth: &depth,
			MaxVisited: &visited, Timeout: &timeout,
		},
	})
	if err != nil {
		t.Fatalf("EDGResolve: %v", err)
	}
	if !reflect.DeepEqual(comparableResolveResult(cliResult), comparableResolveResult(mcpResult)) {
		t.Fatalf("CLI result %#v does not match MCP result %#v", cliResult, mcpResult)
	}

	output.Reset()
	err = runResolveCommand([]string{
		"--branch", "main",
		"--node", repository.SeedNodeID,
		"--max-rows", "200",
		"--max-response-bytes", "2000",
		"--max-depth", "20",
		"--max-visited", "1000",
		"--timeout", "2s",
	}, &output, tool)
	if err == nil {
		if err := json.Unmarshal(output.Bytes(), &cliResult); err != nil {
			t.Fatalf("decode CLI result: %v", err)
		}
		if cliResult.Budget.MaxRows > configured.MaxRows ||
			cliResult.Budget.MaxResponseBytes > configured.MaxResponseBytes ||
			cliResult.Budget.MaxDepth > configured.MaxDepth ||
			cliResult.Budget.MaxVisited > configured.MaxVisited ||
			cliResult.Budget.Timeout > configured.Timeout {
			t.Fatalf("CLI effective budget exceeds configuration: %#v", cliResult.Budget)
		}
	}
}

func TestResolveCLIRejectsDisallowedOrEmptyExplicitCommitWithoutOutput(t *testing.T) {
	repo := repository.NewSeedRepository()
	if _, err := repo.CreateBranch("feature", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	featureCommit, err := repo.AdvanceBranch("feature")
	if err != nil {
		t.Fatalf("AdvanceBranch feature: %v", err)
	}
	for _, testCase := range []struct {
		name string
		args []string
		want error
	}{
		{
			name: "unreachable",
			args: []string{"--branch", "main", "--commit", string(featureCommit), "--node", repository.SeedNodeID},
			want: resolve.ErrUnsupportedCommit,
		},
		{
			name: "empty",
			args: []string{"--branch", "main", "--commit", "", "--node", repository.SeedNodeID},
			want: repository.ErrCommitNotFound,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var output bytes.Buffer
			err := runResolveCommand(testCase.args, &output, resolve.NewResolveTool(repo))
			if !errors.Is(err, testCase.want) {
				t.Fatalf("CLI error = %v, want %v", err, testCase.want)
			}
			if output.Len() != 0 {
				t.Fatalf("CLI wrote success output: %q", output.String())
			}
		})
	}
}

func runResolveCommand(args []string, output *bytes.Buffer, tool *resolve.ResolveTool) error {
	command := NewResolveCommand(tool)
	command.SetOut(output)
	command.SetArgs(args)
	return command.Execute()
}

func comparableResolveResult(result resolve.ResolveResult) resolve.ResolveResult {
	result.Completion = resolve.QueryCompletionMetadata{}
	return result
}
