package storekit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/99designs/keyring"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/config"
)

func TestStoredCredentialNeverMarshalsPrivateKeyMaterial(t *testing.T) {
	stored := StoredCredential{
		Credentials: Credentials{PrivateKeyPEM: "TOP-SECRET-PEM", PrivateKeyPath: "/secret/key.p8"},
		Name:        "Primary",
		Source:      "keychain",
	}
	data, err := json.Marshal(stored)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if string(data) == "" || containsAny(string(data), "TOP-SECRET-PEM", "/secret/key.p8", "PrivateKeyPEM") {
		t.Fatalf("marshaled credential leaked private key material: %s", data)
	}
}

func TestConfigCredentialLifecycle(t *testing.T) {
	t.Setenv("ASC_STOREKIT_BYPASS_KEYCHAIN", "1")
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("ASC_CONFIG_PATH", path)
	credentials := testCredentials(t)
	keyPath := filepath.Join(t.TempDir(), "AuthKey.p8")
	if err := os.WriteFile(keyPath, []byte(credentials.PrivateKeyPEM), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	credentials.PrivateKeyPath = keyPath
	credentials.PrivateKeyPEM = ""

	if err := StoreCredentialsConfig("Primary", credentials); err != nil {
		t.Fatalf("StoreCredentialsConfig() error = %v", err)
	}
	stored, source, err := GetCredentialsWithSource("")
	if err != nil {
		t.Fatalf("GetCredentialsWithSource() error = %v", err)
	}
	if source != "config" || stored.KeyID != credentials.KeyID || stored.BundleID != credentials.BundleID {
		t.Fatalf("stored = %#v source=%q", stored, source)
	}
	if err := SetDefaultCredentials("Primary"); err != nil {
		t.Fatalf("SetDefaultCredentials() error = %v", err)
	}
	if err := RemoveCredentials("Primary"); err != nil {
		t.Fatalf("RemoveCredentials() error = %v", err)
	}
	if _, _, err := GetCredentialsWithSource(""); err == nil {
		t.Fatal("expected missing default credentials error")
	}
}

func TestGetCredentialsPrefersActiveConfigOverSameNamedKeychainProfile(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("ASC_CONFIG_PATH", configPath)
	configCredentials := testCredentials(t)
	configCredentials.KeyID = "CONFIG_KEY"
	configCredentials.PrivateKeyPath = "/config/key.p8"
	configCredentials.PrivateKeyPEM = ""
	if err := StoreCredentialsConfigAt("shared", configCredentials, configPath); err != nil {
		t.Fatalf("StoreCredentialsConfigAt() error = %v", err)
	}

	payload, err := json.Marshal(credentialPayload{
		KeyID: "KEYCHAIN_KEY", IssuerID: "KEYCHAIN_ISSUER", PrivateKeyPEM: "keychain-pem", BundleID: "com.keychain.app",
	})
	if err != nil {
		t.Fatal(err)
	}
	original := openStoreKitKeyring
	openStoreKitKeyring = func() (keyring.Keyring, error) {
		return keyring.NewArrayKeyring([]keyring.Item{{Key: storeKitKeyringItemPrefix + "shared", Data: payload}}), nil
	}
	t.Cleanup(func() { openStoreKitKeyring = original })

	credentials, source, err := GetCredentialsWithSource("shared")
	if err != nil {
		t.Fatalf("GetCredentialsWithSource() error = %v", err)
	}
	if source != "config" || credentials.KeyID != "CONFIG_KEY" {
		t.Fatalf("credentials = %#v source=%q", credentials, source)
	}
}

func TestGetCredentialsFallsBackToGlobalWhenLocalConfigHasNoStoreKitKeys(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ASC_CONFIG_PATH", "")
	t.Setenv("ASC_STOREKIT_BYPASS_KEYCHAIN", "1")
	globalPath, err := config.GlobalPath()
	if err != nil {
		t.Fatal(err)
	}
	credentials := testCredentials(t)
	credentials.PrivateKeyPath = "/global/SubscriptionKey.p8"
	credentials.PrivateKeyPEM = ""
	if err := StoreCredentialsConfigAt("global", credentials, globalPath); err != nil {
		t.Fatalf("StoreCredentialsConfigAt(global) error = %v", err)
	}

	projectDir := t.TempDir()
	localPath := filepath.Join(projectDir, ".asc", "config.json")
	if err := config.SaveAt(localPath, &config.Config{AppID: "local-app"}); err != nil {
		t.Fatalf("SaveAt(local) error = %v", err)
	}
	t.Chdir(projectDir)

	resolved, source, err := GetCredentialsWithSource("global")
	if err != nil {
		t.Fatalf("GetCredentialsWithSource(global) error = %v", err)
	}
	if source != "config" || resolved.KeyID != credentials.KeyID || resolved.Profile != "global" {
		t.Fatalf("credentials = %#v source=%q", resolved, source)
	}
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}
