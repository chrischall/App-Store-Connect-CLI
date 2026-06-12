package alternativedistribution

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/peterbourgon/ff/v3/ffcli"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/cli/shared"
)

const (
	alternativeDistributionEUAddendumAgreement = "eu-addendum"
	alternativeDistributionAgreementsURL       = "https://appstoreconnect.apple.com/agreements/#/"
)

var openAgreementURL = openURL
var openURLCommand = exec.CommandContext

// AlternativeDistributionAgreementsCommand returns the agreements command group.
func AlternativeDistributionAgreementsCommand() *ffcli.Command {
	fs := flag.NewFlagSet("agreements", flag.ExitOnError)

	return &ffcli.Command{
		Name:       "agreements",
		ShortUsage: "asc alternative-distribution agreements <subcommand> [flags]",
		ShortHelp:  "Open App Store Connect agreements for alternative distribution.",
		LongHelp: `Open App Store Connect agreements for alternative distribution.

This command does not accept or sign any agreement. It only points the account
holder to App Store Connect so they can review and sign the EU addendum when
Apple requires it.

Examples:
  asc alternative-distribution agreements open
  asc alternative-distribution agreements open --browser`,
		FlagSet:   fs,
		UsageFunc: shared.DefaultUsageFunc,
		Subcommands: []*ffcli.Command{
			AlternativeDistributionAgreementsOpenCommand(),
		},
		Exec: func(ctx context.Context, args []string) error {
			return flag.ErrHelp
		},
	}
}

// AlternativeDistributionAgreementsOpenCommand returns the agreements open subcommand.
func AlternativeDistributionAgreementsOpenCommand() *ffcli.Command {
	fs := flag.NewFlagSet("open", flag.ExitOnError)

	agreement := fs.String("agreement", alternativeDistributionEUAddendumAgreement, "Agreement to open: eu-addendum")
	browser := fs.Bool("browser", false, "Open the agreement page in the default browser")

	return &ffcli.Command{
		Name:       "open",
		ShortUsage: "asc alternative-distribution agreements open [--agreement eu-addendum] [--browser]",
		ShortHelp:  "Show the App Store Connect EU addendum agreements page.",
		LongHelp: `Show the App Store Connect EU addendum agreements page.

This command does not accept or sign any agreement. It prints the App Store
Connect Agreements page where the account holder can review and sign the
Alternative Distribution Addendum for EU Apps when Apple requires it.

Examples:
  asc alternative-distribution agreements open
  asc alternative-distribution agreements open --browser`,
		FlagSet:   fs,
		UsageFunc: shared.DefaultUsageFunc,
		Exec: func(ctx context.Context, args []string) error {
			if strings.TrimSpace(*agreement) != alternativeDistributionEUAddendumAgreement {
				fmt.Fprintf(os.Stderr, "Error: --agreement must be %q\n", alternativeDistributionEUAddendumAgreement)
				return flag.ErrHelp
			}

			fmt.Fprintln(os.Stdout, "Open App Store Connect Agreements to review and sign the Alternative Distribution Addendum for EU Apps:")
			fmt.Fprintln(os.Stdout, alternativeDistributionAgreementsURL)
			fmt.Fprintln(os.Stdout, "This command does not accept or sign the agreement for you.")

			if *browser {
				if err := openAgreementURL(ctx, alternativeDistributionAgreementsURL); err != nil {
					return fmt.Errorf("alternative-distribution agreements open: failed to open browser: %w", err)
				}
			}

			return nil
		},
	}
}

func openURL(ctx context.Context, rawURL string) error {
	var command string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		command = "open"
		args = []string{rawURL}
	case "windows":
		command = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", rawURL}
	default:
		command = "xdg-open"
		args = []string{rawURL}
	}

	// #nosec G204 -- command is selected from a fixed platform-specific allowlist.
	return openURLCommand(ctx, command, args...).Run()
}
