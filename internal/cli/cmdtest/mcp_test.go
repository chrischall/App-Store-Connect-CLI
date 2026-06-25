package cmdtest

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

// TestMCPListToolsReadOnlyDefault exercises the real command tree through the
// `asc mcp --list-tools` path and asserts the read-only default surface.
func TestMCPListToolsReadOnlyDefault(t *testing.T) {
	root := RootCommand("1.2.3")
	root.FlagSet.SetOutput(io.Discard)

	stdout, stderr := captureOutput(t, func() {
		if err := root.Parse([]string{"mcp", "--list-tools"}); err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if err := root.Run(context.Background()); err != nil {
			t.Fatalf("run error: %v", err)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	var payload struct {
		Count int `json:"count"`
		Tools []struct {
			Name    string `json:"name"`
			Service string `json:"service"`
			Risk    string `json:"risk"`
			Command string `json:"command"`
		} `json:"tools"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("unmarshal: %v\nstdout=%s", err, stdout)
	}
	if payload.Count == 0 || payload.Count != len(payload.Tools) {
		t.Fatalf("count mismatch: %d vs %d tools", payload.Count, len(payload.Tools))
	}

	sawBuildsList := false
	for _, tl := range payload.Tools {
		if tl.Risk != "read" {
			t.Fatalf("default surface exposed non-read tool %q (%s)", tl.Name, tl.Risk)
		}
		if !strings.HasPrefix(tl.Command, "asc ") {
			t.Fatalf("tool %q has malformed command %q", tl.Name, tl.Command)
		}
		if tl.Name == "builds_list" {
			sawBuildsList = true
		}
	}
	if !sawBuildsList {
		t.Fatalf("expected builds_list in the default read-only surface")
	}
}

// TestMCPListToolsWriteSurface asserts that write tools appear only with
// --allow-write and that the surface grows accordingly.
func TestMCPListToolsWriteSurface(t *testing.T) {
	countTools := func(args []string) (int, map[string]int) {
		root := RootCommand("1.2.3")
		root.FlagSet.SetOutput(io.Discard)
		stdout, _ := captureOutput(t, func() {
			if err := root.Parse(args); err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if err := root.Run(context.Background()); err != nil {
				t.Fatalf("run error: %v", err)
			}
		})
		var payload struct {
			Count int `json:"count"`
			Tools []struct {
				Risk string `json:"risk"`
			} `json:"tools"`
		}
		if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		byRisk := map[string]int{}
		for _, tl := range payload.Tools {
			byRisk[tl.Risk]++
		}
		return payload.Count, byRisk
	}

	readOnly, readRisks := countTools([]string{"mcp", "--list-tools"})
	if readRisks["write"] != 0 {
		t.Fatalf("read-only surface contained write tools: %v", readRisks)
	}

	all, allRisks := countTools([]string{"mcp", "--allow-tool", "all", "--allow-write", "--list-tools"})
	if allRisks["write"] == 0 {
		t.Fatalf("expected write tools with --allow-write, got %v", allRisks)
	}
	if all <= readOnly {
		t.Fatalf("expected larger surface with write tools: all=%d read=%d", all, readOnly)
	}
}
