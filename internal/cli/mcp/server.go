package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

const (
	// protocolVersion is the MCP protocol version this server speaks when the
	// client does not request a specific one.
	protocolVersion = "2025-06-18"
	serverName      = "asc-mcp"
)

// Server speaks newline-delimited JSON-RPC 2.0 over stdio, exposing the
// selected asc tools.
type Server struct {
	tools          []Tool
	byName         map[string]Tool
	runner         runner
	timeout        time.Duration
	maxOutputBytes int
	dryRun         bool
	version        string

	mu  sync.Mutex // serializes writes to out
	out io.Writer
}

// ServerConfig configures a Server.
type ServerConfig struct {
	Tools          []Tool
	Runner         runner
	Timeout        time.Duration
	MaxOutputBytes int
	DryRun         bool
	Version        string
}

// NewServer builds a Server from the resolved (already filtered) tool set.
func NewServer(cfg ServerConfig) *Server {
	byName := make(map[string]Tool, len(cfg.Tools))
	for _, t := range cfg.Tools {
		byName[t.Name] = t
	}
	return &Server{
		tools:          cfg.Tools,
		byName:         byName,
		runner:         cfg.Runner,
		timeout:        cfg.Timeout,
		maxOutputBytes: cfg.MaxOutputBytes,
		dryRun:         cfg.DryRun,
		version:        cfg.Version,
	}
}

// JSON-RPC envelope types.

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

// Serve reads JSON-RPC messages from r until EOF, writing responses to w.
// Notifications (requests without an id) never produce a response.
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	s.out = w
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(trimSpace(line)) == 0 {
			continue
		}
		s.handleLine(ctx, line)
	}
	return scanner.Err()
}

func trimSpace(b []byte) []byte {
	start, end := 0, len(b)
	for start < end && isSpace(b[start]) {
		start++
	}
	for end > start && isSpace(b[end-1]) {
		end--
	}
	return b[start:end]
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

func (s *Server) handleLine(ctx context.Context, line []byte) {
	var req rpcRequest
	if err := json.Unmarshal(line, &req); err != nil {
		s.writeError(nil, codeParseError, "parse error", err.Error())
		return
	}
	isNotification := len(req.ID) == 0
	if req.Method == "" {
		if !isNotification {
			s.writeError(req.ID, codeInvalidRequest, "invalid request: missing method", nil)
		}
		return
	}
	result, rpcErr := s.dispatch(ctx, req)
	if isNotification {
		return // notifications get no reply
	}
	if rpcErr != nil {
		s.writeError(req.ID, rpcErr.Code, rpcErr.Message, rpcErr.Data)
		return
	}
	s.writeResult(req.ID, result)
}

func (s *Server) dispatch(ctx context.Context, req rpcRequest) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "tools/list":
		return s.handleToolsList()
	case "tools/call":
		return s.handleToolsCall(ctx, req)
	case "ping":
		return map[string]any{}, nil
	case "notifications/initialized", "notifications/cancelled":
		return map[string]any{}, nil
	default:
		return nil, &rpcError{Code: codeMethodNotFound, Message: "method not found: " + req.Method}
	}
}

func (s *Server) handleInitialize(req rpcRequest) (any, *rpcError) {
	version := protocolVersion
	if len(req.Params) > 0 {
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if err := json.Unmarshal(req.Params, &p); err == nil && p.ProtocolVersion != "" {
			version = p.ProtocolVersion
		}
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities": map[string]any{
			"tools": map[string]any{"listChanged": false},
		},
		"serverInfo": map[string]any{
			"name":    serverName,
			"version": s.version,
		},
	}, nil
}

type toolDescriptor struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	InputSchema jsonSchema `json:"inputSchema"`
	Annotations toolAnno   `json:"annotations"`
}

type toolAnno struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    bool   `json:"readOnlyHint"`
	DestructiveHint bool   `json:"destructiveHint"`
}

func (s *Server) handleToolsList() (any, *rpcError) {
	descriptors := make([]toolDescriptor, 0, len(s.tools))
	for _, t := range s.tools {
		descriptors = append(descriptors, toolDescriptor{
			Name:        t.Name,
			Description: s.describe(t),
			InputSchema: t.InputSchema(),
			Annotations: toolAnno{
				Title:           "asc " + joinPath(t.Path),
				ReadOnlyHint:    t.Risk == RiskRead,
				DestructiveHint: t.Risk == RiskWrite,
			},
		})
	}
	return map[string]any{"tools": descriptors}, nil
}

func (s *Server) describe(t Tool) string {
	desc := t.Description
	if desc == "" {
		desc = "Run `asc " + joinPath(t.Path) + "`."
	}
	return fmt.Sprintf("[%s] %s", t.Risk, desc)
}

func (s *Server) handleToolsCall(ctx context.Context, req rpcRequest) (any, *rpcError) {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &rpcError{Code: codeInvalidParams, Message: "invalid params", Data: err.Error()}
		}
	}
	tool, ok := s.byName[params.Name]
	if !ok {
		return nil, &rpcError{Code: codeInvalidParams, Message: "unknown tool: " + params.Name}
	}
	if params.Arguments == nil {
		params.Arguments = map[string]any{}
	}

	result, err := s.execute(ctx, tool, params.Arguments)
	if err != nil {
		return nil, &rpcError{Code: codeInternalError, Message: err.Error()}
	}

	payload, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, &rpcError{Code: codeInternalError, Message: err.Error()}
	}
	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": string(payload)},
		},
		"isError":           result.ExitCode != 0,
		"structuredContent": result,
	}, nil
}

func (s *Server) writeResult(id json.RawMessage, result any) {
	s.writeMessage(rpcResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func (s *Server) writeError(id json.RawMessage, code int, message string, data any) {
	s.writeMessage(rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message, Data: data}})
}

func (s *Server) writeMessage(resp rpcResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	enc := json.NewEncoder(s.out)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(resp) // newline-delimited; Encode appends '\n'
}

func joinPath(path []string) string {
	out := ""
	for i, p := range path {
		if i > 0 {
			out += " "
		}
		out += p
	}
	return out
}
