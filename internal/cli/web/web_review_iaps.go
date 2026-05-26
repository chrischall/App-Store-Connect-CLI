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

type reviewIAPIAPListClient interface {
	GetInAppPurchasesV2(ctx context.Context, appID string, opts ...asc.IAPOption) (*asc.InAppPurchasesV2Response, error)
}

var newReviewIAPASCClientFn = func() (reviewIAPIAPListClient, error) {
	return shared.GetASCClient()
}

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

func validateReviewIAPAttachInputs(appID, iapID string, confirm bool) error {
	switch {
	case strings.TrimSpace(appID) == "":
		return shared.UsageError("--app is required")
	case shared.SelectorNeedsLookup(appID):
		return shared.UsageError("--app must be a numeric App Store Connect app ID")
	case strings.TrimSpace(iapID) == "":
		return shared.UsageError("--iap-id is required")
	case shared.SelectorNeedsLookup(iapID):
		return shared.UsageError("--iap-id must be a numeric App Store Connect in-app purchase ID")
	case !confirm:
		return shared.UsageError("--confirm is required")
	default:
		return nil
	}
}

func verifyReviewIAPBelongsToApp(ctx context.Context, client reviewIAPIAPListClient, appID, iapID string) error {
	if client == nil {
		return fmt.Errorf("app-scoped IAP verification client is required")
	}

	resp, err := client.GetInAppPurchasesV2(ctx, appID, asc.WithIAPLimit(200))
	if err != nil {
		return fmt.Errorf("verify in-app purchase %q under app %q: %w", iapID, appID, err)
	}

	found := false
	if err := asc.PaginateEach(
		ctx,
		resp,
		func(ctx context.Context, nextURL string) (asc.PaginatedResponse, error) {
			return client.GetInAppPurchasesV2(ctx, appID, asc.WithIAPNextURL(nextURL))
		},
		func(page asc.PaginatedResponse) error {
			iaps, ok := page.(*asc.InAppPurchasesV2Response)
			if !ok {
				return fmt.Errorf("unexpected in-app purchases pagination type %T", page)
			}
			for _, iap := range iaps.Data {
				if strings.TrimSpace(iap.ID) == iapID {
					found = true
					return nil
				}
			}
			return nil
		},
	); err != nil {
		return fmt.Errorf("verify in-app purchase %q under app %q: %w", iapID, appID, err)
	}
	if !found {
		return fmt.Errorf("in-app purchase %q was not found under app %q; refusing to attach", iapID, appID)
	}
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
func WebReviewIAPsAttachCommand() *ffcli.Command {
	fs := flag.NewFlagSet("web review iaps attach", flag.ExitOnError)

	appID := fs.String("app", "", "App ID")
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
			if err := validateReviewIAPAttachInputs(trimmedAppID, trimmedIAPID, *confirm); err != nil {
				return err
			}

			requestCtx, cancel := shared.ContextWithTimeout(ctx)
			defer cancel()

			ascClient, err := newReviewIAPASCClientFn()
			if err != nil {
				return fmt.Errorf("web review iaps attach: app-scoped IAP verification requires App Store Connect API credentials: %w", err)
			}
			if err := verifyReviewIAPBelongsToApp(requestCtx, ascClient, trimmedAppID, trimmedIAPID); err != nil {
				return fmt.Errorf("web review iaps attach: %w", err)
			}

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
