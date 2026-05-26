package web

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/peterbourgon/ff/v3/ffcli"
	webcore "github.com/rudrankriyam/App-Store-Connect-CLI/internal/web"
)

// WebReviewIAPsCommand groups iris-API operations that attach a non-renewing
// in-app purchase to the next app version review. Mirrors
// `asc web review subscriptions` but routes to the IAP iris endpoint.
//
// Apple's public REST API does not expose this — for the first IAP on an app,
// the only path is the App Store Connect web UI's "Add App In-App Purchase or
// Subscription" dialog (which POSTs to /iris/v1/inAppPurchaseSubmissions with
// submitWithNextAppStoreVersion=true). This command exposes that same flow
// programmatically so the first-time-IAP submission can be scripted alongside
// the version submission, instead of requiring a manual web click.
func WebReviewIAPsCommand() *ffcli.Command {
	fs := flag.NewFlagSet("web review iaps", flag.ExitOnError)
	return &ffcli.Command{
		Name:       "iaps",
		ShortUsage: "asc web review iaps <subcommand> [flags]",
		ShortHelp:  "[experimental] Attach non-renewing IAPs to the next app version review.",
		FlagSet:    fs,
		Subcommands: []*ffcli.Command{
			webReviewIAPsAttachCommand(),
		},
		Exec: func(ctx context.Context, args []string) error {
			return fmt.Errorf("subcommand required (try: attach)")
		},
	}
}

func webReviewIAPsAttachCommand() *ffcli.Command {
	fs := flag.NewFlagSet("web review iaps attach", flag.ExitOnError)
	appID := fs.String("app", "", "App ID")
	iapID := fs.String("iap-id", "", "Non-renewing IAP ID (or product ID)")
	confirm := fs.Bool("confirm", false, "Confirm the attach")
	authFlags := bindWebSessionFlags(fs)
	return &ffcli.Command{
		Name:       "attach",
		ShortUsage: "asc web review iaps attach --app APP_ID --iap-id IAP_ID --confirm [flags]",
		ShortHelp:  "[experimental] Attach a non-renewing IAP to the next app version review.",
		FlagSet:    fs,
		Exec: func(ctx context.Context, args []string) error {
			a := strings.TrimSpace(*appID)
			i := strings.TrimSpace(*iapID)
			switch {
			case a == "":
				return fmt.Errorf("--app is required")
			case i == "":
				return fmt.Errorf("--iap-id is required")
			case !*confirm:
				return fmt.Errorf("--confirm is required")
			}

			session, err := resolveWebSessionForCommand(ctx, authFlags)
			if err != nil {
				return err
			}
			client := newWebClientFn(session)

			sub, err := client.CreateInAppPurchaseSubmission(ctx, i)
			if err != nil {
				return withWebAuthHint(err, "web review iaps attach")
			}

			out := map[string]any{
				"appId":      a,
				"iapId":      i,
				"submission": sub,
				"changed":    true,
				"operation":  "attach",
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		},
	}
}

var _ = (*webcore.Client)(nil)
