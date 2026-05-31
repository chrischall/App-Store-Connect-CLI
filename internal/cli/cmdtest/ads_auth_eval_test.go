package cmdtest

import (
	"encoding/json"
	"errors"
	"flag"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdsAuthEvalWorkflow(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	keyPath := filepath.Join(t.TempDir(), "apple-ads-private-key.pem")
	writeECDSAPEM(t, keyPath)

	t.Setenv("ASC_CONFIG_PATH", configPath)
	t.Setenv("ASC_ADS_BYPASS_KEYCHAIN", "1")

	stdout, stderr, err := runAdsEvalCommand(
		t,
		"ads", "auth", "login",
		"--bypass-keychain",
		"--name", "Marketing",
		"--client-id", "SEARCHADS.CLIENT",
		"--team-id", "SEARCHADS.TEAM",
		"--key-id", "KEY_ID",
		"--private-key", keyPath,
		"--org", "987654",
	)
	if err != nil {
		t.Fatalf("login error: %v\nstderr: %s", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("login stderr = %q, want empty", stderr)
	}
	if !strings.Contains(stdout, "Successfully registered Apple Ads API key 'Marketing'") {
		t.Fatalf("login stdout = %q", stdout)
	}

	stdout, stderr, err = runAdsEvalCommand(t, "ads", "auth", "status", "--output", "json")
	if err != nil {
		t.Fatalf("status error: %v\nstderr: %s", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("status stderr = %q, want empty", stderr)
	}
	var status struct {
		Storage     string `json:"storage"`
		Credentials []struct {
			Name     string `json:"name"`
			ClientID string `json:"client_id"`
			OrgID    string `json:"org_id"`
			Default  bool   `json:"default"`
			Source   string `json:"source"`
		} `json:"credentials"`
	}
	if err := json.Unmarshal([]byte(stdout), &status); err != nil {
		t.Fatalf("status stdout is not JSON: %v\n%s", err, stdout)
	}
	if status.Storage != "Config File" || len(status.Credentials) != 1 {
		t.Fatalf("status = %+v, want one config credential", status)
	}
	got := status.Credentials[0]
	if got.Name != "Marketing" || got.ClientID != "SEARCHADS.CLIENT" || got.OrgID != "987654" || !got.Default || got.Source != "config" {
		t.Fatalf("credential = %+v, want stored Marketing profile", got)
	}

	stdout, stderr, err = runAdsEvalCommand(t, "ads", "auth", "switch", "--name", "Marketing")
	if err != nil {
		t.Fatalf("switch error: %v\nstderr: %s", err, stderr)
	}
	if stderr != "" || !strings.Contains(stdout, "Default Apple Ads profile set to 'Marketing'") {
		t.Fatalf("switch stdout = %q stderr = %q", stdout, stderr)
	}

	stdout, stderr, err = runAdsEvalCommand(t, "ads", "auth", "logout", "--name", "Marketing")
	if err != nil {
		t.Fatalf("logout error: %v\nstderr: %s", err, stderr)
	}
	if stderr != "" || !strings.Contains(stdout, "Successfully removed Apple Ads credential 'Marketing'") {
		t.Fatalf("logout stdout = %q stderr = %q", stdout, stderr)
	}
}

func TestAdsAuthEvalValidatesUsageErrors(t *testing.T) {
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	t.Setenv("ASC_ADS_BYPASS_KEYCHAIN", "1")

	_, stderr, err := runAdsEvalCommand(t, "ads", "auth", "login")
	if !errors.Is(err, flag.ErrHelp) || !strings.Contains(stderr, "--name is required") {
		t.Fatalf("login error = %v stderr = %q, want missing name usage error", err, stderr)
	}

	_, stderr, err = runAdsEvalCommand(t, "ads", "auth", "login", "--local", "--name", "Marketing")
	if !errors.Is(err, flag.ErrHelp) || !strings.Contains(stderr, "--client-id is required") {
		t.Fatalf("login partial error = %v stderr = %q, want missing client ID usage error", err, stderr)
	}

	_, stderr, err = runAdsEvalCommand(t, "ads", "auth", "status", "--output", "markdown")
	if !errors.Is(err, flag.ErrHelp) || !strings.Contains(stderr, "unsupported format: markdown") {
		t.Fatalf("status error = %v stderr = %q, want invalid output usage error", err, stderr)
	}

	_, stderr, err = runAdsEvalCommand(t, "ads", "auth", "status", "--output", "json", "unexpected")
	if !errors.Is(err, flag.ErrHelp) || !strings.Contains(stderr, "unexpected argument(s): unexpected") {
		t.Fatalf("status args error = %v stderr = %q, want unexpected argument usage error", err, stderr)
	}

	_, stderr, err = runAdsEvalCommand(t, "ads", "auth", "logout", "--all", "--name", "Marketing")
	if !errors.Is(err, flag.ErrHelp) || !strings.Contains(stderr, "--all and --name are mutually exclusive") {
		t.Fatalf("logout error = %v stderr = %q, want mutually exclusive usage error", err, stderr)
	}

	_, stderr, err = runAdsEvalCommand(t, "ads", "auth", "logout", "--all")
	if !errors.Is(err, flag.ErrHelp) || !strings.Contains(stderr, "--all requires --confirm") {
		t.Fatalf("logout error = %v stderr = %q, want confirm usage error", err, stderr)
	}

	_, stderr, err = runAdsEvalCommand(t, "ads", "auth", "logout")
	if !errors.Is(err, flag.ErrHelp) || !strings.Contains(stderr, "provide --name or --all") {
		t.Fatalf("logout error = %v stderr = %q, want explicit target usage error", err, stderr)
	}
}

func TestAdsAuthTokenEvalRequiresConfirm(t *testing.T) {
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	t.Setenv("ASC_ADS_ACCESS_TOKEN", "ACCESS")

	_, stderr, err := runAdsEvalCommand(t, "ads", "auth", "token", "--output", "json")
	if !errors.Is(err, flag.ErrHelp) || !strings.Contains(stderr, "--confirm is required") {
		t.Fatalf("token error = %v stderr = %q, want confirm usage error", err, stderr)
	}

	stdout, stderr, err := runAdsEvalCommand(t, "ads", "auth", "token", "--confirm", "--output", "json")
	if err != nil {
		t.Fatalf("token error: %v\nstderr: %s", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("token stderr = %q, want empty", stderr)
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("token stdout is not JSON: %v\n%s", err, stdout)
	}
	if parsed.AccessToken != "ACCESS" {
		t.Fatalf("access_token = %q, want ACCESS", parsed.AccessToken)
	}
}
