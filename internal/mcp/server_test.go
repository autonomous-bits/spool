package mcp

import (
	"context"
	"testing"

	"github.com/autonomous-bits/spool/internal/surface"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSpoolMCPServerKeepToolsOnly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	server := NewSpoolServerWithOptions(ServerOptions{
		WorkspaceDir: func() (string, error) { return t.TempDir(), nil },
	})
	go func() {
		_ = server.Run(ctx, serverTransport)
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect failed: %v", err)
	}
	defer func() { _ = session.Close() }()

	initResult := session.InitializeResult()
	if initResult == nil || initResult.ServerInfo == nil || initResult.ServerInfo.Name != "spool" {
		t.Fatalf("expected InitializeResult with server name spool")
	}

	toolsList, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("session.ListTools failed: %v", err)
	}
	if len(toolsList.Tools) != len(KeepToolNames) {
		t.Fatalf("expected %d tools, got %d", len(KeepToolNames), len(toolsList.Tools))
	}
	expected := map[string]bool{}
	for _, name := range KeepToolNames {
		expected[name] = true
	}
	for _, tool := range toolsList.Tools {
		if !expected[tool.Name] {
			t.Errorf("unexpected tool registered: %q", tool.Name)
		}
		delete(expected, tool.Name)
	}
	if len(expected) > 0 {
		t.Fatalf("missing expected tools: %v", expected)
	}

	removed := surface.RemovedMCPTools
	for _, name := range removed {
		for _, tool := range toolsList.Tools {
			if tool.Name == name {
				t.Errorf("removed tool still registered: %q", name)
			}
		}
	}
	if !containsTool(toolsList.Tools, "spl_query_context") {
		t.Fatal("KEEP tools must include spl_query_context")
	}
	if containsTool(toolsList.Tools, "spl_context") {
		t.Fatal("old spl_context query tool must not be registered")
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "spl_version"})
	if err != nil || res.IsError {
		t.Fatalf("spl_version failed: err=%v res=%+v", err, res)
	}

	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "spl_query_context",
		Arguments: map[string]any{
			"query":     "test",
			"direction": "sideways",
		},
	})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected error for invalid direction, got: %+v", res)
	}

	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "spl_context",
		Arguments: map[string]any{"query": "test"},
	})
	if err == nil && (res == nil || !res.IsError) {
		t.Fatalf("expected error for removed spl_context tool, got res=%v, err=%v", res, err)
	}

	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "spl_nonexistent",
		Arguments: map[string]any{},
	})
	if err == nil && (res == nil || !res.IsError) {
		t.Fatalf("expected error for nonexistent tool call, got res=%v, err=%v", res, err)
	}
}

func containsTool(tools []*mcp.Tool, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}
