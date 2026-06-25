package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// fakeRunner records the argv it was called with and returns canned output.
type fakeRunner struct {
	gotArgv  []string
	stdout   string
	stderr   string
	exitCode int
}

func (f *fakeRunner) run(_ context.Context, argv []string) ([]byte, []byte, int, error) {
	f.gotArgv = argv
	return []byte(f.stdout), []byte(f.stderr), f.exitCode, nil
}

func newTestServer(t *testing.T, runner runner, dryRun bool) *Server {
	t.Helper()
	tools := Selection{Selectors: []string{"all"}, AllowWrite: true}.Filter(BuildTools(sampleTree()))
	return NewServer(ServerConfig{
		Tools:          tools,
		Runner:         runner,
		MaxOutputBytes: 1024,
		DryRun:         dryRun,
		Version:        "test",
	})
}

// roundtrip sends one request line and decodes the single response.
func roundtrip(t *testing.T, s *Server, request string) rpcResponse {
	t.Helper()
	var out strings.Builder
	if err := s.Serve(context.Background(), strings.NewReader(request+"\n"), &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	var resp rpcResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &resp); err != nil {
		t.Fatalf("decode response %q: %v", out.String(), err)
	}
	return resp
}

func TestInitialize(t *testing.T) {
	s := newTestServer(t, &fakeRunner{}, false)
	resp := roundtrip(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	result := resp.Result.(map[string]any)
	if result["protocolVersion"] != "2025-06-18" {
		t.Fatalf("protocolVersion = %v", result["protocolVersion"])
	}
	if _, ok := result["capabilities"]; !ok {
		t.Fatalf("missing capabilities")
	}
}

func TestToolsList(t *testing.T) {
	s := newTestServer(t, &fakeRunner{}, false)
	resp := roundtrip(t, s, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	result := resp.Result.(map[string]any)
	toolsRaw, ok := result["tools"].([]any)
	if !ok || len(toolsRaw) == 0 {
		t.Fatalf("expected tools, got %v", result["tools"])
	}
	// Validate the first descriptor has required MCP fields.
	first := toolsRaw[0].(map[string]any)
	for _, key := range []string{"name", "description", "inputSchema"} {
		if _, ok := first[key]; !ok {
			t.Fatalf("descriptor missing %q: %v", key, first)
		}
	}
}

func TestToolsCallExecutesAndParsesJSON(t *testing.T) {
	runner := &fakeRunner{stdout: `{"ok":true,"count":3}`, exitCode: 0}
	s := newTestServer(t, runner, false)

	resp := roundtrip(t, s, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"builds_list","arguments":{"app":"123","limit":5}}}`)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	// argv reconstruction
	wantArgv := []string{"builds", "list", "--app", "123", "--limit", "5"}
	if strings.Join(runner.gotArgv, " ") != strings.Join(wantArgv, " ") {
		t.Fatalf("argv = %v, want %v", runner.gotArgv, wantArgv)
	}

	result := resp.Result.(map[string]any)
	if result["isError"] != false {
		t.Fatalf("isError = %v", result["isError"])
	}
	sc := result["structuredContent"].(map[string]any)
	if sc["risk"] != "read" {
		t.Fatalf("risk = %v", sc["risk"])
	}
	// stdout should be parsed JSON (an object), not a string.
	stdout, ok := sc["stdout"].(map[string]any)
	if !ok {
		t.Fatalf("stdout not parsed as JSON object: %T %v", sc["stdout"], sc["stdout"])
	}
	if stdout["count"].(float64) != 3 {
		t.Fatalf("stdout.count = %v", stdout["count"])
	}
}

func TestToolsCallNonZeroExitIsError(t *testing.T) {
	runner := &fakeRunner{stdout: "boom", stderr: "failed", exitCode: 1}
	s := newTestServer(t, runner, false)
	resp := roundtrip(t, s, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"builds_expire","arguments":{}}}`)
	result := resp.Result.(map[string]any)
	if result["isError"] != true {
		t.Fatalf("expected isError true for non-zero exit, got %v", result["isError"])
	}
	sc := result["structuredContent"].(map[string]any)
	if sc["exit_code"].(float64) != 1 {
		t.Fatalf("exit_code = %v", sc["exit_code"])
	}
	if sc["stderr"] != "failed" {
		t.Fatalf("stderr = %v", sc["stderr"])
	}
}

func TestToolsCallUnknownTool(t *testing.T) {
	s := newTestServer(t, &fakeRunner{}, false)
	resp := roundtrip(t, s, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"does_not_exist"}}`)
	if resp.Error == nil || resp.Error.Code != codeInvalidParams {
		t.Fatalf("expected invalid params error, got %+v", resp.Error)
	}
}

func TestDryRunDoesNotExecute(t *testing.T) {
	runner := &fakeRunner{stdout: "should not run"}
	s := newTestServer(t, runner, true)
	resp := roundtrip(t, s, `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"builds_expire","arguments":{}}}`)
	if runner.gotArgv != nil {
		t.Fatalf("dry-run should not invoke runner, got argv %v", runner.gotArgv)
	}
	sc := resp.Result.(map[string]any)["structuredContent"].(map[string]any)
	if sc["dry_run"] != true {
		t.Fatalf("expected dry_run true, got %v", sc["dry_run"])
	}
	if sc["command"] != "asc builds expire" {
		t.Fatalf("command = %v", sc["command"])
	}
}

func TestNotificationProducesNoResponse(t *testing.T) {
	s := newTestServer(t, &fakeRunner{}, false)
	var out strings.Builder
	input := `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"
	if err := s.Serve(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("notification should produce no response, got %q", out.String())
	}
}

func TestUnknownMethod(t *testing.T) {
	s := newTestServer(t, &fakeRunner{}, false)
	resp := roundtrip(t, s, `{"jsonrpc":"2.0","id":7,"method":"bogus/method"}`)
	if resp.Error == nil || resp.Error.Code != codeMethodNotFound {
		t.Fatalf("expected method not found, got %+v", resp.Error)
	}
}

func TestParseErrorOnGarbage(t *testing.T) {
	s := newTestServer(t, &fakeRunner{}, false)
	resp := roundtrip(t, s, `not json`)
	if resp.Error == nil || resp.Error.Code != codeParseError {
		t.Fatalf("expected parse error, got %+v", resp.Error)
	}
}

func TestStdoutTruncation(t *testing.T) {
	big := strings.Repeat("x", 2000)
	runner := &fakeRunner{stdout: big}
	tools := Selection{Selectors: []string{"all"}, AllowWrite: true}.Filter(BuildTools(sampleTree()))
	s := NewServer(ServerConfig{Tools: tools, Runner: runner, MaxOutputBytes: 100, Version: "t"})
	resp := roundtrip(t, s, `{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"builds_list"}}`)
	sc := resp.Result.(map[string]any)["structuredContent"].(map[string]any)
	if sc["truncated"] != true {
		t.Fatalf("expected truncated true, got %v", sc["truncated"])
	}
	if len(sc["stdout"].(string)) != 100 {
		t.Fatalf("stdout not capped to 100 bytes, got %d", len(sc["stdout"].(string)))
	}
}
