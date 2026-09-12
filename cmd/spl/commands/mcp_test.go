package commands

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/autonomous-bits/spool/internal/mcp"
	"github.com/autonomous-bits/spool/internal/repository"
)

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

	requests := []string{
		// 1. initialize
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		// 2. initialized notification
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		// 3. list tools
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		// 4. status on main
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"spl_status","arguments":{"branch":"main"}}}`,
		// 5. add node via inline JSON batch (no disk file!)
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"spl_add","arguments":{"branch":"main","operations":[{"action":"add","entity":"node","id":"idea-mcp-test","title":"Test MCP node","labels":["Requirement"],"properties":{"priority":{"kind":"integer","integer":1}}}]}}}`,
		// 6. status on main after add
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"spl_status","arguments":{"branch":"main"}}}`,
		// 7. commit
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"spl_commit","arguments":{"branch":"main","author":"agent","message":"Add test node via MCP"}}}`,
		// 8. resolve committed node
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"spl_resolve","arguments":{"branch":"main","node":"idea-mcp-test"}}}`,
		// 9. branch list
		`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"spl_branch_list","arguments":{}}}`,
		// 10. create feature branch
		`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"spl_branch_create","arguments":{"name":"feature/mcp-idea","from_branch":"main"}}}`,
		// 11. diff main and feature branch
		`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"spl_diff","arguments":{"base_branch":"main","target_branch":"feature/mcp-idea"}}}`,
		// 12. branches-containing
		`{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"spl_branches_containing","arguments":{"entity_id":"idea-mcp-test"}}}`,
		// 13. search-expand
		`{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"spl_search_expand","arguments":{"branch":"main","query":"Test"}}}`,
		// 14. gc dry-run
		`{"jsonrpc":"2.0","id":13,"method":"tools/call","params":{"name":"spl_gc","arguments":{"dry_run":true}}}`,
		// 15. version
		`{"jsonrpc":"2.0","id":14,"method":"tools/call","params":{"name":"spl_version","arguments":{}}}`,
		// 16. remote set
		`{"jsonrpc":"2.0","id":15,"method":"tools/call","params":{"name":"spl_remote_set","arguments":{"endpoint":"http://127.0.0.1:8080","auth_mode":"bearer","workspace_id":"ws-test"}}}`,
		// 17. remote show
		`{"jsonrpc":"2.0","id":16,"method":"tools/call","params":{"name":"spl_remote_show","arguments":{}}}`,
		// 18. remote remove
		`{"jsonrpc":"2.0","id":17,"method":"tools/call","params":{"name":"spl_remote_remove","arguments":{}}}`,
	}

	input := strings.Join(requests, "\n") + "\n"
	var stdout bytes.Buffer

	cmd := NewMCPCommand(stateDirProvider)
	cmd.SetIn(strings.NewReader(input))
	cmd.SetOut(&stdout)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 17 { // 17 request IDs expected (notification does not generate a response)
		t.Fatalf("expected 17 responses, got %d:\n%s", len(lines), stdout.String())
	}

	// Helper to parse CallToolResult from JSONRPCResponse
	parseCallResult := func(t *testing.T, line string) (any, mcp.CallToolResult) {
		t.Helper()
		var resp mcp.JSONRPCResponse
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
		var callRes mcp.CallToolResult
		if err := json.Unmarshal(resultBytes, &callRes); err != nil {
			t.Fatalf("failed to unmarshal CallToolResult: %v", err)
		}
		return resp.ID, callRes
	}

	// 1. initialize
	var resp1 mcp.JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[0]), &resp1); err != nil {
		t.Fatalf("resp1 parse: %v", err)
	}
	if resp1.ID != float64(1) {
		t.Fatalf("resp1 id mismatch: %v", resp1.ID)
	}

	// 2. tools/list
	var resp2 mcp.JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[1]), &resp2); err != nil {
		t.Fatalf("resp2 parse: %v", err)
	}
	toolsMap, ok := resp2.Result.(map[string]any)
	if !ok || toolsMap["tools"] == nil {
		t.Fatalf("resp2 missing tools: %#v", resp2)
	}
	toolsList := toolsMap["tools"].([]any)
	if len(toolsList) < 30 {
		t.Fatalf("expected at least 30 tools, got %d", len(toolsList))
	}

	// 3. spl_status (initially empty)
	id3, res3 := parseCallResult(t, lines[2])
	if id3 != float64(3) || res3.IsError {
		t.Fatalf("res3 error: %#v", res3)
	}

	// 4. spl_add
	id4, res4 := parseCallResult(t, lines[3])
	if id4 != float64(4) || res4.IsError {
		t.Fatalf("res4 error: %#v", res4)
	}
	if !strings.Contains(res4.Content[0].Text, `"operations":1`) {
		t.Fatalf("res4 expected operations:1: %s", res4.Content[0].Text)
	}

	// 5. spl_status (after add)
	id5, res5 := parseCallResult(t, lines[4])
	if id5 != float64(5) || res5.IsError {
		t.Fatalf("res5 error: %#v", res5)
	}
	if !strings.Contains(res5.Content[0].Text, `"operations":1`) {
		t.Fatalf("res5 expected operations:1 in status: %s", res5.Content[0].Text)
	}

	// 6. spl_commit
	id6, res6 := parseCallResult(t, lines[5])
	if id6 != float64(6) || res6.IsError {
		t.Fatalf("res6 error: %#v", res6)
	}
	if !strings.Contains(res6.Content[0].Text, "commit") {
		t.Fatalf("res6 expected commit output: %s", res6.Content[0].Text)
	}

	// 7. spl_resolve
	id7, res7 := parseCallResult(t, lines[6])
	if id7 != float64(7) || res7.IsError {
		t.Fatalf("res7 error: %#v", res7)
	}
	if !strings.Contains(res7.Content[0].Text, "Test MCP node") {
		t.Fatalf("res7 expected node title in resolve: %s", res7.Content[0].Text)
	}

	// 8. spl_branch_list
	id8, res8 := parseCallResult(t, lines[7])
	if id8 != float64(8) || res8.IsError {
		t.Fatalf("res8 error: %#v", res8)
	}
	if !strings.Contains(res8.Content[0].Text, "main") {
		t.Fatalf("res8 expected main in branch list: %s", res8.Content[0].Text)
	}

	// 9. spl_branch_create
	id9, res9 := parseCallResult(t, lines[8])
	if id9 != float64(9) || res9.IsError {
		t.Fatalf("res9 error: %#v", res9)
	}

	// 10. spl_diff
	id10, res10 := parseCallResult(t, lines[9])
	if id10 != float64(10) || res10.IsError {
		t.Fatalf("res10 error: %#v", res10)
	}

	// 11. spl_branches_containing
	id11, res11 := parseCallResult(t, lines[10])
	if id11 != float64(11) || res11.IsError {
		t.Fatalf("res11 error: %#v", res11)
	}
	if !strings.Contains(res11.Content[0].Text, "main") {
		t.Fatalf("res11 expected main containing entity: %s", res11.Content[0].Text)
	}

	// 12. spl_search_expand
	id12, res12 := parseCallResult(t, lines[11])
	if id12 != float64(12) || res12.IsError {
		t.Fatalf("res12 error: %#v", res12)
	}

	// 13. spl_gc
	id13, res13 := parseCallResult(t, lines[12])
	if id13 != float64(13) || res13.IsError {
		t.Fatalf("res13 error: %#v", res13)
	}

	// 14. spl_version
	id14, res14 := parseCallResult(t, lines[13])
	if id14 != float64(14) || res14.IsError {
		t.Fatalf("res14 error: %#v", res14)
	}
	if !strings.Contains(res14.Content[0].Text, "version") {
		t.Fatalf("res14 expected version field: %s", res14.Content[0].Text)
	}

	// 15. spl_remote_set
	id15, res15 := parseCallResult(t, lines[14])
	if id15 != float64(15) || res15.IsError {
		t.Fatalf("res15 error: %#v", res15)
	}

	// 16. spl_remote_show
	id16, res16 := parseCallResult(t, lines[15])
	if id16 != float64(16) || res16.IsError {
		t.Fatalf("res16 error: %#v", res16)
	}
	if !strings.Contains(res16.Content[0].Text, "ws-test") {
		t.Fatalf("res16 expected ws-test: %s", res16.Content[0].Text)
	}

	// 17. spl_remote_remove
	id17, res17 := parseCallResult(t, lines[16])
	if id17 != float64(17) || res17.IsError {
		t.Fatalf("res17 error: %#v", res17)
	}
	if !strings.Contains(res17.Content[0].Text, `"removed":true`) {
		t.Fatalf("res17 expected removed:true: %s", res17.Content[0].Text)
	}
}
