package mcp

import "sort"

// Suites are named, cross-cutting groups of services selected with the
// --tool-suite flag (e.g. --tool-suite developer). A suite expands to a set of
// services; a tool matches the suite when its service is in that set.
//
// Because suites live on their own flag, suite names never collide with the
// --allow-tool service namespace: "analytics" under --allow-tool always means
// the analytics service, while "analytics" under --tool-suite means the suite.
//
// Service tokens are stored normalized (hyphens as underscores) to match the
// normalized form used by matchSuite.
var suites = map[string][]string{
	"developer": {
		"builds", "build_bundles", "build_localizations", "testflight",
		"xcode", "xcode_cloud", "profiles", "certificates", "bundle_ids",
		"devices", "capabilities", "signing", "notarization", "encryption",
		"background_assets", "app_clips", "android_ios_mapping", "workflow",
	},
	"release": {
		"apps", "app_setup", "app_tags", "versions", "submit", "publish",
		"release", "release_notes", "review", "reviews", "status", "validate",
		"localizations", "metadata", "screenshots", "video_previews",
		"product_pages", "categories", "age_rating", "accessibility",
		"nominations", "routing_coverage", "eula", "app_events",
	},
	"billing": {
		"finance", "pricing", "agreements", "subscriptions", "iap",
		"storekit", "merchant_ids", "sandbox",
	},
	"analytics": {
		"analytics", "insights", "performance", "ads",
	},
	"admin": {
		"users", "actors", "account", "auth", "webhooks", "pass_type_ids",
		"telemetry", "migrate", "notify",
	},
	"distribution": {
		"marketplace", "alternative_distribution", "pre_orders",
	},
	"gamecenter": {
		"game_center",
	},
}

// IsSuite reports whether name is a known suite (after normalization).
func IsSuite(name string) bool {
	_, ok := suites[normalizeSelector(name)]
	return ok
}

// suiteServices returns the normalized service set for a suite, or nil if the
// name is not a suite.
func suiteServices(name string) map[string]struct{} {
	services, ok := suites[name]
	if !ok {
		return nil
	}
	set := make(map[string]struct{}, len(services))
	for _, s := range services {
		set[s] = struct{}{}
	}
	return set
}

// SuiteNames returns the suite names in sorted order (used for listing/help).
func SuiteNames() []string {
	names := make([]string, 0, len(suites))
	for name := range suites {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// SuiteServices returns the sorted service list for a suite (used for listing).
func SuiteServices(name string) []string {
	services, ok := suites[name]
	if !ok {
		return nil
	}
	out := append([]string{}, services...)
	sort.Strings(out)
	return out
}
