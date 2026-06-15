package cmdtest

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	rootcmd "github.com/rudrankriyam/App-Store-Connect-CLI/cmd"
	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/cli/offercodes"
)

func TestLegacyOfferCodesCreateFreeTrialAcceptsTerritoryPrice(t *testing.T) {
	setupAuth(t)
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "nonexistent.json"))

	originalTransport := http.DefaultTransport
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
	})

	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", req.Method)
		}
		if req.URL.Path != "/v1/subscriptionOfferCodes" {
			t.Fatalf("expected path /v1/subscriptionOfferCodes, got %s", req.URL.Path)
		}

		rawBody, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read body error: %v", err)
		}

		var payload map[string]any
		if err := json.Unmarshal(rawBody, &payload); err != nil {
			t.Fatalf("decode request body: %v\nbody=%s", err, string(rawBody))
		}

		included := payload["included"].([]any)
		if len(included) != 1 {
			t.Fatalf("expected one included price, got %d", len(included))
		}
		relationships := included[0].(map[string]any)["relationships"].(map[string]any)
		territory := relationships["territory"].(map[string]any)["data"].(map[string]any)
		if territory["id"] != "USA" {
			t.Fatalf("expected normalized territory USA, got %#v", territory["id"])
		}
		if _, ok := relationships["subscriptionPricePoint"]; ok {
			t.Fatalf("expected subscriptionPricePoint to be omitted, got %#v", relationships["subscriptionPricePoint"])
		}

		body := `{"data":{"type":"subscriptionOfferCodes","id":"legacy-free-trial","attributes":{"name":"Legacy Free Trial","active":true}}}`
		return &http.Response{
			StatusCode: http.StatusCreated,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}, nil
	})

	command := offercodes.OfferCodesCreateCommand()
	command.FlagSet.SetOutput(io.Discard)

	stdout, stderr := captureOutput(t, func() {
		if err := command.Parse([]string{
			"--subscription-id", "8000000001",
			"--name", "Legacy Free Trial",
			"--customer-eligibilities", "NEW",
			"--offer-eligibility", "STACK_WITH_INTRO_OFFERS",
			"--duration", "ONE_MONTH",
			"--offer-mode", "FREE_TRIAL",
			"--number-of-periods", "1",
			"--prices", "us",
		}); err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if err := command.Run(context.Background()); err != nil {
			t.Fatalf("run error: %v", err)
		}
	})

	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	var output struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatalf("decode output: %v\nstdout=%s", err, stdout)
	}
	if output.Data.ID != "legacy-free-trial" {
		t.Fatalf("expected created offer code id legacy-free-trial, got %q", output.Data.ID)
	}
}

func TestLegacyOfferCodesCreateInvalidFreeTrialPriceReturnsUsageExitCode(t *testing.T) {
	command := offercodes.OfferCodesCreateCommand()
	command.FlagSet.SetOutput(io.Discard)

	var runErr error
	stdout, stderr := captureOutput(t, func() {
		if err := command.Parse([]string{
			"--subscription-id", "8000000001",
			"--name", "Legacy Free Trial",
			"--customer-eligibilities", "NEW",
			"--offer-eligibility", "STACK_WITH_INTRO_OFFERS",
			"--duration", "ONE_MONTH",
			"--offer-mode", "FREE_TRIAL",
			"--number-of-periods", "1",
			"--prices", "USA:PRICE_POINT_ID",
		}); err != nil {
			t.Fatalf("parse error: %v", err)
		}
		runErr = command.Run(context.Background())
	})

	if got := rootcmd.ExitCodeFromError(runErr); got != rootcmd.ExitUsage {
		t.Fatalf("expected exit code %d, got %d from %v", rootcmd.ExitUsage, got, runErr)
	}
	if stdout != "" {
		t.Fatalf("expected empty stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "Error: --prices for FREE_TRIAL must use TERRITORY entries without price point IDs") {
		t.Fatalf("expected FREE_TRIAL price validation in stderr, got %q", stderr)
	}
}
