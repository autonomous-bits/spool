package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/autonomous-bits/spool/internal/resolve"
)

func TestSearchCLIAndToolReturnEquivalentJSON(t *testing.T) {
	repo := retrievalCommandRepository(t)
	tool := resolve.NewResolveTool(repo)
	rows, bytesLimit := 10, 100_000
	timeout := time.Second
	budget := resolve.QueryBudgetRequest{
		MaxRows: &rows, MaxResponseBytes: &bytesLimit, Timeout: &timeout,
	}

	var output bytes.Buffer
	command := NewSearchCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"--branch", "main", "--query", "evidence", "--max-rows", "10", "--max-response-bytes", "100000", "--timeout", "1s"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute search: %v", err)
	}
	var actual resolve.SearchResult
	if err := json.Unmarshal(output.Bytes(), &actual); err != nil {
		t.Fatalf("decode search: %v", err)
	}
	if actual.Completion.ResponseBytes != output.Len() {
		t.Fatalf("search response bytes = %d, want %d", actual.Completion.ResponseBytes, output.Len())
	}

	want, err := tool.SPLSearch(context.Background(), resolve.SearchRequest{
		Selector: resolve.SnapshotSelector{Branch: "main"}, Query: "evidence", Budget: budget,
	})
	if err != nil {
		t.Fatalf("SPLSearch: %v", err)
	}
	actual.Completion = resolve.QueryCompletionMetadata{}
	want.Completion = resolve.QueryCompletionMetadata{}
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("CLI search result %#v, tool result %#v", actual, want)
	}
}

func TestSearchCLIRequiresBranchAndRejectsUnknownSQL(t *testing.T) {
	repo := retrievalCommandRepository(t)
	tool := resolve.NewResolveTool(repo)

	search := NewSearchCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
	search.SetArgs([]string{"--query", "evidence"})
	if err := search.Execute(); err == nil || !strings.Contains(err.Error(), `required flag(s) "branch" not set`) {
		t.Fatalf("missing branch error = %v", err)
	}

	search = NewSearchCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
	search.SetArgs([]string{"--branch", "main", "--query", "evidence", "--sql", "select 1"})
	if err := search.Execute(); err == nil || !strings.Contains(err.Error(), "unknown flag: --sql") {
		t.Fatalf("SQL flag error = %v, want unknown flag", err)
	}
}
