package web

import (
	"context"
	"flag"
	"fmt"
	"strconv"
	"strings"

	"github.com/peterbourgon/ff/v3/ffcli"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/asc"
	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/cli/shared"
	webcore "github.com/rudrankriyam/App-Store-Connect-CLI/internal/web"
)

type reviewIAPMutationOutput struct {
	AppID      string                      `json:"appId"`
	IAPID      string                      `json:"iapId"`
	Operation  string                      `json:"operation"`
	Changed    bool                        `json:"changed"`
	Submission webcore.ReviewIAPSubmission `json:"submission"`
}

func reviewIAPValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "n/a"
	}
	return trimmed
}

func buildReviewIAPMutationRows(payload reviewIAPMutationOutput) [][]string {
	return [][]string{
		{"Mutation", "App ID", reviewIAPValue(payload.AppID)},
		{"Mutation", "IAP ID", reviewIAPValue(payload.IAPID)},
		{"Mutation", "Operation", reviewIAPValue(payload.Operation)},
		{"Mutation", "Changed", strconv.FormatBool(payload.Changed)},
		{"Submission", "Submission ID", reviewIAPValue(payload.Submission.ID)},
		{"Submission", "Next Version", strconv.FormatBool(payload.Submission.SubmitWithNextAppStoreVersion)},
	}
}

func renderReviewIAPMutationTable(payload reviewIAPMutationOutput) error {
	headers := []string{"Section", "Field", "Value"}
	asc.RenderTable(headers, buildReviewIAPMutationRows(payload))
	return nil
}

func renderReviewIAPMutationMarkdown(payload reviewIAPMutationOutput) error {
	headers := []string{"Section", "Field", "Value"}
	asc.RenderMarkdown(headers, buildReviewIAPMutationRows(payload))
	return nil
}

// WebReviewIAPsCommand groups iris-API operations that attach a non-renewing
// in-app purchase to the next app version review. Mirrors
// `asc web review subscriptions` but routes to the IAP iris endpoint.
func WebReviewIAPsCommand() *ffcli.Command {
	fs := flag.NewFlagSet("web review iaps", flag.ExitOnError)
	return &ffcli.Command{
		Name:       "iaps",
		ShortUsage: "asc web review iaps <subcommand> [flags]",
		ShortHelp:  "[experimental] Attach non-renewing IAPs to the next app version review.",
		LongHelp: `EXPERIMENTAL / UNOFFICIAL / DISCOURAGED

Attach a non-renewing in-app purchase to the next app version review. This
uses private Apple web-session /iris endpoints and may break without notice.

Subcommands:
  attach  Attach one non-renewing IAP to the next app version review

Apple's public REST API does not expose this flow for non-renewing purchases:
POST /v1/reviewSubmissionItems rejects all IAP relationship variants, and a
standalone POST /v1/inAppPurchaseSubmissions returns
FIRST_IAP_MUST_BE_SUBMITTED_ON_VERSION. The web UI's "Add App In-App Purchase
or Subscription" dialog posts to /iris/v1/inAppPurchaseSubmissions with
submitWithNextAppStoreVersion=true; this command exposes that same call.

` + webWarningText,
		FlagSet:   fs,
		UsageFunc: shared.DefaultUsageFunc,
		Subcommands: []*ffcli.Command{
			WebReviewIAPsAttachCommand(),
		},
		Exec: func(ctx context.Context, args []string) error {
			return flag.ErrHelp
		},
	}
}

// WebReviewIAPsAttachCommand attaches a non-renewing IAP to the next app
// version review via the private iris endpoint.
//
// The iris attach call itself accepts only the IAP ID — non-renewing IAPs are
// globally addressable within a developer account, unlike subscriptions which
// are grouped. --app is still required here for symmetry with
// `asc web review subscriptions attach` and so the JSON payload and error
// messages carry app context. A future ListReviewIAPs helper would let us
// pre-verify the IAP's state (READY_TO_SUBMIT etc.) the way subscriptions does.
func WebReviewIAPsAttachCommand() *ffcli.Command {
	fs := flag.NewFlagSet("web review iaps attach", flag.ExitOnError)

	appID := fs.String("app", "", "App ID (used for output and error context)")
	iapID := fs.String("iap-id", "", "Non-renewing IAP ID")
	confirm := fs.Bool("confirm", false, "Confirm the attach operation")
	authFlags := bindWebSessionFlags(fs)
	output := shared.BindOutputFlags(fs)

	return &ffcli.Command{
		Name:       "attach",
		ShortUsage: "asc web review iaps attach --app APP_ID --iap-id IAP_ID --confirm [flags]",
		ShortHelp:  "[experimental] Attach a non-renewing IAP to the next app version review.",
		FlagSet:    fs,
		UsageFunc:  shared.DefaultUsageFunc,
		Exec: func(ctx context.Context, args []string) error {
			trimmedAppID := strings.TrimSpace(*appID)
			trimmedIAPID := strings.TrimSpace(*iapID)
			switch {
			case trimmedAppID == "":
				return shared.UsageError("--app is required")
			case trimmedIAPID == "":
				return shared.UsageError("--iap-id is required")
			case !*confirm:
				return shared.UsageError("--confirm is required")
			}

			requestCtx, cancel := shared.ContextWithTimeout(ctx)
			defer cancel()

			session, err := resolveWebSessionForCommand(requestCtx, authFlags)
			if err != nil {
				return err
			}
			client := newWebClientFn(session)

			submission, err := withWebSpinnerValue("Attaching IAP to next app version", func() (webcore.ReviewIAPSubmission, error) {
				return client.CreateInAppPurchaseSubmission(requestCtx, trimmedIAPID)
			})
			if err != nil {
				return withWebAuthHint(
					fmt.Errorf("web review iaps attach for app %q, iap %q: %w", trimmedAppID, trimmedIAPID, err),
					"web review iaps attach",
				)
			}

			payload := reviewIAPMutationOutput{
				AppID:      trimmedAppID,
				IAPID:      trimmedIAPID,
				Operation:  "attach",
				Changed:    submission.SubmitWithNextAppStoreVersion,
				Submission: submission,
			}
			return shared.PrintOutputWithRenderers(
				payload,
				*output.Output,
				*output.Pretty,
				func() error { return renderReviewIAPMutationTable(payload) },
				func() error { return renderReviewIAPMutationMarkdown(payload) },
			)
		},
	}
}
