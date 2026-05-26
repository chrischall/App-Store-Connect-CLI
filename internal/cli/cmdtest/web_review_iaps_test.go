package cmdtest

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os/exec"
	"strings"
	"testing"
	"time"

	rootcmd "github.com/rudrankriyam/App-Store-Connect-CLI/cmd"
	webcore "github.com/rudrankriyam/App-Store-Connect-CLI/internal/web"
)

func TestWebReviewIAPsAttachRootSuccess(t *testing.T) {
	setupAuth(t)
	setupCachedWebReviewIAPSession(t, "user@example.com")

	requests := newRequestLog(3)
	installDefaultTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests.Add(req.Method + " " + req.URL.Host + req.URL.Path)
		switch {
		case req.Method == http.MethodGet &&
			req.URL.Host == "api.appstoreconnect.apple.com" &&
			req.URL.Path == "/v1/apps/123456789/inAppPurchasesV2":
			if got := req.URL.Query().Get("limit"); got != "200" {
				t.Fatalf("expected IAP verification limit=200, got %q", got)
			}
			return webReviewIAPJSONResponse(http.StatusOK, `{
				"data": [{
					"type": "inAppPurchases",
					"id": "9000000001",
					"attributes": {
						"name": "Remove Ads",
						"productId": "com.example.removeads",
						"inAppPurchaseType": "NON_CONSUMABLE"
					}
				}]
			}`, req), nil
		case req.Method == http.MethodGet &&
			req.URL.Host == "appstoreconnect.apple.com" &&
			req.URL.Path == "/olympus/v1/session":
			return webReviewIAPJSONResponse(http.StatusOK, `{
				"provider": {
					"providerId": 123456,
					"publicProviderId": "team-1",
					"name": "Team"
				},
				"user": {
					"emailAddress": "user@example.com"
				}
			}`, req), nil
		case req.Method == http.MethodPost &&
			req.URL.Host == "appstoreconnect.apple.com" &&
			req.URL.Path == "/iris/v1/inAppPurchaseSubmissions":
			bodyBytes, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			body := string(bodyBytes)
			for _, want := range []string{
				`"id":"9000000001"`,
				`"submitWithNextAppStoreVersion":true`,
				`"inAppPurchaseV2"`,
			} {
				if !strings.Contains(body, want) {
					t.Fatalf("expected request body to contain %q, got %s", want, body)
				}
			}
			return webReviewIAPJSONResponse(http.StatusCreated, `{
				"data": {
					"type": "inAppPurchaseSubmissions",
					"id": "submission-1",
					"attributes": {
						"submitWithNextAppStoreVersion": true
					},
					"relationships": {
						"inAppPurchaseV2": {
							"data": {"type": "inAppPurchases", "id": "9000000001"}
						}
					}
				}
			}`, req), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	}))

	root := RootCommand("1.2.3")
	root.FlagSet.SetOutput(io.Discard)

	var runErr error
	stdout, stderr := captureOutput(t, func() {
		if err := root.Parse([]string{
			"web", "review", "iaps", "attach",
			"--app", "123456789",
			"--iap-id", "9000000001",
			"--confirm",
			"--apple-id", "user@example.com",
			"--output", "json",
		}); err != nil {
			t.Fatalf("parse error: %v", err)
		}
		runErr = root.Run(context.Background())
	})

	if runErr != nil {
		t.Fatalf("run error: %v\nstderr=%s", runErr, stderr)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	var payload struct {
		AppID      string `json:"appId"`
		IAPID      string `json:"iapId"`
		Operation  string `json:"operation"`
		Changed    bool   `json:"changed"`
		Submission struct {
			ID              string `json:"id"`
			InAppPurchaseID string `json:"inAppPurchaseId"`
		} `json:"submission"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse stdout JSON: %v\nstdout=%s", err, stdout)
	}
	if payload.AppID != "123456789" || payload.IAPID != "9000000001" || payload.Operation != "attach" || !payload.Changed {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if payload.Submission.ID != "submission-1" || payload.Submission.InAppPurchaseID != "9000000001" {
		t.Fatalf("unexpected submission: %#v", payload.Submission)
	}

	wantRequests := []string{
		"GET api.appstoreconnect.apple.com/v1/apps/123456789/inAppPurchasesV2",
		"GET appstoreconnect.apple.com/olympus/v1/session",
		"POST appstoreconnect.apple.com/iris/v1/inAppPurchaseSubmissions",
	}
	if got := requests.Snapshot(); strings.Join(got, "\n") != strings.Join(wantRequests, "\n") {
		t.Fatalf("unexpected requests:\nwant=%v\ngot=%v", wantRequests, got)
	}
}

func TestWebReviewIAPsAttachValidationErrors(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{
			name:       "missing app",
			args:       []string{"web", "review", "iaps", "attach", "--iap-id", "9000000001", "--confirm"},
			wantStderr: "--app is required",
		},
		{
			name:       "missing iap id",
			args:       []string{"web", "review", "iaps", "attach", "--app", "123456789", "--confirm"},
			wantStderr: "--iap-id is required",
		},
		{
			name:       "missing confirm",
			args:       []string{"web", "review", "iaps", "attach", "--app", "123456789", "--iap-id", "9000000001"},
			wantStderr: "--confirm is required",
		},
		{
			name:       "non numeric app",
			args:       []string{"web", "review", "iaps", "attach", "--app", "com.example.app", "--iap-id", "9000000001", "--confirm"},
			wantStderr: "--app must be a numeric App Store Connect app ID",
		},
		{
			name:       "non numeric iap",
			args:       []string{"web", "review", "iaps", "attach", "--app", "123456789", "--iap-id", "com.example.pro", "--confirm"},
			wantStderr: "--iap-id must be a numeric App Store Connect in-app purchase ID",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := RootCommand("1.2.3")
			root.FlagSet.SetOutput(io.Discard)

			var runErr error
			stdout, stderr := captureOutput(t, func() {
				if err := root.Parse(test.args); err != nil {
					t.Fatalf("parse error: %v", err)
				}
				runErr = root.Run(context.Background())
			})

			if !errors.Is(runErr, flag.ErrHelp) {
				t.Fatalf("expected flag.ErrHelp, got %v", runErr)
			}
			if stdout != "" {
				t.Fatalf("expected empty stdout, got %q", stdout)
			}
			if !strings.Contains(stderr, test.wantStderr) {
				t.Fatalf("expected stderr to contain %q, got %q", test.wantStderr, stderr)
			}
		})
	}
}

func TestWebReviewIAPsAttachArgumentParsingEdges(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "mixed flag order",
			args: []string{
				"web", "review", "iaps", "attach",
				"--iap-id", "9000000001",
				"--apple-id", "attach",
				"--app", "123456789",
				"--confirm",
			},
		},
		{
			name: "flag value matching subcommand",
			args: []string{
				"web", "review", "iaps", "attach",
				"--app", "123456789",
				"--iap-id", "9000000001",
				"--apple-id", "attach",
				"--confirm",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := RootCommand("1.2.3")
			root.FlagSet.SetOutput(io.Discard)

			if err := root.Parse(test.args); err != nil {
				t.Fatalf("parse error: %v", err)
			}
		})
	}
}

func TestWebReviewIAPsAttachInvalidValueExitCodes(t *testing.T) {
	bin := buildCLIBinary(t)
	tests := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{
			name:       "invalid app selector",
			args:       []string{"web", "review", "iaps", "attach", "--app", "com.example.app", "--iap-id", "9000000001", "--confirm"},
			wantStderr: "--app must be a numeric App Store Connect app ID",
		},
		{
			name:       "invalid iap selector",
			args:       []string{"web", "review", "iaps", "attach", "--app", "123456789", "--iap-id", "com.example.pro", "--confirm"},
			wantStderr: "--iap-id must be a numeric App Store Connect in-app purchase ID",
		},
		{
			name:       "invalid confirm value",
			args:       []string{"web", "review", "iaps", "attach", "--app", "123456789", "--iap-id", "9000000001", "--confirm=maybe"},
			wantStderr: `invalid boolean value "maybe" for -confirm`,
		},
		{
			name:       "subcommand flag before attach rejected",
			args:       []string{"web", "review", "iaps", "--app", "123456789", "attach", "--iap-id", "9000000001", "--confirm"},
			wantStderr: "flag provided but not defined: -app",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := exec.Command(bin, test.args...)
			var stdout, stderr strings.Builder
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("expected exit error, got %v", err)
			}
			if code := exitErr.ExitCode(); code != rootcmd.ExitUsage {
				t.Fatalf("exit code = %d, want %d", code, rootcmd.ExitUsage)
			}
			if stdout.String() != "" {
				t.Fatalf("expected empty stdout, got %q", stdout.String())
			}
			if !strings.Contains(stderr.String(), test.wantStderr) {
				t.Fatalf("expected stderr to contain %q, got %q", test.wantStderr, stderr.String())
			}
		})
	}
}

func setupCachedWebReviewIAPSession(t *testing.T, email string) {
	t.Helper()

	t.Setenv("ASC_WEB_SESSION_CACHE", "1")
	t.Setenv("ASC_WEB_SESSION_CACHE_BACKEND", "file")
	t.Setenv("ASC_WEB_SESSION_CACHE_DIR", t.TempDir())

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create cookie jar: %v", err)
	}
	appStoreURL, err := url.Parse("https://appstoreconnect.apple.com/")
	if err != nil {
		t.Fatalf("parse app store URL: %v", err)
	}
	jar.SetCookies(appStoreURL, []*http.Cookie{
		{
			Name:    "myacinfo",
			Value:   "test-token",
			Path:    "/",
			Domain:  "appstoreconnect.apple.com",
			Expires: time.Now().Add(time.Hour),
		},
	})
	if err := webcore.PersistSession(&webcore.AuthSession{
		Client:    &http.Client{Jar: jar},
		UserEmail: email,
	}); err != nil {
		t.Fatalf("persist cached web session: %v", err)
	}
}

func webReviewIAPJSONResponse(status int, body string, req *http.Request) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}
