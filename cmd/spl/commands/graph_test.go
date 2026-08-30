package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/autonomous-bits/spool/internal/resolve"
)

func TestGraphCLIAndToolReturnEquivalentJSON(t *testing.T) {
	repo := retrievalCommandRepository(t)
	tool := resolve.NewResolveTool(repo)
	var output bytes.Buffer
	command := NewGraphCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"--branch", "main"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute graph: %v", err)
	}
	var actual resolve.GraphResult
	if err := json.Unmarshal(output.Bytes(), &actual); err != nil {
		t.Fatalf("decode graph: %v", err)
	}
	want, err := tool.SPLGraph(context.Background(), resolve.SnapshotSelector{Branch: "main"})
	if err != nil {
		t.Fatalf("SPLGraph: %v", err)
	}
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("CLI graph result %#v, tool result %#v", actual, want)
	}
}
