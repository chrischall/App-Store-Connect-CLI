package cmdtest

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func TestDevicesRegisterReusesExistingDeviceWithNormalizedUDID(t *testing.T) {
	setupAuth(t)
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "nonexistent.json"))

	originalTransport := http.DefaultTransport
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
	})

	requestCount := 0
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requestCount++
		switch requestCount {
		case 1:
			if req.Method != http.MethodGet {
				t.Fatalf("expected GET before create, got %s", req.Method)
			}
			if req.URL.Path != "/v1/devices" {
				t.Fatalf("expected devices list path, got %s", req.URL.Path)
			}
			query := req.URL.Query()
			if got := query.Get("filter[platform]"); got != "IOS" {
				t.Fatalf("expected filter[platform]=IOS, got %q", got)
			}
			body := `{"data":[{"type":"devices","id":"device-existing","attributes":{"name":"Existing iPhone","platform":"IOS","udid":"ABC-123-DEF","status":"ENABLED"}}],"links":{"next":""}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		default:
			t.Fatalf("expected register to reuse existing device without POST, got request %d: %s %s", requestCount, req.Method, req.URL.String())
			return nil, nil
		}
	})

	root := RootCommand("1.2.3")
	root.FlagSet.SetOutput(io.Discard)

	stdout, stderr := captureOutput(t, func() {
		if err := root.Parse([]string{"devices", "register", "--name", "Existing iPhone", "--udid", "ABC123DEF", "--platform", "IOS"}); err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if err := root.Run(context.Background()); err != nil {
			t.Fatalf("run error: %v", err)
		}
	})

	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, `"id":"device-existing"`) {
		t.Fatalf("expected existing device output, got %q", stdout)
	}
}
