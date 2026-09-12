package mcp

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSpoolMCPServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	server := NewSpoolServer(func() (string, error) {
		return t.TempDir(), nil
	})

	go func() {
		_ = server.Run(ctx, serverTransport)
	}()

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect failed: %v", err)
	}
	defer session.Close()

	// 1. Verify tools list
	toolsList, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("session.ListTools failed: %v", err)
	}
	if len(toolsList.Tools) != 42 {
		t.Fatalf("expected 42 tools, got %d", len(toolsList.Tools))
	}

	expectedTools := map[string]bool{
		"spl_add":                 true,
		"spl_asset_add":           true,
		"spl_asset_read":          true,
		"spl_branch_create":       true,
		"spl_branch_delete":       true,
		"spl_branch_list":         true,
		"spl_branches_containing": true,
		"spl_cherry_pick":         true,
		"spl_clone":               true,
		"spl_commit":              true,
		"spl_context":             true,
		"spl_diff":                true,
		"spl_filter":              true,
		"spl_fsck":                true,
		"spl_gc":                  true,
		"spl_graph":               true,
		"spl_history":             true,
		"spl_init":                true,
		"spl_merge_abort":         true,
		"spl_merge_apply":         true,
		"spl_merge_conflicts":     true,
		"spl_merge_finalize":      true,
		"spl_merge_preview":       true,
		"spl_merge_resolve":       true,
		"spl_migrate":             true,
		"spl_prune":               true,
		"spl_pull":                true,
		"spl_push":                true,
		"spl_remote_branch":       true,
		"spl_remote_remove":       true,
		"spl_remote_set":          true,
		"spl_remote_show":         true,
		"spl_resolve":             true,
		"spl_schema_migrate":      true,
		"spl_search":              true,
		"spl_search_expand":       true,
		"spl_status":              true,
		"spl_switch":              true,
		"spl_validate":            true,
		"spl_version":             true,
		"spl_workspace_attach":    true,
		"spl_workspace_init":      true,
	}

	for _, tool := range toolsList.Tools {
		if !expectedTools[tool.Name] {
			t.Errorf("unexpected tool registered: %q", tool.Name)
		}
		delete(expectedTools, tool.Name)
	}
	if len(expectedTools) > 0 {
		t.Fatalf("missing expected tools: %v", expectedTools)
	}

	// 2. Call spl_version with nil/empty arguments (verifies argument normalization)
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "spl_version",
	})
	if err != nil {
		t.Fatalf("CallTool spl_version failed: %v", err)
	}
	if res.IsError {
		t.Fatalf("spl_version returned tool error: %+v", res)
	}
	if len(res.Content) == 0 {
		t.Fatalf("spl_version returned no content")
	}

	// 3. Call spl_context with invalid direction (verifies direction enum validation)
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "spl_context",
		Arguments: map[string]any{
			"branch":    "main",
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

	// 4. Call nonexistent tool
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "spl_nonexistent",
		Arguments: map[string]any{},
	})
	if err == nil && (res == nil || !res.IsError) {
		t.Fatalf("expected error for nonexistent tool call, got res=%v, err=%v", res, err)
	}
}
