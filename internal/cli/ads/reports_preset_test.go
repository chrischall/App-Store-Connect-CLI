package ads

import (
	"strings"
	"testing"
	"time"
)

func TestReportPresetDateRangeLastDays(t *testing.T) {
	start, end, err := reportPresetDateRange("", "", 7, time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("reportPresetDateRange() error: %v", err)
	}
	if start != "2026-05-25" || end != "2026-05-31" {
		t.Fatalf("range = %s..%s, want 2026-05-25..2026-05-31", start, end)
	}
}

func TestReportPresetDateRangeValidation(t *testing.T) {
	tests := []struct {
		name    string
		from    string
		to      string
		days    int
		wantErr string
	}{
		{name: "last days conflicts with explicit range", from: "2026-05-01", days: 7, wantErr: "--last-days cannot be combined"},
		{name: "negative last days", days: -1, wantErr: "--last-days must be >= 0"},
		{name: "missing range", wantErr: "either --last-days or both --from and --to are required"},
		{name: "bad from", from: "2026/05/01", to: "2026-05-31", wantErr: "--from must be in YYYY-MM-DD format"},
		{name: "reversed range", from: "2026-06-01", to: "2026-05-31", wantErr: "--to must be on or after --from"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := reportPresetDateRange(tt.from, tt.to, tt.days, time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want contains %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseReportPresetSort(t *testing.T) {
	sortSpec, err := parseReportPresetSort("impressions:asc")
	if err != nil {
		t.Fatalf("parseReportPresetSort() error: %v", err)
	}
	if sortSpec.Field != "impressions" || sortSpec.SortOrder != "ASCENDING" {
		t.Fatalf("sort = %+v, want impressions ASCENDING", sortSpec)
	}

	_, err = parseReportPresetSort("impressions:sideways")
	if err == nil || !strings.Contains(err.Error(), "--sort direction must be asc or desc") {
		t.Fatalf("error = %v, want sort direction validation", err)
	}
}
