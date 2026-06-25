package mcp

import "testing"

func TestParseSelectors(t *testing.T) {
	got := ParseSelectors("read, builds_*  ,, apps.get\nwrite")
	want := []string{"read", "builds_*", "apps.get", "write"}
	if len(got) != len(want) {
		t.Fatalf("ParseSelectors = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ParseSelectors[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func sampleTools() []Tool {
	return []Tool{
		{Name: "builds_list", Service: "builds", Risk: RiskRead},
		{Name: "builds_expire", Service: "builds", Risk: RiskWrite},
		{Name: "apps_get", Service: "apps", Risk: RiskRead},
		{Name: "apps_delete", Service: "apps", Risk: RiskWrite},
		{Name: "age_rating_set", Service: "age-rating", Risk: RiskWrite},
	}
}

func TestSelectionDefaultIsReadOnly(t *testing.T) {
	sel := Selection{} // no selectors, no write
	got := sel.Filter(sampleTools())
	for _, tl := range got {
		if tl.Risk != RiskRead {
			t.Fatalf("default selection exposed write tool %q", tl.Name)
		}
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 read tools by default, got %v", names(got))
	}
}

func TestSelectionWriteRequiresAllowWrite(t *testing.T) {
	// Selector matches a write tool, but AllowWrite is false.
	sel := Selection{Selectors: []string{"builds_expire"}}
	if len(sel.Filter(sampleTools())) != 0 {
		t.Fatalf("write tool exposed without --allow-write")
	}

	sel.AllowWrite = true
	got := sel.Filter(sampleTools())
	if len(got) != 1 || got[0].Name != "builds_expire" {
		t.Fatalf("expected builds_expire with allow-write, got %v", names(got))
	}
}

func TestSelectionServiceSelector(t *testing.T) {
	sel := Selection{Selectors: []string{"builds"}, AllowWrite: true}
	got := sel.Filter(sampleTools())
	if len(got) != 2 {
		t.Fatalf("expected both builds tools, got %v", names(got))
	}
}

func TestSelectionGlobSelector(t *testing.T) {
	sel := Selection{Selectors: []string{"apps_*"}, AllowWrite: true}
	got := sel.Filter(sampleTools())
	if len(got) != 2 {
		t.Fatalf("expected apps_* tools, got %v", names(got))
	}
}

func TestSelectionDotSelectorEqualsUnderscore(t *testing.T) {
	sel := Selection{Selectors: []string{"apps.get"}}
	got := sel.Filter(sampleTools())
	if len(got) != 1 || got[0].Name != "apps_get" {
		t.Fatalf("dotted selector should match apps_get, got %v", names(got))
	}
}

func TestSelectionAllAndReadWriteClasses(t *testing.T) {
	all := Selection{Selectors: []string{"all"}, AllowWrite: true}.Filter(sampleTools())
	if len(all) != 5 {
		t.Fatalf("'all' should expose every tool, got %v", names(all))
	}

	reads := Selection{Selectors: []string{"read"}}.Filter(sampleTools())
	if len(reads) != 2 {
		t.Fatalf("'read' should expose read tools, got %v", names(reads))
	}

	writes := Selection{Selectors: []string{"write"}, AllowWrite: true}.Filter(sampleTools())
	if len(writes) != 3 {
		t.Fatalf("'write' should expose write tools, got %v", names(writes))
	}

	// 'write' class without allow-write still yields nothing.
	noWrite := Selection{Selectors: []string{"write"}}.Filter(sampleTools())
	if len(noWrite) != 0 {
		t.Fatalf("'write' without allow-write should be empty, got %v", names(noWrite))
	}
}
