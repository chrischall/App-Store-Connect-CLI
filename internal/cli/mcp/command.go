package mcp

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/peterbourgon/ff/v3/ffcli"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/cli/shared"
)

const (
	defaultTimeoutSeconds = 60
	defaultMaxOutputBytes = 102400
)

// MCPCommand returns the `asc mcp` command. It receives a provider for the
// registered command tree (same pattern as `asc search`) to avoid an import
// cycle with the registry.
func MCPCommand(commands func() []*ffcli.Command, version string) *ffcli.Command {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	allowTool := fs.String("allow-tool", "", "Comma-separated tool selectors to expose (e.g. 'read', 'builds', 'builds_*', 'apps_get'). Empty exposes all read tools.")
	allowWrite := fs.Bool("allow-write", false, "Expose write (mutating) tools. Required in addition to --allow-tool matches.")
	timeoutSeconds := fs.Int("timeout-seconds", defaultTimeoutSeconds, "Per-tool execution timeout in seconds (0 disables).")
	maxOutputBytes := fs.Int("max-output-bytes", defaultMaxOutputBytes, "Maximum bytes of stdout/stderr returned per tool call (0 disables the cap).")
	dryRun := fs.Bool("dry-run", false, "Resolve and return the asc command line for each call without executing it.")
	listTools := fs.Bool("list-tools", false, "Print the resolved tool set as JSON and exit instead of serving.")

	return &ffcli.Command{
		Name:       "mcp",
		ShortUsage: "asc mcp [flags]",
		ShortHelp:  "Run a Model Context Protocol (MCP) stdio server exposing asc commands as tools.",
		LongHelp: `Run a Model Context Protocol (MCP) stdio server exposing asc commands as tools.

Each leaf asc command is registered as one typed MCP tool named after its
command path (e.g. ` + "`asc builds list`" + ` becomes the ` + "`builds_list`" + ` tool).
Tools map to a single asc operation and return a structured result with the
tool name, service, risk level, exit code, parsed stdout, and stderr.

Read-only tools are exposed by default. Write (mutating) tools are hidden
unless you both match them with --allow-tool and pass --allow-write.

Selectors for --allow-tool (comma-separated):
  read            all read-only tools (the default when --allow-tool is empty)
  write           all write tools (still requires --allow-write)
  all, *          every tool
  builds          every tool under the builds service
  builds_*        glob over tool names
  builds_list     an exact tool name

Examples:
  # Read-only server (safe default)
  asc mcp

  # Expose Builds read + write tools
  asc mcp --allow-write --allow-tool 'builds_*'

  # Inspect which tools would be exposed
  asc mcp --allow-write --allow-tool all --list-tools

Configure your MCP client to launch ` + "`asc mcp`" + ` over stdio.`,
		FlagSet:   fs,
		UsageFunc: shared.DefaultUsageFunc,
		Exec: func(ctx context.Context, args []string) error {
			selection := Selection{
				Selectors:  ParseSelectors(*allowTool),
				AllowWrite: *allowWrite,
			}
			tools := selection.Filter(BuildTools(commands()))

			if *listTools {
				return printToolList(os.Stdout, tools)
			}

			binPath, err := os.Executable()
			if err != nil {
				return shared.UsageError(fmt.Sprintf("cannot resolve asc binary path: %v", err))
			}

			server := NewServer(ServerConfig{
				Tools:          tools,
				Runner:         execRunner{binPath: binPath},
				Timeout:        time.Duration(*timeoutSeconds) * time.Second,
				MaxOutputBytes: *maxOutputBytes,
				DryRun:         *dryRun,
				Version:        version,
			})

			fmt.Fprintf(os.Stderr, "asc mcp: serving %d tool(s) over stdio (write=%t)\n", len(tools), *allowWrite)
			return server.Serve(ctx, os.Stdin, os.Stdout)
		},
	}
}

type toolListEntry struct {
	Name        string `json:"name"`
	Service     string `json:"service"`
	Risk        Risk   `json:"risk"`
	Command     string `json:"command"`
	Description string `json:"description"`
}

func printToolList(w *os.File, tools []Tool) error {
	entries := make([]toolListEntry, 0, len(tools))
	for _, t := range tools {
		entries = append(entries, toolListEntry{
			Name:        t.Name,
			Service:     t.Service,
			Risk:        t.Risk,
			Command:     "asc " + joinPath(t.Path),
			Description: t.Description,
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(map[string]any{"count": len(entries), "tools": entries})
}
