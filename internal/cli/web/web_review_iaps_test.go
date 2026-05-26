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

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/asc"
	webcore "github.com/rudrankriyam/App-Store-Connect-CLI/internal/web"
)

type reviewIAPIAPListClientFunc func(ctx context.Context, appID string, opts ...asc.IAPOption) (*asc.InAppPurchasesV2Response, error)

func (fn reviewIAPIAPListClientFunc) GetInAppPurchasesV2(ctx context.Context, appID string, opts ...asc.IAPOption) (*asc.InAppPurchasesV2Response, error) {
	return fn(ctx, appID, opts...)
}

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

func TestWebReviewIAPsAttachRejectsNonNumericIAPID(t *testing.T) {
	cmd := WebReviewIAPsAttachCommand()
	if err := cmd.FlagSet.Parse([]string{
		"--app", "123456789",
		"--iap-id", "com.example.pro",
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
	if !strings.Contains(stderr, "--iap-id must be a numeric App Store Connect in-app purchase ID") {
		t.Fatalf("expected numeric --iap-id guidance in stderr, got %q", stderr)
	}
}

func TestWebReviewIAPsAttachVerifiesIAPBelongsToAppBeforeMutating(t *testing.T) {
	_ = stubWebProgressLabels(t)

	origASCClient := newReviewIAPASCClientFn
	origResolveSession := resolveSessionFn
	t.Cleanup(func() {
		newReviewIAPASCClientFn = origASCClient
		resolveSessionFn = origResolveSession
	})

	verified := false
	newReviewIAPASCClientFn = func() (reviewIAPIAPListClient, error) {
		return reviewIAPIAPListClientFunc(func(ctx context.Context, appID string, opts ...asc.IAPOption) (*asc.InAppPurchasesV2Response, error) {
			if appID != "123456789" {
				t.Fatalf("expected app ID 123456789, got %q", appID)
			}
			verified = true
			return &asc.InAppPurchasesV2Response{
				Data: []asc.Resource[asc.InAppPurchaseV2Attributes]{
					{Type: asc.ResourceTypeInAppPurchases, ID: "9000000001"},
				},
			}, nil
		}), nil
	}

	resolveSessionFn = func(ctx context.Context, appleID, password, twoFactorCode string) (*webcore.AuthSession, string, error) {
		return &webcore.AuthSession{
			Client: &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					if !verified {
						t.Fatal("expected app-scoped IAP verification before web mutation")
					}
					if req.Method != http.MethodPost || req.URL.Path != "/iris/v1/inAppPurchaseSubmissions" {
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

func TestWebReviewIAPsAttachRefusesIAPOutsideApp(t *testing.T) {
	origASCClient := newReviewIAPASCClientFn
	origResolveSession := resolveSessionFn
	t.Cleanup(func() {
		newReviewIAPASCClientFn = origASCClient
		resolveSessionFn = origResolveSession
	})

	newReviewIAPASCClientFn = func() (reviewIAPIAPListClient, error) {
		return reviewIAPIAPListClientFunc(func(ctx context.Context, appID string, opts ...asc.IAPOption) (*asc.InAppPurchasesV2Response, error) {
			return &asc.InAppPurchasesV2Response{}, nil
		}), nil
	}
	resolveSessionFn = func(ctx context.Context, appleID, password, twoFactorCode string) (*webcore.AuthSession, string, error) {
		t.Fatal("web session should not resolve when IAP is outside the app")
		return nil, "", nil
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
