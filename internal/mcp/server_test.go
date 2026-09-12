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

	// Verify all tool names start with spl_
	for _, tool := range toolsList.Tools {
		if len(tool.Name) < 4 || tool.Name[:4] != "spl_" {
			t.Errorf("tool %q does not start with spl_ prefix", tool.Name)
		}
	}

	// 2. Call spl_version
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "spl_version",
		Arguments: map[string]any{},
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

	// 3. Call nonexistent tool
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "spl_nonexistent",
		Arguments: map[string]any{},
	})
	if err == nil && (res == nil || !res.IsError) {
		t.Fatalf("expected error for nonexistent tool call, got res=%v, err=%v", res, err)
	}
}
