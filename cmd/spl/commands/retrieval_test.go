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

	"github.com/autonomous-bits/spool/internal/contextual"
	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/spf13/cobra"
)

func TestRetrievalCLIAndToolReturnEquivalentJSON(t *testing.T) {
	repo := retrievalCommandRepository(t)
	tool := resolve.NewResolveTool(repo)
	rows, bytesLimit := 10, 100_000
	timeout := time.Second
	budget := resolve.QueryBudgetRequest{
		MaxRows: &rows, MaxResponseBytes: &bytesLimit, Timeout: &timeout,
	}
	cases := []struct {
		name    string
		command cobraCommand
		want    any
		decode  func([]byte) (any, error)
	}{
		{
			name: "filter",
			command: cobraCommand{new: func() *cobra.Command {
				return NewFilterCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
			}, args: []string{"--branch", "main", "--label", "Seed", "--max-rows", "10", "--max-response-bytes", "100000", "--timeout", "1s"}},
			want: func() any {
				result, err := tool.EDGFilter(context.Background(), resolve.FilterRequest{
					Selector: resolve.SnapshotSelector{Branch: "main"}, Labels: []string{"Seed"}, Budget: budget,
				})
				if err != nil {
					t.Fatalf("EDGFilter: %v", err)
				}
				return result
			}(),
			decode: func(data []byte) (any, error) {
				var result resolve.FilterResult
				return result, json.Unmarshal(data, &result)
			},
		},
		{
			name: "search",
			command: cobraCommand{new: func() *cobra.Command {
				return NewSearchCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
			}, args: []string{"--branch", "main", "--query", "evidence", "--max-rows", "10", "--max-response-bytes", "100000", "--timeout", "1s"}},
			want: func() any {
				result, err := tool.EDGSearch(context.Background(), resolve.SearchRequest{
					Selector: resolve.SnapshotSelector{Branch: "main"}, Query: "evidence", Budget: budget,
				})
				if err != nil {
					t.Fatalf("EDGSearch: %v", err)
				}
				return result
			}(),
			decode: func(data []byte) (any, error) {
				var result resolve.SearchResult
				return result, json.Unmarshal(data, &result)
			},
		},
		{
			name: "search-expand",
			command: cobraCommand{new: func() *cobra.Command {
				return NewSearchExpandCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
			}, args: []string{"--branch", "main", "--query", "evidence", "--direction", "out", "--edge-type", "RELATED", "--seed-limit", "1", "--max-rows", "10", "--max-response-bytes", "100000", "--timeout", "1s"}},
			want: func() any {
				result, err := tool.EDGSearchExpand(context.Background(), resolve.SearchExpandRequest{
					Selector: resolve.SnapshotSelector{Branch: "main"}, Seeds: contextual.SeedSelector{Query: "evidence"},
					SeedLimit: 1, Direction: contextual.DirectionOut, EdgeTypes: []string{"RELATED"}, Budget: budget,
				})
				if err != nil {
					t.Fatalf("EDGSearchExpand: %v", err)
				}
				return result
			}(),
			decode: func(data []byte) (any, error) {
				var result resolve.SearchExpandResult
				return result, json.Unmarshal(data, &result)
			},
		},
		{
			name: "context",
			command: cobraCommand{new: func() *cobra.Command {
				return NewContextCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
			}, args: []string{"--branch", "main", "--label", "Seed", "--direction", "both", "--edge-type", "RELATED", "--max-rows", "10", "--max-response-bytes", "100000", "--timeout", "1s"}},
			want: func() any {
				result, err := tool.EDGContext(context.Background(), resolve.ContextRequest{
					Selector: resolve.SnapshotSelector{Branch: "main"}, Seeds: contextual.SeedSelector{Labels: []string{"Seed"}},
					Direction: contextual.DirectionBoth, EdgeTypes: []string{"RELATED"}, Budget: budget,
				})
				if err != nil {
					t.Fatalf("EDGContext: %v", err)
				}
				return result
			}(),
			decode: func(data []byte) (any, error) {
				var result resolve.ContextResult
				return result, json.Unmarshal(data, &result)
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var output bytes.Buffer
			command := testCase.command.new()
			command.SetOut(&output)
			command.SetArgs(testCase.command.args)
			if err := command.Execute(); err != nil {
				t.Fatalf("execute %s: %v", testCase.name, err)
			}
			actual, err := testCase.decode(output.Bytes())
			if err != nil {
				t.Fatalf("decode %s: %v", testCase.name, err)
			}
			if completionResponseBytes(actual) != output.Len() {
				t.Fatalf("%s response bytes = %d, want %d", testCase.name, completionResponseBytes(actual), output.Len())
			}
			if !reflect.DeepEqual(withoutCompletion(actual), withoutCompletion(testCase.want)) {
				t.Fatalf("CLI %s result %#v, tool result %#v", testCase.name, actual, testCase.want)
			}
		})
	}
}

func TestRetrievalCLIEnforcesBranchAndProjectionSelectorConstraints(t *testing.T) {
	repo := retrievalCommandRepository(t)
	tool := resolve.NewResolveTool(repo)
	historical, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("pin branch: %v", err)
	}
	if _, err := repo.AdvanceBranch("main"); err != nil {
		t.Fatalf("advance branch: %v", err)
	}

	search := NewSearchCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
	search.SetArgs([]string{"--query", "evidence"})
	if err := search.Execute(); err == nil || !strings.Contains(err.Error(), `required flag(s) "branch" not set`) {
		t.Fatalf("missing branch error = %v", err)
	}

	var output bytes.Buffer
	filter := NewFilterCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
	filter.SetOut(&output)
	filter.SetArgs([]string{"--branch", "main", "--commit", string(historical), "--label", "Seed"})
	cliErr := filter.Execute()
	commit := string(historical)
	_, toolErr := tool.EDGFilter(context.Background(), resolve.FilterRequest{
		Selector: resolve.SnapshotSelector{Branch: "main", Commit: &commit}, Labels: []string{"Seed"},
	})
	if !errors.Is(cliErr, repository.ErrHistoricalProjectionUnsupported) || !errors.Is(toolErr, repository.ErrHistoricalProjectionUnsupported) {
		t.Fatalf("CLI/tool errors = %v/%v, want historical projection constraint", cliErr, toolErr)
	}
	if output.Len() != 0 {
		t.Fatalf("CLI emitted output on projection error: %q", output.String())
	}

	search = NewSearchCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
	search.SetArgs([]string{"--branch", "main", "--query", "evidence", "--sql", "select 1"})
	if err := search.Execute(); err == nil || !strings.Contains(err.Error(), "unknown flag: --sql") {
		t.Fatalf("SQL flag error = %v, want unknown flag", err)
	}
}

func TestContextualCLIRequiresExactlyOneSeedSelector(t *testing.T) {
	tool := resolve.NewResolveTool(retrievalCommandRepository(t))
	for _, testCase := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing selector",
			args: []string{"--branch", "main"},
			want: "provide --query or at least one typed filter",
		},
		{
			name: "mixed selectors",
			args: []string{"--branch", "main", "--query", "evidence", "--label", "Seed"},
			want: "--query cannot be combined with typed filter flags",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			command := NewContextCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
			command.SetArgs(testCase.args)
			if err := command.Execute(); err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("context error = %v, want %q", err, testCase.want)
			}
		})
	}
}

type cobraCommand struct {
	new  func() *cobra.Command
	args []string
}

func completionResponseBytes(value any) int {
	switch result := value.(type) {
	case resolve.FilterResult:
		return result.Completion.ResponseBytes
	case resolve.SearchResult:
		return result.Completion.ResponseBytes
	case resolve.SearchExpandResult:
		return result.Completion.ResponseBytes
	default:
		return 0
	}
}

func withoutCompletion(value any) any {
	switch result := value.(type) {
	case resolve.FilterResult:
		result.Completion = resolve.QueryCompletionMetadata{}
		return result
	case resolve.SearchResult:
		result.Completion = resolve.QueryCompletionMetadata{}
		return result
	case resolve.SearchExpandResult:
		result.Completion = resolve.QueryCompletionMetadata{}
		return result
	default:
		return value
	}
}

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
