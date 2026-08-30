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

func TestContextCLIAndToolReturnEquivalentJSON(t *testing.T) {
	repo := retrievalCommandRepository(t)
	tool := resolve.NewResolveTool(repo)
	rows, bytesLimit := 10, 100_000
	timeout := time.Second
	budget := resolve.QueryBudgetRequest{
		MaxRows: &rows, MaxResponseBytes: &bytesLimit, Timeout: &timeout,
	}

	var output bytes.Buffer
	command := NewContextCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"--branch", "main", "--label", "Seed", "--direction", "both", "--edge-type", "RELATED", "--max-rows", "10", "--max-response-bytes", "100000", "--timeout", "1s"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute context: %v", err)
	}
	var actual resolve.ContextResult
	if err := json.Unmarshal(output.Bytes(), &actual); err != nil {
		t.Fatalf("decode context: %v", err)
	}
	if actual.Completion.ResponseBytes != output.Len() {
		t.Fatalf("context response bytes = %d, want %d", actual.Completion.ResponseBytes, output.Len())
	}

	want, err := tool.SPLContext(context.Background(), resolve.ContextRequest{
		Selector:  resolve.SnapshotSelector{Branch: "main"},
		Seeds:     resolve.SeedSelector{Labels: []string{"Seed"}},
		Direction: resolve.DirectionBoth,
		EdgeTypes: []string{"RELATED"},
		Budget:    budget,
	})
	if err != nil {
		t.Fatalf("SPLContext: %v", err)
	}
	actual.Completion = resolve.QueryCompletionMetadata{}
	want.Completion = resolve.QueryCompletionMetadata{}
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("CLI context result %#v, tool result %#v", actual, want)
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
