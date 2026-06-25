package mcp

import "testing"

func suiteSampleTools() []Tool {
	return []Tool{
		{Name: "builds_list", Service: "builds", Risk: RiskRead},
		{Name: "testflight_add", Service: "testflight", Risk: RiskWrite},
		{Name: "finance_report", Service: "finance", Risk: RiskRead},
		{Name: "analytics_get", Service: "analytics", Risk: RiskRead},
		{Name: "ads_list", Service: "ads", Risk: RiskRead},
		{Name: "game_center_list", Service: "game-center", Risk: RiskRead},
		{Name: "marketplace_get", Service: "marketplace", Risk: RiskRead},
		{Name: "users_list", Service: "users", Risk: RiskRead},
	}
}

func TestSuiteExpandsToServices(t *testing.T) {
	got := Selection{Suites: []string{"developer"}, AllowWrite: true}.Filter(suiteSampleTools())
	gotNames := map[string]bool{}
	for _, tl := range got {
		gotNames[tl.Name] = true
	}
	if !gotNames["builds_list"] || !gotNames["testflight_add"] {
		t.Fatalf("developer suite should include builds + testflight, got %v", names(got))
	}
	if gotNames["finance_report"] || gotNames["users_list"] {
		t.Fatalf("developer suite leaked unrelated services, got %v", names(got))
	}
}

func TestSuiteRespectsAllowWrite(t *testing.T) {
	// testflight_add is a write tool; without allow-write the developer suite
	// must not expose it.
	got := Selection{Suites: []string{"developer"}}.Filter(suiteSampleTools())
	for _, tl := range got {
		if tl.Risk == RiskWrite {
			t.Fatalf("suite exposed write tool without --allow-write: %q", tl.Name)
		}
	}
}

func TestSuiteAndSelectorAreAdditive(t *testing.T) {
	// --tool-suite billing plus --allow-tool users_list exposes both.
	got := Selection{
		Suites:    []string{"billing"},
		Selectors: []string{"users_list"},
	}.Filter(suiteSampleTools())
	gotNames := map[string]bool{}
	for _, tl := range got {
		gotNames[tl.Name] = true
	}
	if !gotNames["finance_report"] || !gotNames["users_list"] {
		t.Fatalf("expected billing suite + users_list, got %v", names(got))
	}
}

func TestSuiteAndServiceNameDoNotCollide(t *testing.T) {
	// "analytics" under --allow-tool is the service only (not the suite),
	// so the ads tool must NOT be included.
	svc := Selection{Selectors: []string{"analytics"}}.Filter(suiteSampleTools())
	for _, tl := range svc {
		if tl.Service != "analytics" {
			t.Fatalf("--allow-tool analytics should match only the service, got %q", tl.Name)
		}
	}
	if len(svc) == 0 {
		t.Fatalf("--allow-tool analytics matched nothing")
	}

	// "analytics" under --tool-suite is the suite (includes ads).
	suite := Selection{Suites: []string{"analytics"}}.Filter(suiteSampleTools())
	services := map[string]bool{}
	for _, tl := range suite {
		services[tl.Service] = true
	}
	if !services["analytics"] || !services["ads"] {
		t.Fatalf("--tool-suite analytics should be the suite (incl. ads), got %v", services)
	}
}

func TestUnknownSuiteMatchesNothing(t *testing.T) {
	got := Selection{Suites: []string{"does-not-exist"}}.Filter(suiteSampleTools())
	if len(got) != 0 {
		t.Fatalf("unknown suite should match nothing, got %v", names(got))
	}
}

func TestGamecenterSuite(t *testing.T) {
	got := Selection{Suites: []string{"gamecenter"}}.Filter(suiteSampleTools())
	if len(got) != 1 || got[0].Service != "game-center" {
		t.Fatalf("gamecenter suite should match game-center service, got %v", names(got))
	}
}

func TestIsSuite(t *testing.T) {
	if !IsSuite("developer") || !IsSuite("gamecenter") {
		t.Fatal("expected known suites to be recognized")
	}
	if IsSuite("builds") || IsSuite("nope") {
		t.Fatal("non-suite names should not be recognized as suites")
	}
}

func TestSuiteNamesAndServicesStable(t *testing.T) {
	names := SuiteNames()
	if len(names) == 0 {
		t.Fatal("expected suites")
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Fatalf("SuiteNames not sorted: %v", names)
		}
	}
	if got := SuiteServices("does-not-exist"); got != nil {
		t.Fatalf("unknown suite should return nil, got %v", got)
	}
	if got := SuiteServices("gamecenter"); len(got) != 1 || got[0] != "game_center" {
		t.Fatalf("gamecenter services = %v", got)
	}
}
