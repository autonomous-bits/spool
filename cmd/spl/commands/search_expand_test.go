package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/autonomous-bits/spool/internal/resolve"
)

func TestSearchExpandCLIAndToolReturnEquivalentJSON(t *testing.T) {
	repo := retrievalCommandRepository(t)
	tool := resolve.NewResolveTool(repo)
	rows, bytesLimit := 10, 100_000
	timeout := time.Second
	budget := resolve.QueryBudgetRequest{
		MaxRows: &rows, MaxResponseBytes: &bytesLimit, Timeout: &timeout,
	}

	var output bytes.Buffer
	command := NewSearchExpandCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"--branch", "main", "--query", "evidence", "--direction", "out", "--edge-type", "RELATED", "--seed-limit", "1", "--max-rows", "10", "--max-response-bytes", "100000", "--timeout", "1s"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute search-expand: %v", err)
	}
	var actual resolve.SearchExpandResult
	if err := json.Unmarshal(output.Bytes(), &actual); err != nil {
		t.Fatalf("decode search-expand: %v", err)
	}
	if actual.Completion.ResponseBytes != output.Len() {
		t.Fatalf("search-expand response bytes = %d, want %d", actual.Completion.ResponseBytes, output.Len())
	}

	want, err := tool.SPLSearchExpand(context.Background(), resolve.SearchExpandRequest{
		Selector:  resolve.SnapshotSelector{Branch: "main"},
		Seeds:     resolve.SeedSelector{Query: "evidence"},
		SeedLimit: 1,
		Direction: resolve.DirectionOut,
		EdgeTypes: []string{"RELATED"},
		Budget:    budget,
	})
	if err != nil {
		t.Fatalf("SPLSearchExpand: %v", err)
	}
	actual.Completion = resolve.QueryCompletionMetadata{}
	want.Completion = resolve.QueryCompletionMetadata{}
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("CLI search-expand result %#v, tool result %#v", actual, want)
	}
}
