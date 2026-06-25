package mcp

import (
	"flag"
	"strings"
)

// jsonSchema is a minimal JSON Schema object describing a tool's input.
type jsonSchema struct {
	Type       string                `json:"type"`
	Properties map[string]propSchema `json:"properties"`
	// AdditionalProperties is left permissive so clients may pass extra flags.
}

type propSchema struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// InputSchema derives a JSON Schema for a tool from its command's flags plus a
// generic positional "args" array. Bool flags map to boolean; everything else
// maps to string (the CLI parses string values itself). Reserved flag names
// that are managed by the server (e.g. --output) are still surfaced so agents
// can request specific formats.
func (t Tool) InputSchema() jsonSchema {
	props := map[string]propSchema{
		"args": {
			Type:        "array",
			Description: "Positional arguments passed to the command after flags.",
		},
	}
	if t.cmd != nil && t.cmd.FlagSet != nil {
		t.cmd.FlagSet.VisitAll(func(f *flag.Flag) {
			if f == nil {
				return
			}
			ps := propSchema{Type: "string", Description: strings.TrimSpace(f.Usage)}
			if isBoolFlag(f) {
				ps.Type = "boolean"
			}
			props[f.Name] = ps
		})
	}
	return jsonSchema{Type: "object", Properties: props}
}
