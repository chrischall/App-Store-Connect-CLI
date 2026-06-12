package alternativedistribution

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestAlternativeDistributionCommandIncludesAgreements(t *testing.T) {
	cmd := AlternativeDistributionCommand()
	if cmd == nil {
		t.Fatal("expected alternative-distribution command")
	}

	for _, sub := range cmd.Subcommands {
		if sub.Name == "agreements" {
			return
		}
	}

	t.Fatal("expected agreements subcommand")
}

func TestAlternativeDistributionAgreementsOpenCommandPrintsManualSigningHandoff(t *testing.T) {
	cmd := AlternativeDistributionAgreementsOpenCommand()
	if err := cmd.FlagSet.Parse([]string{}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	stdout := captureStdout(t, func() {
		if err := cmd.Exec(context.Background(), []string{}); err != nil {
			t.Fatalf("Exec() error: %v", err)
		}
	})

	if !strings.Contains(stdout, alternativeDistributionAgreementsURL) {
		t.Fatalf("expected stdout to include agreements URL, got %q", stdout)
	}
	if !strings.Contains(stdout, "does not accept or sign") {
		t.Fatalf("expected stdout to clarify manual signing, got %q", stdout)
	}
}

func TestAlternativeDistributionAgreementsOpenCommandRejectsUnsupportedAgreement(t *testing.T) {
	cmd := AlternativeDistributionAgreementsOpenCommand()
	if err := cmd.FlagSet.Parse([]string{"--agreement", "paid-apps"}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	if err := cmd.Exec(context.Background(), []string{}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp for unsupported agreement, got %v", err)
	}
}

func TestAlternativeDistributionAgreementsOpenCommandBrowserUsesAgreementsURL(t *testing.T) {
	var openedURL string
	prevOpen := openAgreementURL
	openAgreementURL = func(ctx context.Context, rawURL string) error {
		openedURL = rawURL
		return nil
	}
	t.Cleanup(func() {
		openAgreementURL = prevOpen
	})

	cmd := AlternativeDistributionAgreementsOpenCommand()
	if err := cmd.FlagSet.Parse([]string{"--browser"}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	_ = captureStdout(t, func() {
		if err := cmd.Exec(context.Background(), []string{}); err != nil {
			t.Fatalf("Exec() error: %v", err)
		}
	})

	if openedURL != alternativeDistributionAgreementsURL {
		t.Fatalf("opened URL = %q, want %q", openedURL, alternativeDistributionAgreementsURL)
	}
}

func TestOpenURLReturnsOpenerExitFailure(t *testing.T) {
	prevCommand := openURLCommand
	openURLCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, os.Args[0], "-test.run=TestOpenURLHelperProcess", "--")
	}
	t.Cleanup(func() {
		openURLCommand = prevCommand
	})

	if err := openURL(context.Background(), alternativeDistributionAgreementsURL); err == nil {
		t.Fatal("expected opener exit failure")
	}
}

func TestOpenURLHelperProcess(t *testing.T) {
	for _, arg := range os.Args {
		if arg == "--" {
			os.Exit(1)
		}
	}
}

func TestAlternativeDistributionDomainsGetCommand_MissingID(t *testing.T) {
	cmd := AlternativeDistributionDomainsGetCommand()
	if err := cmd.FlagSet.Parse([]string{}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	if err := cmd.Exec(context.Background(), []string{}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp when --domain-id is missing, got %v", err)
	}
}

func TestAlternativeDistributionDomainsCreateCommand_MissingDomain(t *testing.T) {
	cmd := AlternativeDistributionDomainsCreateCommand()
	if err := cmd.FlagSet.Parse([]string{"--reference-name", "Example"}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	if err := cmd.Exec(context.Background(), []string{}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp when --domain is missing, got %v", err)
	}
}

func TestAlternativeDistributionDomainsCreateCommand_MissingReferenceName(t *testing.T) {
	cmd := AlternativeDistributionDomainsCreateCommand()
	if err := cmd.FlagSet.Parse([]string{"--domain", "example.com"}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	if err := cmd.Exec(context.Background(), []string{}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp when --reference-name is missing, got %v", err)
	}
}

func TestAlternativeDistributionDomainsDeleteCommand_MissingID(t *testing.T) {
	cmd := AlternativeDistributionDomainsDeleteCommand()
	if err := cmd.FlagSet.Parse([]string{"--confirm"}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	if err := cmd.Exec(context.Background(), []string{}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp when --domain-id is missing, got %v", err)
	}
}

func TestAlternativeDistributionDomainsDeleteCommand_MissingConfirm(t *testing.T) {
	cmd := AlternativeDistributionDomainsDeleteCommand()
	if err := cmd.FlagSet.Parse([]string{"--domain-id", "DOMAIN_ID"}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	if err := cmd.Exec(context.Background(), []string{}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp when --confirm is missing, got %v", err)
	}
}

func TestAlternativeDistributionDomainsListCommand_InvalidLimit(t *testing.T) {
	cmd := AlternativeDistributionDomainsListCommand()
	if err := cmd.FlagSet.Parse([]string{"--limit", "500"}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	if err := cmd.Exec(context.Background(), []string{}); err == nil || errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected validation error for invalid --limit, got %v", err)
	}
}

func TestAlternativeDistributionKeysGetCommand_MissingID(t *testing.T) {
	cmd := AlternativeDistributionKeysGetCommand()
	if err := cmd.FlagSet.Parse([]string{}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	if err := cmd.Exec(context.Background(), []string{}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp when --key-id is missing, got %v", err)
	}
}

func TestAlternativeDistributionKeysCreateCommand_MissingApp(t *testing.T) {
	t.Setenv("ASC_APP_ID", "")

	cmd := AlternativeDistributionKeysCreateCommand()
	if err := cmd.FlagSet.Parse([]string{"--public-key", "KEY"}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	if err := cmd.Exec(context.Background(), []string{}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp when --app is missing, got %v", err)
	}
}

func TestAlternativeDistributionKeysCreateCommand_MissingKey(t *testing.T) {
	cmd := AlternativeDistributionKeysCreateCommand()
	if err := cmd.FlagSet.Parse([]string{"--app", "APP_ID"}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	if err := cmd.Exec(context.Background(), []string{}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp when key input is missing, got %v", err)
	}
}

func TestAlternativeDistributionKeysCreateCommand_ConflictingKeyInputs(t *testing.T) {
	cmd := AlternativeDistributionKeysCreateCommand()
	if err := cmd.FlagSet.Parse([]string{"--app", "APP_ID", "--public-key", "KEY", "--public-key-path", "./key.pem"}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	if err := cmd.Exec(context.Background(), []string{}); err == nil || errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected validation error for conflicting key inputs, got %v", err)
	}
}

func TestAlternativeDistributionKeysDeleteCommand_MissingID(t *testing.T) {
	cmd := AlternativeDistributionKeysDeleteCommand()
	if err := cmd.FlagSet.Parse([]string{"--confirm"}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	if err := cmd.Exec(context.Background(), []string{}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp when --key-id is missing, got %v", err)
	}
}

func TestAlternativeDistributionKeysDeleteCommand_MissingConfirm(t *testing.T) {
	cmd := AlternativeDistributionKeysDeleteCommand()
	if err := cmd.FlagSet.Parse([]string{"--key-id", "KEY_ID"}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	if err := cmd.Exec(context.Background(), []string{}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp when --confirm is missing, got %v", err)
	}
}

func TestAlternativeDistributionKeysAppCommand_MissingApp(t *testing.T) {
	t.Setenv("ASC_APP_ID", "")

	cmd := AlternativeDistributionKeysAppCommand()
	if err := cmd.FlagSet.Parse([]string{}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	if err := cmd.Exec(context.Background(), []string{}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp when --app is missing, got %v", err)
	}
}

func TestAlternativeDistributionPackagesGetCommand_MissingID(t *testing.T) {
	cmd := AlternativeDistributionPackagesGetCommand()
	if err := cmd.FlagSet.Parse([]string{}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	if err := cmd.Exec(context.Background(), []string{}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp when --package-id is missing, got %v", err)
	}
}

func TestAlternativeDistributionPackagesCreateCommand_MissingAppStoreVersionID(t *testing.T) {
	cmd := AlternativeDistributionPackagesCreateCommand()
	if err := cmd.FlagSet.Parse([]string{}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	if err := cmd.Exec(context.Background(), []string{}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp when --app-store-version-id is missing, got %v", err)
	}
}

func TestAlternativeDistributionPackagesAppStoreVersionCommand_MissingID(t *testing.T) {
	cmd := AlternativeDistributionPackagesAppStoreVersionCommand()
	if err := cmd.FlagSet.Parse([]string{}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	if err := cmd.Exec(context.Background(), []string{}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp when --app-store-version-id is missing, got %v", err)
	}
}

func TestAlternativeDistributionPackageVariantsCommand_MissingID(t *testing.T) {
	cmd := AlternativeDistributionPackageVariantsCommand()
	if err := cmd.FlagSet.Parse([]string{}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	if err := cmd.Exec(context.Background(), []string{}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp when --variant-id is missing, got %v", err)
	}
}

func TestAlternativeDistributionPackageDeltasCommand_MissingID(t *testing.T) {
	cmd := AlternativeDistributionPackageDeltasCommand()
	if err := cmd.FlagSet.Parse([]string{}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	if err := cmd.Exec(context.Background(), []string{}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp when --delta-id is missing, got %v", err)
	}
}

func TestAlternativeDistributionPackageVersionsGetCommand_MissingID(t *testing.T) {
	cmd := AlternativeDistributionPackageVersionsGetCommand()
	if err := cmd.FlagSet.Parse([]string{}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	if err := cmd.Exec(context.Background(), []string{}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp when --version-id is missing, got %v", err)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	originalStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}

	os.Stdout = w
	defer func() {
		os.Stdout = originalStdout
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close stdout pipe: %v", err)
	}

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stdout pipe: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close stdout reader: %v", err)
	}

	return string(data)
}

func TestAlternativeDistributionPackageVersionsListCommand_MissingID(t *testing.T) {
	cmd := AlternativeDistributionPackageVersionsListCommand()
	if err := cmd.FlagSet.Parse([]string{}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	if err := cmd.Exec(context.Background(), []string{}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp when --package-id is missing, got %v", err)
	}
}

func TestAlternativeDistributionPackageVersionsDeltasCommand_MissingID(t *testing.T) {
	cmd := AlternativeDistributionPackageVersionsDeltasCommand()
	if err := cmd.FlagSet.Parse([]string{}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	if err := cmd.Exec(context.Background(), []string{}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp when --version-id is missing, got %v", err)
	}
}

func TestAlternativeDistributionPackageVersionsVariantsCommand_MissingID(t *testing.T) {
	cmd := AlternativeDistributionPackageVersionsVariantsCommand()
	if err := cmd.FlagSet.Parse([]string{}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	if err := cmd.Exec(context.Background(), []string{}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp when --version-id is missing, got %v", err)
	}
}

func TestAlternativeDistributionPackageVersionsDeltasCommand_InvalidLimit(t *testing.T) {
	cmd := AlternativeDistributionPackageVersionsDeltasCommand()
	if err := cmd.FlagSet.Parse([]string{"--version-id", "VERSION_ID", "--limit", "1000"}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	if err := cmd.Exec(context.Background(), []string{}); err == nil || errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected validation error for invalid --limit, got %v", err)
	}
}

func TestAlternativeDistributionPackageVersionsListCommand_InvalidLimit(t *testing.T) {
	cmd := AlternativeDistributionPackageVersionsListCommand()
	if err := cmd.FlagSet.Parse([]string{"--package-id", "PACKAGE_ID", "--limit", "1000"}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	if err := cmd.Exec(context.Background(), []string{}); err == nil || errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected validation error for invalid --limit, got %v", err)
	}
}
