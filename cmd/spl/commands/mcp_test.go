package commands

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
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

func TestMCPCommand_FullWorkflow(t *testing.T) {
	stateDir := t.TempDir()
	initRepo, err := repository.InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	if err := initRepo.Close(); err != nil {
		t.Fatalf("close initRepo: %v", err)
	}

	stateDirProvider := func() (string, error) {
		return stateDir, nil
	}

	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()

	cmd := NewMCPCommand(stateDirProvider)
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

	// 1. initialize
	send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}}}`)
	var resp1 jsonRPCResponse
	if err := json.Unmarshal([]byte(recv()), &resp1); err != nil {
		t.Fatalf("resp1 parse: %v", err)
	}
	if resp1.ID != float64(1) {
		t.Fatalf("resp1 id mismatch: %v", resp1.ID)
	}

	// 2. initialized notification
	send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)

	// 3. list tools
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
	if len(toolsList) != 42 {
		t.Fatalf("expected 42 tools, got %d", len(toolsList))
	}

	// 4. status on main (initially empty)
	send(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"spl_status","arguments":{"branch":"main"}}}`)
	id3, res3 := parseCallResult(t, recv())
	if id3 != float64(3) || res3.IsError {
		t.Fatalf("res3 error: %#v", res3)
	}

	// 5. add node via inline JSON batch (no disk file!)
	send(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"spl_add","arguments":{"branch":"main","operations":[{"action":"add","entity":"node","id":"idea-mcp-test","title":"Test MCP node","labels":["Requirement"],"properties":{"priority":{"kind":"integer","integer":1}}}]}}}`)
	id4, res4 := parseCallResult(t, recv())
	if id4 != float64(4) || res4.IsError {
		t.Fatalf("res4 error: %#v", res4)
	}
	if !strings.Contains(res4.Content[0].Text, `"operations":1`) {
		t.Fatalf("res4 expected operations:1: %s", res4.Content[0].Text)
	}

	// 6. status on main after add
	send(`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"spl_status","arguments":{"branch":"main"}}}`)
	id5, res5 := parseCallResult(t, recv())
	if id5 != float64(5) || res5.IsError {
		t.Fatalf("res5 error: %#v", res5)
	}
	if !strings.Contains(res5.Content[0].Text, `"operations":1`) {
		t.Fatalf("res5 expected operations:1 in status: %s", res5.Content[0].Text)
	}

	// 7. commit
	send(`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"spl_commit","arguments":{"branch":"main","author":"agent","message":"Add test node via MCP"}}}`)
	id6, res6 := parseCallResult(t, recv())
	if id6 != float64(6) || res6.IsError {
		t.Fatalf("res6 error: %#v", res6)
	}
	if !strings.Contains(res6.Content[0].Text, `"commit"`) {
		t.Fatalf("res6 expected commit_id: %s", res6.Content[0].Text)
	}

	// 8. resolve committed node
	send(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"spl_resolve","arguments":{"branch":"main","node":"idea-mcp-test"}}}`)
	id7, res7 := parseCallResult(t, recv())
	if id7 != float64(7) || res7.IsError {
		t.Fatalf("res7 error: %#v", res7)
	}
	if !strings.Contains(res7.Content[0].Text, "Test MCP node") {
		t.Fatalf("res7 expected 'Test MCP node': %s", res7.Content[0].Text)
	}

	// 9. branch list
	send(`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"spl_branch_list","arguments":{}}}`)
	id8, res8 := parseCallResult(t, recv())
	if id8 != float64(8) || res8.IsError {
		t.Fatalf("res8 error: %#v", res8)
	}
	if !strings.Contains(res8.Content[0].Text, `"main"`) {
		t.Fatalf("res8 expected main branch: %s", res8.Content[0].Text)
	}

	// 10. create feature branch
	send(`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"spl_branch_create","arguments":{"name":"feature/mcp-idea","from_branch":"main"}}}`)
	id9, res9 := parseCallResult(t, recv())
	if id9 != float64(9) || res9.IsError {
		t.Fatalf("res9 error: %#v", res9)
	}

	// 11. diff main and feature branch
	send(`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"spl_diff","arguments":{"base_branch":"main","target_branch":"feature/mcp-idea"}}}`)
	id10, res10 := parseCallResult(t, recv())
	if id10 != float64(10) || res10.IsError {
		t.Fatalf("res10 error: %#v", res10)
	}

	// 12. branches-containing
	send(`{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"spl_branches_containing","arguments":{"entity_id":"idea-mcp-test"}}}`)
	id11, res11 := parseCallResult(t, recv())
	if id11 != float64(11) || res11.IsError {
		t.Fatalf("res11 error: %#v", res11)
	}
	if !strings.Contains(res11.Content[0].Text, `"main"`) {
		t.Fatalf("res11 expected main in branches_containing: %s", res11.Content[0].Text)
	}

	// 13. search-expand
	send(`{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"spl_search_expand","arguments":{"branch":"main","query":"Test"}}}`)
	id12, res12 := parseCallResult(t, recv())
	if id12 != float64(12) || res12.IsError {
		t.Fatalf("res12 error: %#v", res12)
	}
	if !strings.Contains(res12.Content[0].Text, "idea-mcp-test") {
		t.Fatalf("res12 expected idea-mcp-test in search_expand: %s", res12.Content[0].Text)
	}

	// 14. gc dry-run
	send(`{"jsonrpc":"2.0","id":13,"method":"tools/call","params":{"name":"spl_gc","arguments":{"dry_run":true}}}`)
	id13, res13 := parseCallResult(t, recv())
	if id13 != float64(13) || res13.IsError {
		t.Fatalf("res13 error: %#v", res13)
	}

	// 15. version
	send(`{"jsonrpc":"2.0","id":14,"method":"tools/call","params":{"name":"spl_version","arguments":{}}}`)
	id14, res14 := parseCallResult(t, recv())
	if id14 != float64(14) || res14.IsError {
		t.Fatalf("res14 error: %#v", res14)
	}

	// 16. remote set
	send(`{"jsonrpc":"2.0","id":15,"method":"tools/call","params":{"name":"spl_remote_set","arguments":{"endpoint":"http://127.0.0.1:8080","auth_mode":"bearer","workspace_id":"ws-test"}}}`)
	id15, res15 := parseCallResult(t, recv())
	if id15 != float64(15) || res15.IsError {
		t.Fatalf("res15 error: %#v", res15)
	}

	// 17. remote show
	send(`{"jsonrpc":"2.0","id":16,"method":"tools/call","params":{"name":"spl_remote_show","arguments":{}}}`)
	id16, res16 := parseCallResult(t, recv())
	if id16 != float64(16) || res16.IsError {
		t.Fatalf("res16 error: %#v", res16)
	}
	if !strings.Contains(res16.Content[0].Text, "127.0.0.1:8080") {
		t.Fatalf("res16 expected 127.0.0.1:8080: %s", res16.Content[0].Text)
	}

	// 18. remote remove
	send(`{"jsonrpc":"2.0","id":17,"method":"tools/call","params":{"name":"spl_remote_remove","arguments":{}}}`)
	id17, res17 := parseCallResult(t, recv())
	if id17 != float64(17) || res17.IsError {
		t.Fatalf("res17 error: %#v", res17)
	}

	// Close stdin and verify server cleanly shuts down
	_ = stdinWriter.Close()
	if err := <-errCh; err != nil {
		t.Fatalf("cmd.Execute failed: %v", err)
	}
}
