// Package mcp exposes the asc command surface as a Model Context Protocol
// (MCP) stdio server. It mirrors the design of gogcli's `gog mcp` server:
// each leaf CLI command is registered as one typed MCP tool that maps to a
// single asc operation, read tools are exposed by default, and write tools
// stay hidden unless explicitly allowed.
package mcp

import (
	"flag"
	"sort"
	"strings"

	"github.com/peterbourgon/ff/v3/ffcli"
)

// Risk classifies a tool as read-only or mutating. Read tools are exposed by
// default; write tools require --allow-write in addition to an --allow-tool
// match.
type Risk string

const (
	// RiskRead marks a tool that does not mutate App Store Connect state.
	RiskRead Risk = "read"
	// RiskWrite marks a tool that may mutate state (create/update/delete/etc).
	RiskWrite Risk = "write"
)

// Tool is one MCP tool derived from a leaf asc command.
type Tool struct {
	// Name is the MCP tool name, e.g. "builds_list" for `asc builds list`.
	Name string
	// Service is the top-level command group, e.g. "builds".
	Service string
	// Path is the command path without the leading "asc", e.g. ["builds","list"].
	Path []string
	// Description is the human-readable summary shown to MCP clients.
	Description string
	// Risk is the read/write classification.
	Risk Risk
	// cmd is the underlying leaf command, retained for schema + flag typing.
	cmd *ffcli.Command
}

// readVerbs are leaf command names (or name fragments) that never mutate state.
var readVerbs = map[string]struct{}{
	"list": {}, "get": {}, "show": {}, "view": {}, "info": {}, "describe": {},
	"status": {}, "count": {}, "latest": {}, "related": {}, "metrics": {},
	"download": {}, "export": {}, "history": {}, "search": {}, "summary": {},
	"read": {}, "fetch": {}, "inspect": {}, "doctor": {}, "diff": {}, "tree": {},
	"paths": {}, "schema": {}, "ls": {}, "cat": {}, "tail": {}, "uploads": {},
	"dsyms": {}, "relationships": {}, "report": {}, "reports": {}, "check": {},
	"whoami": {}, "validate": {}, "verify": {}, "open": {}, "url": {},
}

// writeVerbs are leaf command names (or name fragments) that may mutate state.
var writeVerbs = map[string]struct{}{
	"create": {}, "add": {}, "new": {}, "update": {}, "set": {}, "edit": {},
	"modify": {}, "patch": {}, "delete": {}, "remove": {}, "rm": {}, "destroy": {},
	"cancel": {}, "submit": {}, "publish": {}, "release": {}, "upload": {},
	"push": {}, "expire": {}, "enable": {}, "disable": {}, "revoke": {},
	"register": {}, "rotate": {}, "generate": {}, "gen": {}, "sync": {},
	"send": {}, "invite": {}, "approve": {}, "reject": {}, "accept": {},
	"decline": {}, "clear": {}, "reset": {}, "install": {}, "uninstall": {},
	"link": {}, "unlink": {}, "attach": {}, "detach": {}, "assign": {},
	"unassign": {}, "start": {}, "stop": {}, "restart": {}, "trigger": {},
	"apply": {}, "import": {}, "init": {}, "migrate": {}, "sign": {},
	"notarize": {}, "deploy": {}, "rename": {}, "replace": {}, "move": {},
	"copy": {}, "duplicate": {}, "set-live": {}, "rollback": {}, "wait": {},
	"expire-all": {}, "renew": {},
}

// classifyRisk decides whether a leaf command is read-only or mutating.
// The leaf name is checked first, then any path token; commands that match
// neither set default to RiskWrite so unknown operations stay hidden by
// default (safety first, matching gogcli's write-allowlist model).
func classifyRisk(path []string) Risk {
	if len(path) == 0 {
		return RiskWrite
	}
	leaf := strings.ToLower(path[len(path)-1])
	if _, ok := writeVerbs[leaf]; ok {
		return RiskWrite
	}
	if _, ok := readVerbs[leaf]; ok {
		return RiskRead
	}
	// Inspect earlier path tokens for a decisive verb (e.g. "builds list app").
	for i := len(path) - 1; i >= 0; i-- {
		tok := strings.ToLower(path[i])
		if _, ok := writeVerbs[tok]; ok {
			return RiskWrite
		}
		if _, ok := readVerbs[tok]; ok {
			return RiskRead
		}
	}
	return RiskWrite
}

// toolName converts a command path to an MCP tool name: tokens joined with
// "_" and any "-" within a token normalized to "_" (e.g. ["age-rating","set"]
// becomes "age_rating_set").
func toolName(path []string) string {
	parts := make([]string, 0, len(path))
	for _, p := range path {
		parts = append(parts, strings.ReplaceAll(p, "-", "_"))
	}
	return strings.Join(parts, "_")
}

// hiddenCommand reports whether a command should be excluded from the tool
// surface (deprecated/removed/alias commands).
func hiddenCommand(cmd *ffcli.Command) bool {
	help := strings.TrimSpace(cmd.ShortHelp)
	return strings.HasPrefix(help, "DEPRECATED:") ||
		strings.HasPrefix(help, "REMOVED:") ||
		strings.HasPrefix(help, "Compatibility alias:")
}

// BuildTools walks the registered command tree and returns one Tool per leaf
// command (a command with no subcommands and a runnable Exec). Results are
// sorted by tool name for deterministic output.
func BuildTools(commands []*ffcli.Command) []Tool {
	var tools []Tool
	for _, cmd := range commands {
		collectTools(&tools, cmd, nil)
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools
}

func collectTools(tools *[]Tool, cmd *ffcli.Command, parents []string) {
	if cmd == nil || hiddenCommand(cmd) {
		return
	}
	path := append(append([]string{}, parents...), cmd.Name)

	if len(cmd.Subcommands) > 0 {
		for _, sub := range cmd.Subcommands {
			collectTools(tools, sub, path)
		}
		return
	}
	// Leaf command. Skip pure group placeholders that cannot run.
	if cmd.Exec == nil {
		return
	}

	description := strings.TrimSpace(cmd.ShortHelp)
	if description == "" {
		description = strings.TrimSpace(cmd.ShortUsage)
	}
	*tools = append(*tools, Tool{
		Name:        toolName(path),
		Service:     path[0],
		Path:        path,
		Description: description,
		Risk:        classifyRisk(path),
		cmd:         cmd,
	})
}

// boolFlagNames returns the set of bool-typed flag names on the tool's command.
func (t Tool) boolFlagNames() map[string]struct{} {
	out := map[string]struct{}{}
	if t.cmd == nil || t.cmd.FlagSet == nil {
		return out
	}
	t.cmd.FlagSet.VisitAll(func(f *flag.Flag) {
		if isBoolFlag(f) {
			out[f.Name] = struct{}{}
		}
	})
	return out
}

func isBoolFlag(f *flag.Flag) bool {
	type boolFlag interface{ IsBoolFlag() bool }
	v, ok := f.Value.(boolFlag)
	return ok && v.IsBoolFlag()
}
