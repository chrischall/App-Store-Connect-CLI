package web

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net/http"
	"strings"
	"testing"

	webcore "github.com/rudrankriyam/App-Store-Connect-CLI/internal/web"
)

func TestWebReviewIAPsAttachRequiresApp(t *testing.T) {
	cmd := WebReviewIAPsAttachCommand()
	if err := cmd.FlagSet.Parse([]string{
		"--iap-id", "9000000001",
		"--confirm",
	}); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	_, stderr := captureOutput(t, func() {
		err := cmd.Exec(context.Background(), nil)
		if !errors.Is(err, flag.ErrHelp) {
			t.Fatalf("expected flag.ErrHelp, got %v", err)
		}
	})
	if !strings.Contains(stderr, "--app is required") {
		t.Fatalf("expected --app guidance in stderr, got %q", stderr)
	}
}

func TestWebReviewIAPsAttachRequiresIAPID(t *testing.T) {
	cmd := WebReviewIAPsAttachCommand()
	if err := cmd.FlagSet.Parse([]string{
		"--app", "123456789",
		"--confirm",
	}); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	_, stderr := captureOutput(t, func() {
		err := cmd.Exec(context.Background(), nil)
		if !errors.Is(err, flag.ErrHelp) {
			t.Fatalf("expected flag.ErrHelp, got %v", err)
		}
	})
	if !strings.Contains(stderr, "--iap-id is required") {
		t.Fatalf("expected --iap-id guidance in stderr, got %q", stderr)
	}
}

func TestWebReviewIAPsAttachRequiresConfirm(t *testing.T) {
	cmd := WebReviewIAPsAttachCommand()
	if err := cmd.FlagSet.Parse([]string{
		"--app", "123456789",
		"--iap-id", "9000000001",
	}); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	_, stderr := captureOutput(t, func() {
		err := cmd.Exec(context.Background(), nil)
		if !errors.Is(err, flag.ErrHelp) {
			t.Fatalf("expected flag.ErrHelp, got %v", err)
		}
	})
	if !strings.Contains(stderr, "--confirm is required") {
		t.Fatalf("expected --confirm guidance in stderr, got %q", stderr)
	}
}

func TestWebReviewIAPsAttachRejectsNonNumericAppID(t *testing.T) {
	cmd := WebReviewIAPsAttachCommand()
	if err := cmd.FlagSet.Parse([]string{
		"--app", "com.example.app",
		"--iap-id", "9000000001",
		"--confirm",
	}); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	_, stderr := captureOutput(t, func() {
		err := cmd.Exec(context.Background(), nil)
		if !errors.Is(err, flag.ErrHelp) {
			t.Fatalf("expected flag.ErrHelp, got %v", err)
		}
	})
	if !strings.Contains(stderr, "--app must be a numeric App Store Connect app ID") {
		t.Fatalf("expected numeric --app guidance in stderr, got %q", stderr)
	}
}

func TestWebReviewIAPsAttachVerifiesIAPBelongsToAppBeforeMutating(t *testing.T) {
	_ = stubWebProgressLabels(t)

	origResolveSession := resolveSessionFn
	t.Cleanup(func() {
		resolveSessionFn = origResolveSession
	})

	verified := false
	resolveSessionFn = func(ctx context.Context, appleID, password, twoFactorCode string) (*webcore.AuthSession, string, error) {
		return &webcore.AuthSession{
			Client: &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					switch {
					case req.Method == http.MethodGet && req.URL.Path == "/iris/v1/apps/123456789/inAppPurchases":
						verified = true
						return &http.Response{
							StatusCode: http.StatusOK,
							Header:     http.Header{"Content-Type": []string{"application/json"}},
							Body: io.NopCloser(strings.NewReader(`{
								"data": [{
									"type": "inAppPurchases",
									"id": "9000000001",
									"attributes": {
										"name": "Remove Ads",
										"productId": "com.example.removeads",
										"inAppPurchaseType": "NON_CONSUMABLE",
										"state": "READY_TO_SUBMIT",
										"submitWithNextAppStoreVersion": false
									}
								}]
							}`)),
							Request: req,
						}, nil
					case req.Method == http.MethodPost && req.URL.Path == "/iris/v1/inAppPurchaseSubmissions":
						if !verified {
							t.Fatal("expected app-scoped IAP verification before web mutation")
						}
					default:
						t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
					}
					requestBody, err := io.ReadAll(req.Body)
					if err != nil {
						t.Fatalf("read request body: %v", err)
					}
					body := string(requestBody)
					for _, want := range []string{
						`"id":"9000000001"`,
						`"submitWithNextAppStoreVersion":true`,
						`"inAppPurchaseV2"`,
					} {
						if !strings.Contains(body, want) {
							t.Fatalf("expected request body to contain %q, got %s", want, body)
						}
					}
					return &http.Response{
						StatusCode: http.StatusCreated,
						Header:     http.Header{"Content-Type": []string{"application/json"}},
						Body: io.NopCloser(strings.NewReader(`{
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
						}`)),
						Request: req,
					}, nil
				}),
			},
		}, "cache", nil
	}

	cmd := WebReviewIAPsAttachCommand()
	if err := cmd.FlagSet.Parse([]string{
		"--app", "123456789",
		"--iap-id", "9000000001",
		"--confirm",
		"--output", "json",
	}); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	stdout, _ := captureOutput(t, func() {
		if err := cmd.Exec(context.Background(), nil); err != nil {
			t.Fatalf("exec error: %v", err)
		}
	})

	var payload reviewIAPMutationOutput
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse stdout JSON: %v\nstdout=%s", err, stdout)
	}
	if payload.AppID != "123456789" || payload.IAPID != "9000000001" || payload.Operation != "attach" || !payload.Changed {
		t.Fatalf("unexpected mutation output: %#v", payload)
	}
	if payload.Submission.ID != "submission-1" || payload.Submission.InAppPurchaseID != "9000000001" {
		t.Fatalf("unexpected submission output: %#v", payload.Submission)
	}
}

func TestWebReviewIAPsAttachSkipsAlreadyAttachedIAP(t *testing.T) {
	_ = stubWebProgressLabels(t)

	origResolveSession := resolveSessionFn
	t.Cleanup(func() {
		resolveSessionFn = origResolveSession
	})

	resolveSessionFn = func(ctx context.Context, appleID, password, twoFactorCode string) (*webcore.AuthSession, string, error) {
		return &webcore.AuthSession{
			Client: &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					if req.Method != http.MethodGet || req.URL.Path != "/iris/v1/apps/123456789/inAppPurchases" {
						t.Fatalf("unexpected request for already-attached IAP: %s %s", req.Method, req.URL.Path)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"application/json"}},
						Body: io.NopCloser(strings.NewReader(`{
							"data": [{
								"type": "inAppPurchases",
								"id": "9000000001",
								"attributes": {
									"name": "Remove Ads",
									"productId": "com.example.removeads",
									"inAppPurchaseType": "NON_CONSUMABLE",
									"state": "READY_TO_SUBMIT",
									"submitWithNextAppStoreVersion": true
								}
							}]
						}`)),
						Request: req,
					}, nil
				}),
			},
		}, "cache", nil
	}

	cmd := WebReviewIAPsAttachCommand()
	if err := cmd.FlagSet.Parse([]string{
		"--app", "123456789",
		"--iap-id", "9000000001",
		"--confirm",
		"--output", "json",
	}); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	stdout, _ := captureOutput(t, func() {
		if err := cmd.Exec(context.Background(), nil); err != nil {
			t.Fatalf("exec error: %v", err)
		}
	})

	var payload reviewIAPMutationOutput
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse stdout JSON: %v\nstdout=%s", err, stdout)
	}
	if payload.AppID != "123456789" || payload.IAPID != "9000000001" || payload.Operation != "attach" {
		t.Fatalf("unexpected mutation output: %#v", payload)
	}
	if payload.Changed {
		t.Fatalf("expected already-attached IAP to report changed=false, got %#v", payload)
	}
	if payload.Submission.ID != "" || payload.Submission.InAppPurchaseID != "9000000001" || !payload.Submission.SubmitWithNextAppStoreVersion {
		t.Fatalf("unexpected idempotent submission output: %#v", payload.Submission)
	}
}

func TestWebReviewIAPsAttachRefusesIAPOutsideApp(t *testing.T) {
	origResolveSession := resolveSessionFn
	t.Cleanup(func() {
		resolveSessionFn = origResolveSession
	})

	resolveSessionFn = func(ctx context.Context, appleID, password, twoFactorCode string) (*webcore.AuthSession, string, error) {
		return &webcore.AuthSession{
			Client: &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					if req.Method != http.MethodGet || req.URL.Path != "/iris/v1/apps/123456789/inAppPurchases" {
						t.Fatalf("unexpected request before app-scoping refusal: %s %s", req.Method, req.URL.Path)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"application/json"}},
						Body:       io.NopCloser(strings.NewReader(`{"data":[]}`)),
						Request:    req,
					}, nil
				}),
			},
		}, "cache", nil
	}

	cmd := WebReviewIAPsAttachCommand()
	if err := cmd.FlagSet.Parse([]string{
		"--app", "123456789",
		"--iap-id", "9000000001",
		"--confirm",
	}); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	err := cmd.Exec(context.Background(), nil)
	if err == nil {
		t.Fatal("expected app-scoping error")
	}
	if !strings.Contains(err.Error(), `in-app purchase "9000000001" was not found under app "123456789"`) {
		t.Fatalf("expected app-scoping error, got %v", err)
	}
}

func TestWebReviewIAPsGroupCommandReturnsHelpWhenNoSubcommand(t *testing.T) {
	cmd := WebReviewIAPsCommand()
	if cmd.UsageFunc == nil {
		t.Fatal("WebReviewIAPsCommand should set UsageFunc for consistent rendering")
	}
	err := cmd.Exec(context.Background(), nil)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp from group Exec with no subcommand, got %v", err)
	}
}
