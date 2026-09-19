package mcp

import (
	"context"
	"strings"
	"testing"

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

	removed := []string{
		"spl_add", "spl_status", "spl_commit", "spl_init", "spl_context",
		"spl_diff", "spl_history", "spl_branches_containing", "spl_fsck", "spl_gc",
		"spl_cherry_pick", "spl_clone", "spl_push", "spl_pull", "spl_migrate",
		"spl_workspace_init", "spl_workspace_attach", "spl_remote_set",
		"spl_branch_list", "spl_branch_create", "spl_switch",
	}
	for _, name := range removed {
		for _, tool := range toolsList.Tools {
			if tool.Name == name {
				t.Errorf("removed tool still registered: %q", name)
			}
		}
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
		Name:      "spl_nonexistent",
		Arguments: map[string]any{},
	})
	if err == nil && (res == nil || !res.IsError) {
		t.Fatalf("expected error for nonexistent tool call, got res=%v, err=%v", res, err)
	}
	_ = strings.TrimSpace
}
