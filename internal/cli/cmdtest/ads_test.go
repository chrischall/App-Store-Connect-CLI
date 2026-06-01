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

func TestAdsCampaignPauseAndResumeUseCuratedStatusPayloads(t *testing.T) {
	t.Setenv("ASC_ADS_ACCESS_TOKEN", "ACCESS")
	t.Setenv("ASC_ADS_ORG_ID", "123456")
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.json"))

	log := newRequestLog(2)
	installDefaultTransport(t, adsRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPut || req.URL.Path != "/api/v5/campaigns/123" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
		}
		if got := req.Header.Get("X-AP-Context"); got != "orgId=123456" {
			t.Fatalf("X-AP-Context = %q, want orgId=123456", got)
		}
		var body struct {
			Campaign struct {
				Status string `json:"status"`
			} `json:"campaign"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		status := body.Campaign.Status
		log.Add(status)
		return adsJSONResponse(200, `{"data":{"id":123,"status":"`+status+`"}}`), nil
	}))

	for _, args := range [][]string{
		{"ads", "campaigns", "pause", "--campaign", "123", "--confirm", "--output", "json"},
		{"ads", "campaigns", "resume", "--campaign", "123", "--confirm", "--output", "json"},
	} {
		root := RootCommand("dev")
		if err := root.Parse(args); err != nil {
			t.Fatalf("parse %s: %v", strings.Join(args, " "), err)
		}
		stdout, stderr := captureOutput(t, func() {
			if err := root.Run(context.Background()); err != nil {
				t.Fatalf("run %s: %v", strings.Join(args, " "), err)
			}
		})
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		var parsed struct {
			Data struct {
				ID     int    `json:"id"`
				Status string `json:"status"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
			t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
		}
		if parsed.Data.ID != 123 || parsed.Data.Status == "" {
			t.Fatalf("parsed data = %+v, want campaign status response", parsed.Data)
		}
	}

	requests := strings.Join(log.Snapshot(), "\n")
	if requests != "PAUSED\nENABLED" {
		t.Fatalf("payload statuses = %q, want PAUSED then ENABLED", requests)
	}
}

func TestAdsCampaignPauseHonorsParentFlagsBeforeWorkflowSubcommand(t *testing.T) {
	t.Setenv("ASC_ADS_ACCESS_TOKEN", "ACCESS")
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.json"))

	installDefaultTransport(t, adsRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPut || req.URL.Path != "/api/v5/campaigns/123" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
		}
		if got := req.Header.Get("X-AP-Context"); got != "orgId=123456" {
			t.Fatalf("X-AP-Context = %q, want parent --org value", got)
		}
		return adsJSONResponse(200, `{"data":{"id":123,"status":"PAUSED"}}`), nil
	}))

	root := RootCommand("dev")
	if err := root.Parse([]string{"ads", "campaigns", "--org", "123456", "pause", "--campaign", "123", "--confirm"}); err != nil {
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
	if !strings.Contains(stdout, `"status":"PAUSED"`) {
		t.Fatalf("stdout = %q, want paused response", stdout)
	}
}

func TestAdsCampaignPauseValidatesBeforeNetwork(t *testing.T) {
	t.Setenv("ASC_ADS_ACCESS_TOKEN", "ACCESS")
	t.Setenv("ASC_ADS_ORG_ID", "123456")
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.json"))
	installDefaultTransport(t, adsRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected network request: %s %s", req.Method, req.URL.String())
		return nil, nil
	}))

	for _, tc := range []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing confirm",
			args:    []string{"ads", "campaigns", "pause", "--campaign", "123"},
			wantErr: "--confirm is required",
		},
		{
			name:    "invalid campaign",
			args:    []string{"ads", "campaigns", "pause", "--campaign", "abc", "--confirm"},
			wantErr: "--campaign must be an integer",
		},
		{
			name:    "missing campaign",
			args:    []string{"ads", "campaigns", "pause", "--confirm"},
			wantErr: "--campaign is required",
		},
		{
			name:    "parent output conflicts with child pretty",
			args:    []string{"ads", "campaigns", "--output", "table", "pause", "--campaign", "123", "--confirm", "--pretty"},
			wantErr: "--pretty is only valid with JSON output",
		},
		{
			name:    "parent pretty conflicts with child output",
			args:    []string{"ads", "campaigns", "--pretty", "resume", "--campaign", "123", "--confirm", "--output", "table"},
			wantErr: "--pretty is only valid with JSON output",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := RootCommand("dev")
			if err := root.Parse(tc.args); err != nil {
				t.Fatalf("parse error: %v", err)
			}
			var runErr error
			_, stderr := captureOutput(t, func() {
				runErr = root.Run(context.Background())
			})
			if !errors.Is(runErr, flag.ErrHelp) || !strings.Contains(stderr, tc.wantErr) {
				t.Fatalf("run error = %v stderr = %q, want %q", runErr, stderr, tc.wantErr)
			}
		})
	}
}

func TestAdsCampaignResumeReportsCommandNameOnAuthFailure(t *testing.T) {
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.json"))
	installDefaultTransport(t, adsRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected network request: %s %s", req.Method, req.URL.String())
		return nil, nil
	}))

	root := RootCommand("dev")
	if err := root.Parse([]string{"ads", "campaigns", "resume", "--campaign", "123", "--confirm", "--output", "json"}); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	var runErr error
	_, stderr := captureOutput(t, func() {
		runErr = root.Run(context.Background())
	})
	if runErr == nil || !strings.Contains(runErr.Error(), "ads campaigns resume:") {
		t.Fatalf("run error = %v, want resume command name", runErr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
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
