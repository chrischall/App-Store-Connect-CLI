package cmdtest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rudrankriyam/App-Store-Connect-CLI/cmd"
)

func TestLocalizationsUploadDryRunWarnsForPlannedCreate(t *testing.T) {
	setupAuth(t)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ja.strings"), []byte("\"description\" = \"日本語説明\";\n"), 0o644); err != nil {
		t.Fatalf("write strings file: %v", err)
	}

	originalTransport := http.DefaultTransport
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
	})

	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/v1/appStoreVersions/version-1/appStoreVersionLocalizations":
			body := `{"data":[],"links":{"next":""}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		case req.Method == http.MethodGet && req.URL.Path == "/v1/appStoreVersions/version-1":
			body := `{"data":{"type":"appStoreVersions","id":"version-1","attributes":{"versionString":"1.2.3","platform":"IOS"},"relationships":{"app":{"data":{"type":"apps","id":"app-1"}}}}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		case req.Method == http.MethodGet && req.URL.Path == "/v1/apps/app-1/appStoreVersions":
			body := `{"data":[],"links":{"next":""}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})

	root := RootCommand("1.2.3")
	root.FlagSet.SetOutput(io.Discard)

	stdout, stderr := captureOutput(t, func() {
		if err := root.Parse([]string{
			"localizations", "upload",
			"--version", "version-1",
			"--path", dir,
			"--dry-run",
		}); err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if err := root.Run(context.Background()); err != nil {
			t.Fatalf("run error: %v", err)
		}
	})

	if !strings.Contains(stderr, "creating locale ja would make it participate in submission validation") {
		t.Fatalf("expected planned create warning on stderr, got %q", stderr)
	}
	if !strings.Contains(stderr, "keywords, supportUrl") {
		t.Fatalf("expected missing submit fields in warning, got %q", stderr)
	}

	var out struct {
		DryRun  bool `json:"dryRun"`
		Results []struct {
			Locale string `json:"locale"`
			Action string `json:"action"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout should be valid json: %v\nstdout=%q", err, stdout)
	}
	if !out.DryRun {
		t.Fatalf("expected dryRun=true, got %+v", out)
	}
	if len(out.Results) != 1 || out.Results[0].Locale != "ja" || out.Results[0].Action != "create" {
		t.Fatalf("expected single create result, got %+v", out.Results)
	}
}

func TestLocalizationsUploadAppliedCreateWarns(t *testing.T) {
	setupAuth(t)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ja.strings"), []byte("\"description\" = \"日本語説明\";\n"), 0o644); err != nil {
		t.Fatalf("write strings file: %v", err)
	}

	originalTransport := http.DefaultTransport
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
	})

	createCount := 0
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/v1/appStoreVersions/version-1/appStoreVersionLocalizations":
			body := `{"data":[],"links":{"next":""}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		case req.Method == http.MethodGet && req.URL.Path == "/v1/appStoreVersions/version-1":
			body := `{"data":{"type":"appStoreVersions","id":"version-1","attributes":{"versionString":"1.2.3","platform":"IOS"},"relationships":{"app":{"data":{"type":"apps","id":"app-1"}}}}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		case req.Method == http.MethodGet && req.URL.Path == "/v1/apps/app-1/appStoreVersions":
			body := `{"data":[],"links":{"next":""}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		case req.Method == http.MethodPost && req.URL.Path == "/v1/appStoreVersionLocalizations":
			createCount++
			body := `{"data":{"type":"appStoreVersionLocalizations","id":"loc-ja","attributes":{"locale":"ja","description":"日本語説明"}}}`
			return &http.Response{
				StatusCode: http.StatusCreated,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})

	root := RootCommand("1.2.3")
	root.FlagSet.SetOutput(io.Discard)

	stdout, stderr := captureOutput(t, func() {
		if err := root.Parse([]string{
			"localizations", "upload",
			"--version", "version-1",
			"--path", dir,
		}); err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if err := root.Run(context.Background()); err != nil {
			t.Fatalf("run error: %v", err)
		}
	})

	if createCount != 1 {
		t.Fatalf("expected one create request, got %d", createCount)
	}
	if !strings.Contains(stderr, "created locale ja now participates in submission validation") {
		t.Fatalf("expected applied create warning on stderr, got %q", stderr)
	}

	var out struct {
		DryRun  bool `json:"dryRun"`
		Results []struct {
			Locale string `json:"locale"`
			Action string `json:"action"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout should be valid json: %v\nstdout=%q", err, stdout)
	}
	if out.DryRun {
		t.Fatalf("expected dryRun=false, got %+v", out)
	}
	if len(out.Results) != 1 || out.Results[0].Locale != "ja" || out.Results[0].Action != "create" {
		t.Fatalf("expected single create result, got %+v", out.Results)
	}
}

func TestRunLocalizationsUploadRejectsOverLimitKeywordCharactersBeforeAuthResolution(t *testing.T) {
	t.Setenv("ASC_BYPASS_KEYCHAIN", "1")
	t.Setenv("ASC_KEY_ID", "")
	t.Setenv("ASC_ISSUER_ID", "")
	t.Setenv("ASC_PRIVATE_KEY_PATH", "")
	t.Setenv("ASC_PRIVATE_KEY", "")
	t.Setenv("ASC_PRIVATE_KEY_B64", "")
	t.Setenv("ASC_APP_ID", "")
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "nonexistent.json"))

	dir := t.TempDir()
	content := "\"description\" = \"日本語説明\";\n\"keywords\" = \"" + strings.Repeat("語", 101) + "\";\n"
	if err := os.WriteFile(filepath.Join(dir, "ja.strings"), []byte(content), 0o644); err != nil {
		t.Fatalf("write strings file: %v", err)
	}

	originalTransport := http.DefaultTransport
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
	})

	requestCount := 0
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requestCount++
		t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
		return nil, nil
	})

	stdout, stderr := captureOutput(t, func() {
		code := cmd.Run([]string{
			"localizations", "upload",
			"--version", "version-1",
			"--path", dir,
		}, "1.2.3")
		if code != cmd.ExitUsage {
			t.Fatalf("expected exit code %d, got %d", cmd.ExitUsage, code)
		}
	})

	if stdout != "" {
		t.Fatalf("expected empty stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "keywords exceed 100 characters") {
		t.Fatalf("expected keyword character-limit error, got %q", stderr)
	}
	if requestCount != 0 {
		t.Fatalf("expected no HTTP requests, got %d", requestCount)
	}
}

func TestRunLocalizationsUploadRejectsRawKeywordCharactersIncludingTrailingSpaceBeforeAuthResolution(t *testing.T) {
	t.Setenv("ASC_BYPASS_KEYCHAIN", "1")
	t.Setenv("ASC_KEY_ID", "")
	t.Setenv("ASC_ISSUER_ID", "")
	t.Setenv("ASC_PRIVATE_KEY_PATH", "")
	t.Setenv("ASC_PRIVATE_KEY", "")
	t.Setenv("ASC_PRIVATE_KEY_B64", "")
	t.Setenv("ASC_APP_ID", "")
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "nonexistent.json"))

	dir := t.TempDir()
	content := "\"description\" = \"日本語説明\";\n\"keywords\" = \"" + strings.Repeat("a", 100) + " \";\n"
	if err := os.WriteFile(filepath.Join(dir, "ja.strings"), []byte(content), 0o644); err != nil {
		t.Fatalf("write strings file: %v", err)
	}

	originalTransport := http.DefaultTransport
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
	})

	requestCount := 0
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requestCount++
		t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
		return nil, nil
	})

	stdout, stderr := captureOutput(t, func() {
		code := cmd.Run([]string{
			"localizations", "upload",
			"--version", "version-1",
			"--path", dir,
		}, "1.2.3")
		if code != cmd.ExitUsage {
			t.Fatalf("expected exit code %d, got %d", cmd.ExitUsage, code)
		}
	})

	if stdout != "" {
		t.Fatalf("expected empty stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "keywords exceed 100 characters") {
		t.Fatalf("expected keyword character-limit error, got %q", stderr)
	}
	if requestCount != 0 {
		t.Fatalf("expected no HTTP requests, got %d", requestCount)
	}
}

func TestLocalizationsUploadUpdateOnlyDoesNotWarn(t *testing.T) {
	setupAuth(t)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "en-US.strings"), []byte("\"description\" = \"Updated description\";\n"), 0o644); err != nil {
		t.Fatalf("write strings file: %v", err)
	}

	originalTransport := http.DefaultTransport
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
	})

	updateCount := 0
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/v1/appStoreVersions/version-1/appStoreVersionLocalizations":
			body := `{"data":[{"type":"appStoreVersionLocalizations","id":"loc-en","attributes":{"locale":"en-US","description":"Old description"}}],"links":{"next":""}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		case req.Method == http.MethodGet && req.URL.Path == "/v1/appStoreVersions/version-1":
			body := `{"data":{"type":"appStoreVersions","id":"version-1","attributes":{"versionString":"1.2.3","platform":"IOS"},"relationships":{"app":{"data":{"type":"apps","id":"app-1"}}}}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		case req.Method == http.MethodGet && req.URL.Path == "/v1/apps/app-1/appStoreVersions":
			body := `{"data":[],"links":{"next":""}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		case req.Method == http.MethodPatch && req.URL.Path == "/v1/appStoreVersionLocalizations/loc-en":
			updateCount++
			body := `{"data":{"type":"appStoreVersionLocalizations","id":"loc-en","attributes":{"locale":"en-US","description":"Updated description"}}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})

	root := RootCommand("1.2.3")
	root.FlagSet.SetOutput(io.Discard)

	stdout, stderr := captureOutput(t, func() {
		if err := root.Parse([]string{
			"localizations", "upload",
			"--version", "version-1",
			"--path", dir,
		}); err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if err := root.Run(context.Background()); err != nil {
			t.Fatalf("run error: %v", err)
		}
	})

	if updateCount != 1 {
		t.Fatalf("expected one update request, got %d", updateCount)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	var out struct {
		Results []struct {
			Locale string `json:"locale"`
			Action string `json:"action"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout should be valid json: %v\nstdout=%q", err, stdout)
	}
	if len(out.Results) != 1 || out.Results[0].Locale != "en-US" || out.Results[0].Action != "update" {
		t.Fatalf("expected single update result, got %+v", out.Results)
	}
}

func TestLocalizationsUploadPartialFailurePrintsSummaryAndArtifact(t *testing.T) {
	result, runErr := runPartialLocalizationsUpload(t, false)
	if _, ok := errors.AsType[ReportedError](runErr); !ok {
		t.Fatalf("expected ReportedError, got %T: %v", runErr, runErr)
	}
	if result.Total != 3 || result.Succeeded != 2 || result.Failed != 1 || result.FailureArtifactPath == "" || result.FailureArtifactError != "" {
		t.Fatalf("unexpected partial summary: %+v", result)
	}
	payload, err := os.ReadFile(result.FailureArtifactPath)
	if err != nil {
		t.Fatalf("read failure artifact: %v", err)
	}
	if !strings.Contains(string(payload), `"schemaVersion": 1`) || !strings.Contains(string(payload), `"description": "New French"`) {
		t.Fatalf("failure artifact is not resumable: %s", payload)
	}
}

func TestLocalizationsUploadArtifactWriteFailureStillPrintsSummary(t *testing.T) {
	result, runErr := runPartialLocalizationsUpload(t, true)
	if _, ok := errors.AsType[ReportedError](runErr); !ok {
		t.Fatalf("expected ReportedError, got %T: %v", runErr, runErr)
	}
	if result.Total != 3 || result.Succeeded != 2 || result.Failed != 1 || result.FailureArtifactPath != "" || result.FailureArtifactError == "" {
		t.Fatalf("expected printed artifact failure summary: %+v", result)
	}
}

func TestLocalizationsUploadInitialReadFailureIsNotZeroFailureSummary(t *testing.T) {
	setupAuth(t)
	t.Setenv("ASC_MAX_RETRIES", "0")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "en-US.strings"), []byte("\"description\" = \"Desired\";\n\"whatsNew\" = \"Notes\";\n"), 0o644); err != nil {
		t.Fatalf("write strings: %v", err)
	}
	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.Path != "/v1/appStoreVersions/version-1/appStoreVersionLocalizations" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
		}
		return jsonHTTPResponse(http.StatusInternalServerError, `{"errors":[{"status":"500","code":"SERVER_ERROR","detail":"unavailable"}]}`), nil
	})

	root := RootCommand("1.2.3")
	root.FlagSet.SetOutput(io.Discard)
	var runErr error
	stdout, _ := captureOutput(t, func() {
		if err := root.Parse([]string{"localizations", "upload", "--version", "version-1", "--path", dir, "--output", "json"}); err != nil {
			t.Fatalf("parse: %v", err)
		}
		runErr = root.Run(context.Background())
	})
	if runErr == nil {
		t.Fatal("expected initial read failure")
	}
	if _, ok := errors.AsType[ReportedError](runErr); ok {
		t.Fatalf("expected ordinary global error, got ReportedError: %v", runErr)
	}
	if stdout != "" || strings.Contains(runErr.Error(), "0 locale(s) failed") {
		t.Fatalf("unexpected zero-failure summary: stdout=%q err=%v", stdout, runErr)
	}
}

type partialLocalizationUploadResult struct {
	Total                int    `json:"total"`
	Succeeded            int    `json:"succeeded"`
	Failed               int    `json:"failed"`
	FailureArtifactPath  string `json:"failureArtifactPath"`
	FailureArtifactError string `json:"failureArtifactError"`
}

func runPartialLocalizationsUpload(t *testing.T, blockArtifact bool) (partialLocalizationUploadResult, error) {
	t.Helper()
	setupAuth(t)
	t.Setenv("ASC_MAX_RETRIES", "0")
	workDir := t.TempDir()
	t.Chdir(workDir)
	inputDir := filepath.Join(workDir, "input")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatalf("mkdir input: %v", err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "en-US.strings"), []byte("\"description\" = \"New English\";\n\"whatsNew\" = \"English notes\";\n"), 0o644); err != nil {
		t.Fatalf("write English: %v", err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "fr-FR.strings"), []byte("\"description\" = \"New French\";\n\"whatsNew\" = \"French notes\";\n"), 0o644); err != nil {
		t.Fatalf("write French: %v", err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "ja.strings"), []byte("\"description\" = \"New Japanese\";\n\"whatsNew\" = \"Japanese notes\";\n"), 0o644); err != nil {
		t.Fatalf("write Japanese: %v", err)
	}
	if blockArtifact {
		if err := os.WriteFile(".asc", []byte("not a directory"), 0o644); err != nil {
			t.Fatalf("write artifact blocker: %v", err)
		}
	}

	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	patches := 0
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/v1/appStoreVersions/version-1/appStoreVersionLocalizations":
			return jsonHTTPResponse(http.StatusOK, `{"data":[{"type":"appStoreVersionLocalizations","id":"en-id","attributes":{"locale":"en-US","description":"Old English"}},{"type":"appStoreVersionLocalizations","id":"fr-id","attributes":{"locale":"fr-FR","description":"Old French"}},{"type":"appStoreVersionLocalizations","id":"ja-id","attributes":{"locale":"ja","description":"Old Japanese"}}],"links":{"next":""}}`), nil
		case req.Method == http.MethodPatch && req.URL.Path == "/v1/appStoreVersionLocalizations/en-id":
			patches++
			return jsonHTTPResponse(http.StatusOK, `{"data":{"type":"appStoreVersionLocalizations","id":"en-id","attributes":{"locale":"en-US","description":"New English","whatsNew":"English notes"}}}`), nil
		case req.Method == http.MethodPatch && req.URL.Path == "/v1/appStoreVersionLocalizations/fr-id":
			patches++
			return jsonHTTPResponse(http.StatusBadRequest, `{"errors":[{"status":"400","code":"ENTITY_ERROR","detail":"French rejected"}]}`), nil
		case req.Method == http.MethodPatch && req.URL.Path == "/v1/appStoreVersionLocalizations/ja-id":
			patches++
			return jsonHTTPResponse(http.StatusOK, `{"data":{"type":"appStoreVersionLocalizations","id":"ja-id","attributes":{"locale":"ja","description":"New Japanese","whatsNew":"Japanese notes"}}}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})

	root := RootCommand("1.2.3")
	root.FlagSet.SetOutput(io.Discard)
	var runErr error
	stdout, _ := captureOutput(t, func() {
		if err := root.Parse([]string{"localizations", "upload", "--version", "version-1", "--path", inputDir, "--output", "json"}); err != nil {
			t.Fatalf("parse: %v", err)
		}
		runErr = root.Run(context.Background())
	})
	if patches != 3 {
		t.Fatalf("expected later locale after failure, patches=%d", patches)
	}
	var result partialLocalizationUploadResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("parse summary: %v; stdout=%q", err, stdout)
	}
	return result, runErr
}
