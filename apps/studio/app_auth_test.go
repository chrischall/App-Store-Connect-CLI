package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCacheAuthFromConfigUsesActiveConfigPath(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "custom-config.json")
	if err := os.WriteFile(configPath, []byte(`{
		"default_key_name":"custom",
		"keys":[{"name":"custom","key_id":"KEY123","issuer_id":"ISS456","private_key_path":"/tmp/custom.p8"}]
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("ASC_CONFIG_PATH", configPath)

	app := &App{}
	app.cacheAuthFromConfig()

	if app.cachedKeyID != "KEY123" {
		t.Fatalf("cachedKeyID = %q, want KEY123", app.cachedKeyID)
	}
	if app.cachedIssuerID != "ISS456" {
		t.Fatalf("cachedIssuerID = %q, want ISS456", app.cachedIssuerID)
	}
	if app.cachedPrivateKeyPath != "/tmp/custom.p8" {
		t.Fatalf("cachedPrivateKeyPath = %q, want /tmp/custom.p8", app.cachedPrivateKeyPath)
	}
}
