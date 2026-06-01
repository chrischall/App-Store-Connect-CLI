package cmdtest

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

type adsRoundTripFunc func(*http.Request) (*http.Response, error)

func (f adsRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestAdsCampaignsAliasPaginatesWithOrgContext(t *testing.T) {
	t.Setenv("ASC_ADS_ACCESS_TOKEN", "ACCESS")
	t.Setenv("ASC_ADS_ORG_ID", "123456")
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.json"))

	log := newRequestLog(2)
	installDefaultTransport(t, adsRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "api.searchads.apple.com" {
			t.Fatalf("unexpected host %s", req.URL.Host)
		}
		if got := req.Header.Get("Authorization"); got != "Bearer ACCESS" {
			t.Fatalf("Authorization = %q, want Bearer ACCESS", got)
		}
		if got := req.Header.Get("X-AP-Context"); got != "orgId=123456" {
			t.Fatalf("X-AP-Context = %q, want orgId=123456", got)
		}
		log.Add(req.URL.Path + "?" + req.URL.RawQuery)
		switch req.URL.Query().Get("offset") {
		case "0":
			return adsJSONResponse(200, `{"data":[{"id":1},{"id":2}],"pagination":{"itemsPerPage":2,"startIndex":0,"totalResults":3}}`), nil
		case "2":
			return adsJSONResponse(200, `{"data":[{"id":3}],"pagination":{"itemsPerPage":2,"startIndex":2,"totalResults":3}}`), nil
		default:
			t.Fatalf("unexpected offset %q", req.URL.Query().Get("offset"))
			return nil, nil
		}
	}))

	root := RootCommand("dev")
	stdout, stderr := captureOutput(t, func() {
		if err := root.Parse([]string{"ads", "campaigns", "--limit", "2", "--paginate", "--output", "json"}); err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if err := root.Run(context.Background()); err != nil {
			t.Fatalf("run error: %v", err)
		}
	})
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	var parsed struct {
		Data []map[string]int `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	if len(parsed.Data) != 3 || parsed.Data[2]["id"] != 3 {
		t.Fatalf("data = %+v, want three aggregated campaign rows", parsed.Data)
	}
	requests := strings.Join(log.Snapshot(), "\n")
	if !strings.Contains(requests, "/api/v5/campaigns?limit=2&offset=0") || !strings.Contains(requests, "/api/v5/campaigns?limit=2&offset=2") {
		t.Fatalf("requests = %q, want both paginated offsets", requests)
	}
}

func TestAdsReportsPresetBuildsCampaignRequest(t *testing.T) {
	t.Setenv("ASC_ADS_ACCESS_TOKEN", "ACCESS")
	t.Setenv("ASC_ADS_ORG_ID", "123456")
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.json"))

	installDefaultTransport(t, adsRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.Path != "/api/v5/reports/campaigns" {
			t.Fatalf("request = %s %s, want POST /api/v5/reports/campaigns", req.Method, req.URL.String())
		}
		if got := req.Header.Get("X-AP-Context"); got != "orgId=123456" {
			t.Fatalf("X-AP-Context = %q, want orgId=123456", got)
		}
		var body struct {
			StartTime       string `json:"startTime"`
			EndTime         string `json:"endTime"`
			Granularity     string `json:"granularity"`
			ReturnRowTotals bool   `json:"returnRowTotals"`
			TimeZone        string `json:"timeZone"`
			Selector        struct {
				Fields  []string `json:"fields"`
				OrderBy []struct {
					Field     string `json:"field"`
					SortOrder string `json:"sortOrder"`
				} `json:"orderBy"`
				Pagination struct {
					Offset int `json:"offset"`
					Limit  int `json:"limit"`
				} `json:"pagination"`
			} `json:"selector"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.StartTime != "2026-05-01" || body.EndTime != "2026-05-31" {
			t.Fatalf("date range = %s..%s, want May 2026", body.StartTime, body.EndTime)
		}
		if body.Granularity != "HOURLY" || body.TimeZone != "UTC" || !body.ReturnRowTotals {
			t.Fatalf("report options = %+v, want hourly UTC totals", body)
		}
		if strings.Join(body.Selector.Fields, ",") != "campaignName,impressions,taps,spend" {
			t.Fatalf("fields = %v", body.Selector.Fields)
		}
		if len(body.Selector.OrderBy) != 1 || body.Selector.OrderBy[0].Field != "impressions" || body.Selector.OrderBy[0].SortOrder != "DESCENDING" {
			t.Fatalf("orderBy = %+v, want impressions descending", body.Selector.OrderBy)
		}
		if body.Selector.Pagination.Offset != 5 || body.Selector.Pagination.Limit != 25 {
			t.Fatalf("pagination = %+v, want offset 5 limit 25", body.Selector.Pagination)
		}
		return adsJSONResponse(200, `{"data":{"reportingDataResponse":{"row":[{"metadata":{"campaignId":12345},"total":{"impressions":42}}]}}}`), nil
	}))

	root := RootCommand("dev")
	args := []string{
		"ads", "reports", "preset",
		"--level", "campaigns",
		"--from", "2026-05-01",
		"--to", "2026-05-31",
		"--fields", "campaignName,impressions,taps,spend",
		"--granularity", "hourly",
		"--sort", "impressions:desc",
		"--limit", "25",
		"--offset", "5",
		"--return-row-totals",
		"--output", "json",
	}
	if err := root.Parse(args); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	stdout, stderr := captureOutput(t, func() {
		if err := root.Run(context.Background()); err != nil {
			t.Fatalf("run error: %v", err)
		}
	})
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
}

func TestAdsReportsPresetBuildsScopedKeywordRequest(t *testing.T) {
	t.Setenv("ASC_ADS_ACCESS_TOKEN", "ACCESS")
	t.Setenv("ASC_ADS_ORG_ID", "")
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.json"))

	installDefaultTransport(t, adsRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.Path != "/api/v5/reports/campaigns/12345/keywords" {
			t.Fatalf("request = %s %s, want keyword report path", req.Method, req.URL.String())
		}
		if got := req.Header.Get("X-AP-Context"); got != "orgId=987654" {
			t.Fatalf("X-AP-Context = %q, want explicit org", got)
		}
		var body map[string]any
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["startTime"] != "2026-05-25" || body["endTime"] != "2026-05-31" {
			t.Fatalf("date range = %#v..%#v, want last-days payload", body["startTime"], body["endTime"])
		}
		return adsJSONResponse(200, `{"data":{"reportingDataResponse":{"row":[]}}}`), nil
	}))

	root := RootCommand("dev")
	args := []string{
		"ads", "reports", "preset",
		"--level", "keywords",
		"--campaign", "12345",
		"--from", "2026-05-25",
		"--to", "2026-05-31",
		"--org", "987654",
		"--output", "json",
	}
	if err := root.Parse(args); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	stdout, stderr := captureOutput(t, func() {
		if err := root.Run(context.Background()); err != nil {
			t.Fatalf("run error: %v", err)
		}
	})
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
}

func TestAdsReportsPresetBuildsAdLevelRequestWithSort(t *testing.T) {
	t.Setenv("ASC_ADS_ACCESS_TOKEN", "ACCESS")
	t.Setenv("ASC_ADS_ORG_ID", "123456")
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.json"))

	installDefaultTransport(t, adsRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.Path != "/api/v5/reports/campaigns/12345/ads" {
			t.Fatalf("request = %s %s, want ad report path", req.Method, req.URL.String())
		}
		var body struct {
			Selector struct {
				OrderBy []struct {
					Field     string `json:"field"`
					SortOrder string `json:"sortOrder"`
				} `json:"orderBy"`
			} `json:"selector"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if len(body.Selector.OrderBy) != 1 || body.Selector.OrderBy[0].Field != "impressions" || body.Selector.OrderBy[0].SortOrder != "DESCENDING" {
			t.Fatalf("orderBy = %+v, want impressions descending", body.Selector.OrderBy)
		}
		return adsJSONResponse(200, `{"data":{"reportingDataResponse":{"row":[]}}}`), nil
	}))

	root := RootCommand("dev")
	args := []string{
		"ads", "reports", "preset",
		"--level", "ads",
		"--campaign", "12345",
		"--from", "2026-05-01",
		"--to", "2026-05-31",
		"--sort", "impressions:desc",
		"--output", "json",
	}
	if err := root.Parse(args); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	stdout, stderr := captureOutput(t, func() {
		if err := root.Run(context.Background()); err != nil {
			t.Fatalf("run error: %v", err)
		}
	})
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
}

func TestAdsReportsPresetValidatesUsageBeforeNetwork(t *testing.T) {
	t.Setenv("ASC_ADS_ACCESS_TOKEN", "ACCESS")
	t.Setenv("ASC_ADS_ORG_ID", "123456")
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.json"))
	installDefaultTransport(t, adsRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected network request: %s %s", req.Method, req.URL.String())
		return nil, nil
	}))

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing date range",
			args:    []string{"ads", "reports", "preset", "--level", "campaigns", "--output", "json"},
			wantErr: "either --last-days or both --from and --to are required",
		},
		{
			name:    "invalid level",
			args:    []string{"ads", "reports", "preset", "--level", "unsupported", "--from", "2026-05-01", "--to", "2026-05-31", "--output", "json"},
			wantErr: "--level must be one of:",
		},
		{
			name:    "campaign required",
			args:    []string{"ads", "reports", "preset", "--level", "keywords", "--from", "2026-05-01", "--to", "2026-05-31", "--output", "json"},
			wantErr: "--campaign is required for --level keywords",
		},
		{
			name:    "campaign nonnegative",
			args:    []string{"ads", "reports", "preset", "--level", "keywords", "--campaign", "-1", "--from", "2026-05-01", "--to", "2026-05-31", "--output", "json"},
			wantErr: "--campaign must be >= 0",
		},
		{
			name:    "invalid sort direction",
			args:    []string{"ads", "reports", "preset", "--level", "campaigns", "--from", "2026-05-01", "--to", "2026-05-31", "--sort", "impressions:sideways", "--output", "json"},
			wantErr: "--sort direction must be asc or desc",
		},
		{
			name:    "invalid granularity",
			args:    []string{"ads", "reports", "preset", "--level", "campaigns", "--from", "2026-05-01", "--to", "2026-05-31", "--granularity", "YEARLY", "--output", "json"},
			wantErr: "--granularity must be one of: HOURLY, DAILY, WEEKLY, MONTHLY",
		},
		{
			name:    "hourly unsupported for search terms",
			args:    []string{"ads", "reports", "preset", "--level", "search-terms", "--campaign", "12345", "--from", "2026-05-01", "--to", "2026-05-31", "--time-zone", "ORTZ", "--granularity", "HOURLY", "--output", "json"},
			wantErr: "--granularity HOURLY is only supported",
		},
		{
			name:    "hourly unsupported for ads",
			args:    []string{"ads", "reports", "preset", "--level", "ads", "--campaign", "12345", "--from", "2026-05-01", "--to", "2026-05-31", "--granularity", "HOURLY", "--sort", "impressions:desc", "--output", "json"},
			wantErr: "--granularity HOURLY is only supported",
		},
		{
			name:    "row totals unsupported for search terms",
			args:    []string{"ads", "reports", "preset", "--level", "search-terms", "--campaign", "12345", "--from", "2026-05-01", "--to", "2026-05-31", "--time-zone", "ORTZ", "--return-row-totals", "--output", "json"},
			wantErr: "--return-row-totals cannot be used with search-term report levels",
		},
		{
			name:    "invalid time zone",
			args:    []string{"ads", "reports", "preset", "--level", "campaigns", "--last-days", "1", "--time-zone", "America/Los_Angeles", "--output", "json"},
			wantErr: "--time-zone must be UTC or ORTZ",
		},
		{
			name:    "last days require UTC",
			args:    []string{"ads", "reports", "preset", "--level", "campaigns", "--last-days", "1", "--time-zone", "ORTZ", "--output", "json"},
			wantErr: "--last-days requires --time-zone UTC",
		},
		{
			name:    "search terms require ORTZ",
			args:    []string{"ads", "reports", "preset", "--level", "search-terms", "--campaign", "12345", "--from", "2026-05-01", "--to", "2026-05-31", "--time-zone", "UTC", "--output", "json"},
			wantErr: "--time-zone must be ORTZ for search-term report levels",
		},
		{
			name:    "ad level requires sort",
			args:    []string{"ads", "reports", "preset", "--level", "ads", "--campaign", "12345", "--from", "2026-05-01", "--to", "2026-05-31", "--output", "json"},
			wantErr: "--sort is required for --level ads",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := RootCommand("dev")
			if err := root.Parse(tt.args); err != nil {
				t.Fatalf("parse error: %v", err)
			}
			var runErr error
			_, stderr := captureOutput(t, func() {
				runErr = root.Run(context.Background())
			})
			if !errors.Is(runErr, flag.ErrHelp) || !strings.Contains(stderr, tt.wantErr) {
				t.Fatalf("run error = %v stderr = %q, want %q", runErr, stderr, tt.wantErr)
			}
		})
	}
}

func TestAdsImpressionShareReportsLimitValidation(t *testing.T) {
	t.Setenv("ASC_ADS_ACCESS_TOKEN", "ACCESS")
	t.Setenv("ASC_ADS_ORG_ID", "123456")
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.json"))

	root := RootCommand("dev")
	if err := root.Parse([]string{"ads", "impression-share-reports", "--limit", "51", "--output", "json"}); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	var runErr error
	_, stderr := captureOutput(t, func() {
		runErr = root.Run(context.Background())
	})
	if !errors.Is(runErr, flag.ErrHelp) || !strings.Contains(stderr, "--limit must be between 1 and 50") {
		t.Fatalf("run error = %v stderr = %q, want custom reports limit validation", runErr, stderr)
	}
}

func TestAdsLimitZeroValidation(t *testing.T) {
	t.Setenv("ASC_ADS_ACCESS_TOKEN", "ACCESS")
	t.Setenv("ASC_ADS_ORG_ID", "123456")
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.json"))
	installDefaultTransport(t, adsRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected network request: %s %s", req.Method, req.URL.String())
		return nil, nil
	}))

	root := RootCommand("dev")
	if err := root.Parse([]string{"ads", "campaigns", "--limit", "0", "--output", "json"}); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	var runErr error
	_, stderr := captureOutput(t, func() {
		runErr = root.Run(context.Background())
	})
	if !errors.Is(runErr, flag.ErrHelp) || !strings.Contains(stderr, "--limit must be between 1 and 1000") {
		t.Fatalf("run error = %v stderr = %q, want zero limit validation", runErr, stderr)
	}
}

func TestAdsDeleteRequiresConfirmBeforeNetwork(t *testing.T) {
	t.Setenv("ASC_ADS_ACCESS_TOKEN", "ACCESS")
	t.Setenv("ASC_ADS_ORG_ID", "123456")
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.json"))
	installDefaultTransport(t, adsRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected network request: %s %s", req.Method, req.URL.String())
		return nil, nil
	}))

	root := RootCommand("dev")
	if err := root.Parse([]string{"ads", "campaigns", "delete", "--campaign", "123"}); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	var runErr error
	_, stderr := captureOutput(t, func() {
		runErr = root.Run(context.Background())
	})
	if !errors.Is(runErr, flag.ErrHelp) || !strings.Contains(stderr, "--confirm is required") {
		t.Fatalf("run error = %v stderr = %q, want confirm validation", runErr, stderr)
	}
}

func TestAdsEndpointRejectsUnexpectedArgsBeforeNetwork(t *testing.T) {
	t.Setenv("ASC_ADS_ACCESS_TOKEN", "ACCESS")
	t.Setenv("ASC_ADS_ORG_ID", "123456")
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.json"))
	installDefaultTransport(t, adsRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected network request: %s %s", req.Method, req.URL.String())
		return nil, nil
	}))

	root := RootCommand("dev")
	if err := root.Parse([]string{"ads", "campaigns", "--output", "json", "unexpected"}); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	var runErr error
	_, stderr := captureOutput(t, func() {
		runErr = root.Run(context.Background())
	})
	if !errors.Is(runErr, flag.ErrHelp) || !strings.Contains(stderr, "unexpected argument(s): unexpected") {
		t.Fatalf("run error = %v stderr = %q, want unexpected argument usage error", runErr, stderr)
	}
}

func TestAdsAPIRequestRejectsNonAppleURLsBeforeNetwork(t *testing.T) {
	t.Setenv("ASC_ADS_ACCESS_TOKEN", "ACCESS")
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.json"))
	installDefaultTransport(t, adsRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected network request: %s %s", req.Method, req.URL.String())
		return nil, nil
	}))

	root := RootCommand("dev")
	if err := root.Parse([]string{"ads", "api", "request", "--path", "https://example.com/api/v5/campaigns"}); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	var runErr error
	_, stderr := captureOutput(t, func() {
		runErr = root.Run(context.Background())
	})
	if !errors.Is(runErr, flag.ErrHelp) || !strings.Contains(stderr, "Apple Ads v5 URL") {
		t.Fatalf("run error = %v stderr = %q, want Apple host guardrail", runErr, stderr)
	}
}

func TestAdsAPIRequestRejectsUnexpectedArgsBeforeNetwork(t *testing.T) {
	t.Setenv("ASC_ADS_ACCESS_TOKEN", "ACCESS")
	t.Setenv("ASC_ADS_ORG_ID", "123456")
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.json"))
	installDefaultTransport(t, adsRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected network request: %s %s", req.Method, req.URL.String())
		return nil, nil
	}))

	root := RootCommand("dev")
	if err := root.Parse([]string{"ads", "api", "request", "--path", "v5/campaigns", "--output", "json", "unexpected"}); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	var runErr error
	_, stderr := captureOutput(t, func() {
		runErr = root.Run(context.Background())
	})
	if !errors.Is(runErr, flag.ErrHelp) || !strings.Contains(stderr, "unexpected argument(s): unexpected") {
		t.Fatalf("run error = %v stderr = %q, want unexpected argument usage error", runErr, stderr)
	}
}

func adsJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
