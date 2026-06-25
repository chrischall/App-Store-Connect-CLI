package mcp

import (
	"context"
	"flag"
	"reflect"
	"testing"

	"github.com/peterbourgon/ff/v3/ffcli"
)

// sampleTree builds a small synthetic command tree mirroring asc's shape.
func sampleTree() []*ffcli.Command {
	buildsList := &ffcli.Command{
		Name:      "list",
		ShortHelp: "List builds for an app.",
		FlagSet:   flag.NewFlagSet("list", flag.ContinueOnError),
		Exec:      func(context.Context, []string) error { return nil },
	}
	buildsList.FlagSet.String("app", "", "App ID")
	buildsList.FlagSet.Int("limit", 0, "Max results")
	buildsList.FlagSet.Bool("paginate", false, "Fetch all pages")

	buildsExpire := &ffcli.Command{
		Name:      "expire",
		ShortHelp: "Expire a build.",
		FlagSet:   flag.NewFlagSet("expire", flag.ContinueOnError),
		Exec:      func(context.Context, []string) error { return nil },
	}

	builds := &ffcli.Command{
		Name:        "builds",
		ShortHelp:   "Manage builds.",
		Subcommands: []*ffcli.Command{buildsList, buildsExpire},
		Exec:        func(context.Context, []string) error { return nil },
	}

	ageRatingSet := &ffcli.Command{
		Name:      "set",
		ShortHelp: "Set the age rating.",
		Exec:      func(context.Context, []string) error { return nil },
	}
	ageRating := &ffcli.Command{
		Name:        "age-rating",
		ShortHelp:   "Manage age ratings.",
		Subcommands: []*ffcli.Command{ageRatingSet},
	}

	deprecated := &ffcli.Command{
		Name:      "old",
		ShortHelp: "DEPRECATED: use builds list.",
		Exec:      func(context.Context, []string) error { return nil },
	}

	// A group whose Exec is nil-leaf placeholder should be skipped.
	emptyGroupLeaf := &ffcli.Command{
		Name:      "placeholder",
		ShortHelp: "Group placeholder.",
	}

	return []*ffcli.Command{builds, ageRating, deprecated, emptyGroupLeaf}
}

func TestBuildToolsEnumeratesLeaves(t *testing.T) {
	tools := BuildTools(sampleTree())

	got := map[string]Tool{}
	for _, tl := range tools {
		got[tl.Name] = tl
	}

	if _, ok := got["builds_list"]; !ok {
		t.Fatalf("expected builds_list tool, got %v", names(tools))
	}
	if _, ok := got["builds_expire"]; !ok {
		t.Fatalf("expected builds_expire tool, got %v", names(tools))
	}
	if _, ok := got["age_rating_set"]; !ok {
		t.Fatalf("expected age_rating_set tool (hyphen normalized), got %v", names(tools))
	}
	if _, ok := got["old"]; ok {
		t.Fatalf("deprecated command should be hidden, got %v", names(tools))
	}
	if _, ok := got["placeholder"]; ok {
		t.Fatalf("leaf without Exec should be skipped, got %v", names(tools))
	}

	if got["builds_list"].Service != "builds" {
		t.Fatalf("expected service builds, got %q", got["builds_list"].Service)
	}
}

func TestBuildToolsSortedByName(t *testing.T) {
	tools := BuildTools(sampleTree())
	for i := 1; i < len(tools); i++ {
		if tools[i-1].Name > tools[i].Name {
			t.Fatalf("tools not sorted: %q before %q", tools[i-1].Name, tools[i].Name)
		}
	}
}

func TestClassifyRisk(t *testing.T) {
	cases := []struct {
		path []string
		want Risk
	}{
		{[]string{"builds", "list"}, RiskRead},
		{[]string{"apps", "get"}, RiskRead},
		{[]string{"builds", "expire"}, RiskWrite},
		{[]string{"age-rating", "set"}, RiskWrite},
		{[]string{"profiles", "create"}, RiskWrite},
		{[]string{"something", "unknownverb"}, RiskWrite}, // safe default
		{[]string{"builds", "download", "dsyms"}, RiskRead},
	}
	for _, c := range cases {
		if got := classifyRisk(c.path); got != c.want {
			t.Errorf("classifyRisk(%v) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestToolName(t *testing.T) {
	if got := toolName([]string{"age-rating", "set"}); got != "age_rating_set" {
		t.Fatalf("toolName = %q", got)
	}
	if got := toolName([]string{"builds", "list"}); got != "builds_list" {
		t.Fatalf("toolName = %q", got)
	}
}

func TestInputSchemaFromFlags(t *testing.T) {
	tools := BuildTools(sampleTree())
	var list Tool
	for _, tl := range tools {
		if tl.Name == "builds_list" {
			list = tl
		}
	}
	schema := list.InputSchema()
	if schema.Type != "object" {
		t.Fatalf("schema type = %q", schema.Type)
	}
	if schema.Properties["paginate"].Type != "boolean" {
		t.Fatalf("paginate should be boolean, got %q", schema.Properties["paginate"].Type)
	}
	if schema.Properties["app"].Type != "string" {
		t.Fatalf("app should be string, got %q", schema.Properties["app"].Type)
	}
	if _, ok := schema.Properties["args"]; !ok {
		t.Fatalf("schema should always include positional args property")
	}
}

func TestBoolFlagNames(t *testing.T) {
	tools := BuildTools(sampleTree())
	var list Tool
	for _, tl := range tools {
		if tl.Name == "builds_list" {
			list = tl
		}
	}
	bf := list.boolFlagNames()
	if _, ok := bf["paginate"]; !ok {
		t.Fatalf("expected paginate in bool flags, got %v", bf)
	}
	if _, ok := bf["app"]; ok {
		t.Fatalf("app should not be a bool flag")
	}
}

func TestBuildArgv(t *testing.T) {
	tools := BuildTools(sampleTree())
	var list Tool
	for _, tl := range tools {
		if tl.Name == "builds_list" {
			list = tl
		}
	}

	argv, err := buildArgv(list, map[string]any{
		"app":      "123",
		"limit":    float64(5),
		"paginate": true,
		"args":     []any{"extra"},
	})
	if err != nil {
		t.Fatalf("buildArgv: %v", err)
	}
	want := []string{"builds", "list", "--app", "123", "--limit", "5", "--paginate", "extra"}
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("argv = %v, want %v", argv, want)
	}
}

func TestBuildArgvFalseBoolOmitted(t *testing.T) {
	tools := BuildTools(sampleTree())
	var list Tool
	for _, tl := range tools {
		if tl.Name == "builds_list" {
			list = tl
		}
	}
	argv, err := buildArgv(list, map[string]any{"paginate": false})
	if err != nil {
		t.Fatalf("buildArgv: %v", err)
	}
	want := []string{"builds", "list"}
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("argv = %v, want %v", argv, want)
	}
}

func names(tools []Tool) []string {
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		out = append(out, t.Name)
	}
	return out
}
