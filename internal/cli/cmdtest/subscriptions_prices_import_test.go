package cmdtest

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSubscriptionsPricesImport_InvalidBooleanReturnsUsageError(t *testing.T) {
	setupAuth(t)

	originalTransport := http.DefaultTransport
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
	})
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected HTTP request: %s %s", req.Method, req.URL.Path)
		return nil, nil
	})

	csvPath := filepath.Join(t.TempDir(), "input.csv")
	csvBody := "" +
		"territory,price,preserved\n" +
		"USA,19.99,not-a-bool\n"
	if err := os.WriteFile(csvPath, []byte(csvBody), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	root := RootCommand("1.2.3")
	root.FlagSet.SetOutput(io.Discard)

	stdout, stderr := captureOutput(t, func() {
		if err := root.Parse([]string{"subscriptions", "pricing", "prices", "import", "--subscription-id", "8000000001", "--input", csvPath, "--dry-run"}); err != nil {
			t.Fatalf("parse error: %v", err)
		}
		err := root.Run(context.Background())
		if !errors.Is(err, flag.ErrHelp) {
			t.Fatalf("expected ErrHelp, got %v", err)
		}
	})

	if stdout != "" {
		t.Fatalf("expected empty stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "must be true or false") {
		t.Fatalf("expected boolean validation error, got %q", stderr)
	}
}

func TestSubscriptionsPricesImport_DryRunResolvesASCExportAliasWithoutMutations(t *testing.T) {
	setupAuth(t)

	originalTransport := http.DefaultTransport
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
	})

	seenTerritories := map[string]int{}
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("expected only GET in dry-run, got %s %s", req.Method, req.URL.Path)
		}
		if req.URL.Path != "/v1/subscriptions/8000000001/pricePoints" {
			t.Fatalf("unexpected path: %s", req.URL.Path)
		}
		territory := req.URL.Query().Get("filter[territory]")
		seenTerritories[territory]++

		switch territory {
		case "USA":
			body := `{"data":[{"type":"subscriptionPricePoints","id":"pp-usa","attributes":{"customerPrice":"19.99"}}],"links":{}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		case "AFG":
			body := `{"data":[{"type":"subscriptionPricePoints","id":"pp-afg","attributes":{"customerPrice":"299.00"}}],"links":{}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		default:
			t.Fatalf("unexpected filter[territory]=%q", territory)
			return nil, nil
		}
	})

	csvPath := filepath.Join(t.TempDir(), "input.csv")
	csvBody := "" +
		"Countries or Regions,Currency Code,Price,start_date,preserved\n" +
		"USA,USD,19.99,2026-03-01,false\n" +
		"Afghanistan,AFN,299.00,2026-03-01,true\n"
	if err := os.WriteFile(csvPath, []byte(csvBody), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	root := RootCommand("1.2.3")
	root.FlagSet.SetOutput(io.Discard)

	type importSummary struct {
		DryRun  bool `json:"dryRun"`
		Total   int  `json:"total"`
		Created int  `json:"created"`
		Failed  int  `json:"failed"`
	}

	stdout, stderr := captureOutput(t, func() {
		if err := root.Parse([]string{"subscriptions", "pricing", "prices", "import", "--subscription-id", "8000000001", "--input", csvPath, "--dry-run"}); err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if err := root.Run(context.Background()); err != nil {
			t.Fatalf("run error: %v", err)
		}
	})

	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	var summary importSummary
	if err := json.Unmarshal([]byte(stdout), &summary); err != nil {
		t.Fatalf("parse JSON summary: %v", err)
	}
	if !summary.DryRun {
		t.Fatalf("expected dryRun=true in summary")
	}
	if summary.Total != 2 || summary.Created != 2 || summary.Failed != 0 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if seenTerritories["USA"] == 0 || seenTerritories["AFG"] == 0 {
		t.Fatalf("expected lookups for USA and AFG, got %+v", seenTerritories)
	}
}

func TestSubscriptionsPricesImport_PartialFailureReturnsReportedErrorAndSummary(t *testing.T) {
	setupAuth(t)
	t.Chdir(t.TempDir())

	originalTransport := http.DefaultTransport
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
	})

	createCount := 0
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/v1/subscriptions/8000000001/pricePoints":
			if req.URL.Query().Get("filter[territory]") != "USA" {
				t.Fatalf("expected filter[territory]=USA, got %q", req.URL.Query().Get("filter[territory]"))
			}
			body := `{"data":[{"type":"subscriptionPricePoints","id":"pp-usa","attributes":{"customerPrice":"19.99"}}],"links":{}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		case req.Method == http.MethodGet && req.URL.Path == "/v1/subscriptions/8000000001/prices":
			return jsonHTTPResponse(http.StatusOK, `{"data":[],"links":{}}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/v1/subscriptionPrices":
			createCount++
			payload, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("ReadAll() error: %v", err)
			}
			if !strings.Contains(string(payload), `"id":"pp-usa"`) {
				t.Fatalf("expected resolved price point id in payload, got %s", string(payload))
			}
			body := `{"data":{"type":"subscriptionPrices","id":"price-1"}}`
			return &http.Response{
				StatusCode: http.StatusCreated,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
			return nil, nil
		}
	})

	csvPath := filepath.Join(t.TempDir(), "input.csv")
	csvBody := "" +
		"territory,price\n" +
		"USA,19.99\n" +
		"Atlantis,9.99\n"
	if err := os.WriteFile(csvPath, []byte(csvBody), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	root := RootCommand("1.2.3")
	root.FlagSet.SetOutput(io.Discard)

	type importFailure struct {
		Row int `json:"row"`
	}
	type importSummary struct {
		Total    int             `json:"total"`
		Created  int             `json:"created"`
		Failed   int             `json:"failed"`
		Failures []importFailure `json:"failures"`
	}

	var runErr error
	stdout, stderr := captureOutput(t, func() {
		if err := root.Parse([]string{"subscriptions", "pricing", "prices", "import", "--subscription-id", "8000000001", "--input", csvPath, "--output", "json"}); err != nil {
			t.Fatalf("parse error: %v", err)
		}
		runErr = root.Run(context.Background())
	})

	if runErr == nil {
		t.Fatalf("expected error")
	}
	if _, ok := errors.AsType[ReportedError](runErr); !ok {
		t.Fatalf("expected ReportedError, got %v", runErr)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	var summary importSummary
	if err := json.Unmarshal([]byte(stdout), &summary); err != nil {
		t.Fatalf("parse JSON summary: %v", err)
	}
	if summary.Total != 2 || summary.Created != 1 || summary.Failed != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if len(summary.Failures) != 1 || summary.Failures[0].Row != 2 {
		t.Fatalf("expected one failure at row 2, got %+v", summary.Failures)
	}
	if createCount != 1 {
		t.Fatalf("expected one successful create, got %d", createCount)
	}
}

func TestSubscriptionsPricesImport_SkipsExactExistingPrice(t *testing.T) {
	setupAuth(t)

	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	postCount := 0
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/v1/subscriptions/8000000001/prices":
			body := `{"data":[{"type":"subscriptionPrices","id":"price-existing","attributes":{"startDate":"2026-08-01","preserved":true},"relationships":{"territory":{"data":{"type":"territories","id":"USA"}},"subscriptionPricePoint":{"data":{"type":"subscriptionPricePoints","id":"pp-usa"}}}}],"links":{}}`
			return jsonHTTPResponse(http.StatusOK, body), nil
		case req.Method == http.MethodPost:
			postCount++
			t.Fatalf("exact existing price must not be posted again")
			return nil, nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})

	csvPath := filepath.Join(t.TempDir(), "input.csv")
	csvBody := "territory,price,start_date,preserved,price_point_id\nUSA,19.99,2026-08-01,true,pp-usa\n"
	if err := os.WriteFile(csvPath, []byte(csvBody), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	root := RootCommand("1.2.3")
	root.FlagSet.SetOutput(io.Discard)
	stdout, _ := captureOutput(t, func() {
		if err := root.Parse([]string{"subscriptions", "pricing", "prices", "import", "--subscription-id", "8000000001", "--input", csvPath, "--output", "json"}); err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if err := root.Run(context.Background()); err != nil {
			t.Fatalf("run error: %v", err)
		}
	})

	var summary struct {
		Created int `json:"created"`
		Skipped int `json:"skipped"`
		Failed  int `json:"failed"`
		Results []struct {
			Status string `json:"status"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(stdout), &summary); err != nil {
		t.Fatalf("parse JSON summary: %v", err)
	}
	if summary.Created != 0 || summary.Skipped != 1 || summary.Failed != 0 || postCount != 0 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if len(summary.Results) != 1 || summary.Results[0].Status != "skipped" {
		t.Fatalf("expected skipped row result, got %+v", summary.Results)
	}
}

func TestSubscriptionsPricesImport_ReconcilesAmbiguousCreateWithoutReplay(t *testing.T) {
	setupAuth(t)
	t.Setenv("ASC_MAX_RETRIES", "2")

	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	readCount := 0
	postCount := 0
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/v1/subscriptions/8000000001/prices":
			readCount++
			if readCount == 1 {
				return jsonHTTPResponse(http.StatusOK, `{"data":[],"links":{}}`), nil
			}
			body := `{"data":[{"type":"subscriptionPrices","id":"price-created","attributes":{"startDate":"2026-08-01","preserved":false},"relationships":{"territory":{"data":{"type":"territories","id":"USA"}},"subscriptionPricePoint":{"data":{"type":"subscriptionPricePoints","id":"pp-usa"}}}}],"links":{}}`
			return jsonHTTPResponse(http.StatusOK, body), nil
		case req.Method == http.MethodPost && req.URL.Path == "/v1/subscriptionPrices":
			postCount++
			return jsonHTTPResponse(http.StatusInternalServerError, `{"errors":[{"status":"500","code":"UNEXPECTED_ERROR","detail":"ambiguous"}]}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})

	csvPath := filepath.Join(t.TempDir(), "input.csv")
	csvBody := "territory,price,start_date,price_point_id\nUSA,19.99,2026-08-01,pp-usa\n"
	if err := os.WriteFile(csvPath, []byte(csvBody), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	root := RootCommand("1.2.3")
	root.FlagSet.SetOutput(io.Discard)
	stdout, _ := captureOutput(t, func() {
		if err := root.Parse([]string{"subscriptions", "pricing", "prices", "import", "--subscription-id", "8000000001", "--input", csvPath, "--output", "json"}); err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if err := root.Run(context.Background()); err != nil {
			t.Fatalf("run error: %v", err)
		}
	})

	var summary struct {
		Reconciled int `json:"reconciled"`
		Failed     int `json:"failed"`
		Results    []struct {
			Status string `json:"status"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(stdout), &summary); err != nil {
		t.Fatalf("parse JSON summary: %v", err)
	}
	if summary.Reconciled != 1 || summary.Failed != 0 || postCount != 1 || readCount != 2 {
		t.Fatalf("unexpected recovery result: summary=%+v posts=%d reads=%d", summary, postCount, readCount)
	}
	if len(summary.Results) != 1 || summary.Results[0].Status != "reconciled" {
		t.Fatalf("expected reconciled row result, got %+v", summary.Results)
	}
}

func TestSubscriptionsPricesImport_SkipsImmediatePriceWithConcreteEffectiveDate(t *testing.T) {
	setupAuth(t)

	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	readCount := 0
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/v1/subscriptions/8000000001/prices":
			readCount++
			body := `{"data":[{"type":"subscriptionPrices","id":"price-existing","attributes":{"startDate":"` + time.Now().UTC().Format("2006-01-02") + `","preserved":false},"relationships":{"territory":{"data":{"type":"territories","id":"USA"}},"subscriptionPricePoint":{"data":{"type":"subscriptionPricePoints","id":"pp-usa"}}}}],"links":{}}`
			return jsonHTTPResponse(http.StatusOK, body), nil
		case req.Method == http.MethodPost:
			t.Fatalf("current immediate price must not be posted again")
			return nil, nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})

	csvPath := filepath.Join(t.TempDir(), "input.csv")
	if err := os.WriteFile(csvPath, []byte("territory,price,price_point_id\nUSA,19.99,pp-usa\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	root := RootCommand("1.2.3")
	root.FlagSet.SetOutput(io.Discard)
	stdout, _ := captureOutput(t, func() {
		if err := root.Parse([]string{"subscriptions", "pricing", "prices", "import", "--subscription-id", "8000000001", "--input", csvPath, "--output", "json"}); err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if err := root.Run(context.Background()); err != nil {
			t.Fatalf("run error: %v", err)
		}
	})
	var summary struct {
		Skipped int `json:"skipped"`
	}
	if err := json.Unmarshal([]byte(stdout), &summary); err != nil {
		t.Fatalf("parse summary: %v", err)
	}
	if summary.Skipped != 1 || readCount != 1 {
		t.Fatalf("expected one indexed skip, summary=%+v reads=%d", summary, readCount)
	}
}

func TestSubscriptionsPricesImport_WritesVersionedFailureArtifact(t *testing.T) {
	setupAuth(t)
	t.Setenv("ASC_MAX_RETRIES", "0")
	t.Chdir(t.TempDir())

	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/v1/subscriptions/8000000001/prices":
			return jsonHTTPResponse(http.StatusOK, `{"data":[],"links":{}}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/v1/subscriptionPrices":
			return jsonHTTPResponse(http.StatusInternalServerError, `{"errors":[{"status":"500","code":"UNEXPECTED_ERROR","detail":"still unavailable"}]}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})

	csvPath := filepath.Join(t.TempDir(), "input.csv")
	csvBody := "territory,price,price_point_id\nUSA,19.99,pp-usa\n"
	if err := os.WriteFile(csvPath, []byte(csvBody), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	root := RootCommand("1.2.3")
	root.FlagSet.SetOutput(io.Discard)
	var runErr error
	stdout, _ := captureOutput(t, func() {
		if err := root.Parse([]string{"subscriptions", "pricing", "prices", "import", "--subscription-id", "8000000001", "--input", csvPath, "--output", "json"}); err != nil {
			t.Fatalf("parse error: %v", err)
		}
		runErr = root.Run(context.Background())
	})
	if _, ok := errors.AsType[ReportedError](runErr); !ok {
		t.Fatalf("expected ReportedError, got %v", runErr)
	}

	var summary struct {
		Failed              int    `json:"failed"`
		FailureArtifactPath string `json:"failureArtifactPath"`
	}
	if err := json.Unmarshal([]byte(stdout), &summary); err != nil {
		t.Fatalf("parse JSON summary: %v", err)
	}
	if summary.Failed != 1 || summary.FailureArtifactPath == "" {
		t.Fatalf("expected failure artifact, got %+v", summary)
	}
	data, err := os.ReadFile(summary.FailureArtifactPath)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	var artifact struct {
		SchemaVersion int `json:"schemaVersion"`
		Failed        int `json:"failed"`
		Results       []struct {
			Status               string `json:"status"`
			Territory            string `json:"territory"`
			Price                string `json:"price"`
			PricePointID         string `json:"pricePointId"`
			StartDate            string `json:"startDate"`
			PreserveCurrentPrice bool   `json:"preserveCurrentPrice"`
		} `json:"results"`
	}
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("parse artifact: %v", err)
	}
	if artifact.SchemaVersion != 1 || artifact.Failed != 1 || len(artifact.Results) != 1 || artifact.Results[0].Status != "failed" {
		t.Fatalf("unexpected artifact: %+v", artifact)
	}
	result := artifact.Results[0]
	if result.Territory != "USA" || result.Price != "19.99" || result.PricePointID != "pp-usa" || result.StartDate != "" || result.PreserveCurrentPrice {
		t.Fatalf("artifact is missing desired price state: %+v", result)
	}
}
