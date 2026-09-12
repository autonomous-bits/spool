package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestMCPServerLifecycle(t *testing.T) {
	server := NewServer("test-server", "1.0.0")
	server.RegisterTool(Tool{
		Name:        "echo",
		Description: "Echo input text",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{"type": "string"},
			},
			"required": []string{"text"},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Text == "error" {
				return nil, errors.New("simulated error")
			}
			return map[string]string{"echoed": in.Text}, nil
		},
	})

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"echo","arguments":{"text":"hello"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"echo","arguments":{"text":"error"}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"unknown","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"unsupported_method"}`,
		`not valid json`,
	}, "\n") + "\n"

	var out bytes.Buffer
	err := server.Serve(context.Background(), strings.NewReader(input), &out)
	if err != nil {
		t.Fatalf("Serve failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 8 {
		t.Fatalf("expected 8 responses, got %d: %s", len(lines), out.String())
	}

	// 1. initialize
	var resp1 JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[0]), &resp1); err != nil {
		t.Fatalf("failed to decode resp1: %v", err)
	}
	if resp1.ID != float64(1) || resp1.Error != nil {
		t.Fatalf("resp1 error: %#v", resp1)
	}

	// 2. ping
	var resp2 JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[1]), &resp2); err != nil {
		t.Fatalf("failed to decode resp2: %v", err)
	}
	if resp2.ID != float64(2) {
		t.Fatalf("resp2 id mismatch: %v", resp2.ID)
	}

	// 3. tools/list
	var resp3 JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[2]), &resp3); err != nil {
		t.Fatalf("failed to decode resp3: %v", err)
	}
	result3, ok := resp3.Result.(map[string]any)
	if !ok || result3["tools"] == nil {
		t.Fatalf("resp3 missing tools: %#v", resp3)
	}

	// 4. tools/call (success)
	var resp4 JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[3]), &resp4); err != nil {
		t.Fatalf("failed to decode resp4: %v", err)
	}
	result4Bytes, _ := json.Marshal(resp4.Result)
	var callResult4 CallToolResult
	_ = json.Unmarshal(result4Bytes, &callResult4)
	if callResult4.IsError || len(callResult4.Content) != 1 || !strings.Contains(callResult4.Content[0].Text, "hello") {
		t.Fatalf("resp4 unexpected call result: %#v", callResult4)
	}

	// 5. tools/call (handler error)
	var resp5 JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[4]), &resp5); err != nil {
		t.Fatalf("failed to decode resp5: %v", err)
	}
	result5Bytes, _ := json.Marshal(resp5.Result)
	var callResult5 CallToolResult
	_ = json.Unmarshal(result5Bytes, &callResult5)
	if !callResult5.IsError || !strings.Contains(callResult5.Content[0].Text, "simulated error") {
		t.Fatalf("resp5 expected error result: %#v", callResult5)
	}

	// 6. tools/call (unknown tool)
	var resp6 JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[5]), &resp6); err != nil {
		t.Fatalf("failed to decode resp6: %v", err)
	}
	result6Bytes, _ := json.Marshal(resp6.Result)
	var callResult6 CallToolResult
	_ = json.Unmarshal(result6Bytes, &callResult6)
	if !callResult6.IsError || !strings.Contains(callResult6.Content[0].Text, "tool not found") {
		t.Fatalf("resp6 expected tool not found: %#v", callResult6)
	}

	// 7. unsupported method
	var resp7 JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[6]), &resp7); err != nil {
		t.Fatalf("failed to decode resp7: %v", err)
	}
	if resp7.Error == nil || resp7.Error.Code != -32601 {
		t.Fatalf("resp7 expected method not found error: %#v", resp7)
	}

	// 8. parse error
	var resp8 JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[7]), &resp8); err != nil {
		t.Fatalf("failed to decode resp8: %v", err)
	}
	if resp8.Error == nil || resp8.Error.Code != -32700 {
		t.Fatalf("resp8 expected parse error: %#v", resp8)
	}
}
