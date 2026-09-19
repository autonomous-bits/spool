package commands

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/autonomous-bits/spool/internal/ctxgit"
	"github.com/autonomous-bits/spool/internal/surface"
)

type jsonRPCResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

type callToolResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError,omitempty"`
}

func TestMCPCommand_KeepSurface(t *testing.T) {
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()

	cmd := NewMCPCommand(ctxgit.Options{WorkspaceDir: t.TempDir()})
	cmd.SetIn(stdinReader)
	cmd.SetOut(stdoutWriter)

	errCh := make(chan error, 1)
	go func() {
		errCh <- cmd.Execute()
		_ = stdoutWriter.Close()
	}()

	scanner := bufio.NewScanner(stdoutReader)
	send := func(req string) {
		if _, err := stdinWriter.Write([]byte(req + "\n")); err != nil {
			t.Fatalf("failed to write request: %v", err)
		}
	}
	recv := func() string {
		if !scanner.Scan() {
			t.Fatalf("failed to scan response: %v", scanner.Err())
		}
		return scanner.Text()
	}
	parseCallResult := func(t *testing.T, line string) (any, callToolResult) {
		t.Helper()
		var resp jsonRPCResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("failed to parse response line: %v", err)
		}
		if resp.Error != nil {
			t.Fatalf("unexpected RPC error: %#v", resp.Error)
		}
		resultBytes, err := json.Marshal(resp.Result)
		if err != nil {
			t.Fatalf("failed to marshal result: %v", err)
		}
		var callRes callToolResult
		if err := json.Unmarshal(resultBytes, &callRes); err != nil {
			t.Fatalf("failed to unmarshal callToolResult: %v", err)
		}
		return resp.ID, callRes
	}

	send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}}}`)
	var resp1 jsonRPCResponse
	if err := json.Unmarshal([]byte(recv()), &resp1); err != nil {
		t.Fatalf("resp1 parse: %v", err)
	}
	send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	send(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	var resp2 jsonRPCResponse
	if err := json.Unmarshal([]byte(recv()), &resp2); err != nil {
		t.Fatalf("resp2 parse: %v", err)
	}
	toolsMap, ok := resp2.Result.(map[string]any)
	if !ok || toolsMap["tools"] == nil {
		t.Fatalf("resp2 missing tools: %#v", resp2)
	}
	toolsList := toolsMap["tools"].([]any)
	if len(toolsList) != len(surface.KeepMCPTools) {
		t.Fatalf("expected %d KEEP tools, got %d", len(surface.KeepMCPTools), len(toolsList))
	}
	names := map[string]bool{}
	for _, raw := range toolsList {
		tool, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("tool entry = %#v", raw)
		}
		name, _ := tool["name"].(string)
		names[name] = true
	}
	for _, name := range surface.KeepMCPTools {
		if !names[name] {
			t.Errorf("KEEP tool missing from MCP tools/list: %q", name)
		}
	}
	for _, name := range surface.RemovedMCPTools {
		if names[name] {
			t.Errorf("removed tool still advertised: %q", name)
		}
	}

	send(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"spl_prune","arguments":{"dry_run":true}}}`)
	id3, res3 := parseCallResult(t, recv())
	if id3 != float64(3) || !res3.IsError {
		t.Fatalf("unbound prune should fail: %#v", res3)
	}
	if !strings.Contains(res3.Content[0].Text, "not bound") {
		t.Fatalf("expected unbound error: %s", res3.Content[0].Text)
	}

	send(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"spl_version","arguments":{}}}`)
	id4, res4 := parseCallResult(t, recv())
	if id4 != float64(4) || res4.IsError {
		t.Fatalf("version error: %#v", res4)
	}

	_ = stdinWriter.Close()
	if err := <-errCh; err != nil {
		t.Fatalf("cmd.Execute failed: %v", err)
	}
}
