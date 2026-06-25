package mcp

import (
	"path"
	"strings"
)

// Selection captures the access-control configuration for a server: which
// tool selectors and suites are allowed and whether write tools are permitted.
type Selection struct {
	// Selectors is the parsed --allow-tool list (services, globs, exact names,
	// read/write/all). Empty (with no Suites) means "all read tools".
	Selectors []string
	// Suites is the parsed --tool-suite list of named service groups.
	Suites []string
	// AllowWrite mirrors --allow-write; required to expose any write tool.
	AllowWrite bool
}

// ParseSelectors splits a comma/space separated --allow-tool value into
// normalized selectors. Dotted selectors (gmail.*) are accepted and treated
// the same as underscore selectors (gmail_*) for parity with gogcli.
func ParseSelectors(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// normalizeSelector lowercases and converts "." separators to "_" so that
// "gmail.*"-style selectors behave like "gmail_*".
func normalizeSelector(sel string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(sel)), ".", "_")
}

// matchSelector reports whether a single --allow-tool selector matches the
// tool. Supported selector forms:
//
//   - / all                     -> every tool
//     read         / write         -> tools of that risk class
//     builds                       -> the "builds" service (any tool under it)
//     builds_*     / builds.*      -> glob over the tool name
//     builds_list  / builds.list   -> an exact tool name
//
// Named suites are selected with the separate --tool-suite flag (see suites.go),
// so service names like "analytics" here always mean the service, never a suite.
func matchSelector(sel string, t Tool) bool {
	s := normalizeSelector(sel)
	switch s {
	case "", "*", "all":
		return true
	case "read":
		return t.Risk == RiskRead
	case "write":
		return t.Risk == RiskWrite
	}

	name := strings.ToLower(t.Name)
	service := strings.ToLower(strings.ReplaceAll(t.Service, "-", "_"))

	if strings.ContainsAny(s, "*?[") {
		if ok, err := path.Match(s, name); err == nil && ok {
			return true
		}
		// Allow a bare-service glob like "builds*" to match the service too.
		if ok, err := path.Match(s, service); err == nil && ok {
			return true
		}
		return false
	}

	if s == name {
		return true
	}
	// A bare service token matches every tool under that service.
	return s == service
}

// matchSuite reports whether a --tool-suite name includes the tool's service.
// Unknown suite names match nothing.
func matchSuite(name string, t Tool) bool {
	set := suiteServices(normalizeSelector(name))
	if set == nil {
		return false
	}
	service := strings.ToLower(strings.ReplaceAll(t.Service, "-", "_"))
	_, ok := set[service]
	return ok
}

// allowed reports whether a tool is exposed under this selection. Write tools
// additionally require AllowWrite. When neither selectors nor suites are
// configured, the default surface is all read tools.
func (sel Selection) allowed(t Tool) bool {
	if t.Risk == RiskWrite && !sel.AllowWrite {
		return false
	}
	if len(sel.Selectors) == 0 && len(sel.Suites) == 0 {
		return t.Risk == RiskRead
	}
	for _, s := range sel.Selectors {
		if matchSelector(s, t) {
			return true
		}
	}
	for _, suite := range sel.Suites {
		if matchSuite(suite, t) {
			return true
		}
	}
	return false
}

// Filter returns the subset of tools exposed under this selection, preserving
// input order.
func (sel Selection) Filter(tools []Tool) []Tool {
	out := make([]Tool, 0, len(tools))
	for _, t := range tools {
		if sel.allowed(t) {
			out = append(out, t)
		}
	}
	return out
}
