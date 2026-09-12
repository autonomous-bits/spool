package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// ProtocolVersion is the MCP protocol specification version supported by this server.
const ProtocolVersion = "2024-11-05"

// JSONRPCRequest represents an incoming JSON-RPC 2.0 message.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents an outgoing JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      any           `json:"id"`
	Result  any           `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

// JSONRPCError defines the standard JSON-RPC 2.0 error object.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// ToolHandler handles execution of an MCP tool.
type ToolHandler func(ctx context.Context, args json.RawMessage) (any, error)

// Tool represents a registered MCP tool specification and its execution handler.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Handler     ToolHandler    `json:"-"`
}

// ContentItem represents a content block returned by an MCP tool.
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// CallToolResult represents the payload returned for tools/call.
type CallToolResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// Server implements an MCP server over an input and output stream.
type Server struct {
	name    string
	version string
	tools   map[string]Tool
	mu      sync.RWMutex
}

// NewServer creates a new MCP server with the specified name and version.
func NewServer(name, version string) *Server {
	return &Server{
		name:    name,
		version: version,
		tools:   make(map[string]Tool),
	}
}

// RegisterTool registers a tool with the server.
func (s *Server) RegisterTool(tool Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[tool.Name] = tool
}

// RegisterTools registers multiple tools with the server.
func (s *Server) RegisterTools(tools []Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range tools {
		s.tools[t.Name] = t
	}
}

// Serve reads JSON-RPC requests line-by-line from in, processes them, and writes
// responses to out until in reaches EOF or the context is cancelled.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	// Allow large payloads up to 64MB for substantial graph query outputs
	const maxScanTokenSize = 64 * 1024 * 1024
	scanner.Buffer(make([]byte, 1024*1024), maxScanTokenSize)

	var writeMu sync.Mutex
	writeResponse := func(resp JSONRPCResponse) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		data, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		data = append(data, '\n')
		_, err = out.Write(data)
		return err
	}

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			_ = writeResponse(JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      nil,
				Error: &JSONRPCError{
					Code:    -32700,
					Message: "Parse error: " + err.Error(),
				},
			})
			continue
		}

		if req.ID == nil && req.Method != "notifications/initialized" {
			// Notification that is not initialized
			continue
		}

		s.handleRequest(ctx, req, writeResponse)
	}

	return scanner.Err()
}

func (s *Server) handleRequest(ctx context.Context, req JSONRPCRequest, respond func(JSONRPCResponse) error) {
	switch req.Method {
	case "initialize":
		_ = respond(JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": ProtocolVersion,
				"serverInfo": map[string]string{
					"name":    s.name,
					"version": s.version,
				},
				"capabilities": map[string]any{
					"tools": map[string]any{
						"listChanged": false,
					},
				},
			},
		})

	case "notifications/initialized":
		// No-op per MCP spec

	case "ping":
		_ = respond(JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{},
		})

	case "tools/list":
		s.mu.RLock()
		toolsList := make([]map[string]any, 0, len(s.tools))
		for _, tool := range s.tools {
			toolsList = append(toolsList, map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"inputSchema": tool.InputSchema,
			})
		}
		s.mu.RUnlock()

		_ = respond(JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"tools": toolsList,
			},
		})

	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			_ = respond(JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &JSONRPCError{
					Code:    -32602,
					Message: fmt.Sprintf("Invalid params: %v", err),
				},
			})
			return
		}

		s.mu.RLock()
		tool, exists := s.tools[params.Name]
		s.mu.RUnlock()

		if !exists {
			_ = respond(JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: CallToolResult{
					IsError: true,
					Content: []ContentItem{
						{
							Type: "text",
							Text: fmt.Sprintf("tool not found: %q", params.Name),
						},
					},
				},
			})
			return
		}

		result, err := tool.Handler(ctx, params.Arguments)
		if err != nil {
			_ = respond(JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: CallToolResult{
					IsError: true,
					Content: []ContentItem{
						{
							Type: "text",
							Text: fmt.Sprintf("error: %v", err),
						},
					},
				},
			})
			return
		}

		var text string
		switch v := result.(type) {
		case string:
			text = v
		case []byte:
			text = string(v)
		default:
			encoded, encodeErr := json.Marshal(v)
			if encodeErr != nil {
				_ = respond(JSONRPCResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Result: CallToolResult{
						IsError: true,
						Content: []ContentItem{
							{
								Type: "text",
								Text: fmt.Sprintf("failed to encode tool result: %v", encodeErr),
							},
						},
					},
				})
				return
			}
			text = string(encoded)
		}

		_ = respond(JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: CallToolResult{
				IsError: false,
				Content: []ContentItem{
					{
						Type: "text",
						Text: text,
					},
				},
			},
		})

	default:
		_ = respond(JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    -32601,
				Message: fmt.Sprintf("Method not found: %s", req.Method),
			},
		})
	}
}
