package web

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/peterbourgon/ff/v3/ffcli"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/asc"
	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/cli/shared"
	webcore "github.com/rudrankriyam/App-Store-Connect-CLI/internal/web"
)

var (
	createWebSubscriptionPlanAvailabilityFn = func(ctx context.Context, client *webcore.Client, subscriptionID, planType string, territoryIDs []string, availableInNewTerritories bool) (*webcore.SubscriptionPlanAvailability, error) {
		return client.CreateSubscriptionPlanAvailability(ctx, subscriptionID, planType, territoryIDs, availableInNewTerritories)
	}
	createWebSubscriptionPlanPricesFn = func(ctx context.Context, client *webcore.Client, subscriptionID, upfrontPricePointID, monthlyPricePointID string) (*webcore.SubscriptionPlanPricesResult, error) {
		return client.CreateSubscriptionPlanPrices(ctx, subscriptionID, upfrontPricePointID, monthlyPricePointID)
	}
)

type webSubscriptionMonthlyCommitmentBootstrapResult struct {
	SubscriptionID      string `json:"subscriptionId"`
	Territory           string `json:"territory"`
	PlanAvailabilityID  string `json:"planAvailabilityId"`
	PlanAvailabilityNew bool   `json:"planAvailabilityCreated"`
	UpfrontPricePointID string `json:"upfrontPricePointId"`
	MonthlyPricePointID string `json:"monthlyPricePointId"`
	PricesCreated       bool   `json:"pricesCreated"`
}

// WebSubscriptionsPricingCommand returns the web subscription pricing command group.
func WebSubscriptionsPricingCommand() *ffcli.Command {
	fs := flag.NewFlagSet("web subscriptions pricing", flag.ExitOnError)

	return &ffcli.Command{
		Name:       "pricing",
		ShortUsage: "asc web subscriptions pricing <subcommand> [flags]",
		ShortHelp:  "[experimental] Manage subscription pricing via web sessions.",
		LongHelp: `EXPERIMENTAL / UNOFFICIAL / DISCOURAGED

Manage subscription pricing through Apple's internal web API.

` + webWarningText,
		FlagSet:   fs,
		UsageFunc: shared.DefaultUsageFunc,
		Subcommands: []*ffcli.Command{
			WebSubscriptionsPricingMonthlyCommitmentCommand(),
		},
		Exec: func(ctx context.Context, args []string) error {
			return flag.ErrHelp
		},
	}
}

// WebSubscriptionsPricingMonthlyCommitmentCommand returns the monthly commitment group.
func WebSubscriptionsPricingMonthlyCommitmentCommand() *ffcli.Command {
	fs := flag.NewFlagSet("web subscriptions pricing monthly-commitment", flag.ExitOnError)

	return &ffcli.Command{
		Name:       "monthly-commitment",
		ShortUsage: "asc web subscriptions pricing monthly-commitment <subcommand> [flags]",
		ShortHelp:  "[experimental] Bootstrap monthly-with-commitment pricing.",
		LongHelp: `EXPERIMENTAL / UNOFFICIAL / DISCOURAGED

Bootstrap monthly-with-12-month-commitment pricing through Apple's internal web API.

` + webWarningText,
		FlagSet:   fs,
		UsageFunc: shared.DefaultUsageFunc,
		Subcommands: []*ffcli.Command{
			WebSubscriptionsPricingMonthlyCommitmentBootstrapCommand(),
		},
		Exec: func(ctx context.Context, args []string) error {
			return flag.ErrHelp
		},
	}
}

// WebSubscriptionsPricingMonthlyCommitmentBootstrapCommand creates availability and prices.
func WebSubscriptionsPricingMonthlyCommitmentBootstrapCommand() *ffcli.Command {
	fs := flag.NewFlagSet("web subscriptions pricing monthly-commitment bootstrap", flag.ExitOnError)

	subscriptionID := fs.String("subscription-id", "", "Subscription ID")
	territory := fs.String("territory", "", "Three-letter territory ID, for example NOR")
	upfrontPricePointID := fs.String("upfront-price-point-id", "", "UPFRONT subscription price point ID")
	monthlyPricePointID := fs.String("monthly-price-point-id", "", "MONTHLY subscription price point ID")
	confirm := fs.Bool("confirm", false, "Confirm creating monthly availability and paired prices")
	authFlags := bindWebSessionFlags(fs)
	output := shared.BindOutputFlags(fs)

	return &ffcli.Command{
		Name:       "bootstrap",
		ShortUsage: "asc web subscriptions pricing monthly-commitment bootstrap --subscription-id SUB_ID --territory NOR --upfront-price-point-id ID --monthly-price-point-id ID --confirm [flags]",
		ShortHelp:  "[experimental] Create monthly plan availability and paired prices.",
		LongHelp: `EXPERIMENTAL / UNOFFICIAL / DISCOURAGED

Create MONTHLY plan availability, then attach paired UPFRONT and MONTHLY prices
using the same private inline subscription PATCH as App Store Connect.

Run once per base territory. Price point IDs must belong to the selected territory.

` + webWarningText,
		FlagSet:   fs,
		UsageFunc: shared.DefaultUsageFunc,
		Exec: func(ctx context.Context, args []string) error {
			if len(args) > 0 {
				return shared.UsageError("web subscriptions pricing monthly-commitment bootstrap does not accept positional arguments")
			}
			id := strings.TrimSpace(*subscriptionID)
			territoryID := strings.ToUpper(strings.TrimSpace(*territory))
			upfrontID := strings.TrimSpace(*upfrontPricePointID)
			monthlyID := strings.TrimSpace(*monthlyPricePointID)
			switch {
			case id == "":
				return shared.UsageError("--subscription-id is required")
			case len(territoryID) != 3:
				return shared.UsageError("--territory must be a three-letter territory ID")
			case upfrontID == "":
				return shared.UsageError("--upfront-price-point-id is required")
			case monthlyID == "":
				return shared.UsageError("--monthly-price-point-id is required")
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

			availabilities, err := listWebSubscriptionPlanAvailabilitiesFn(requestCtx, client, id)
			if err != nil {
				return withWebAuthHint(err, "web subscriptions pricing monthly-commitment bootstrap")
			}
			monthlyAvailability, found := findPlanAvailabilityByType(availabilities, "MONTHLY")
			created := false
			if !found {
				monthlyAvailability, err = dereferencePlanAvailability(createWebSubscriptionPlanAvailabilityFn(requestCtx, client, id, "MONTHLY", []string{territoryID}, false))
				if err != nil {
					return withWebAuthHint(err, "web subscriptions pricing monthly-commitment bootstrap")
				}
				created = true
			} else if !containsTerritory(monthlyAvailability.AvailableTerritories, territoryID) {
				return fmt.Errorf("MONTHLY plan availability %q exists but does not include %s; update its territories before bootstrapping prices", monthlyAvailability.ID, territoryID)
			}

			prices, err := createWebSubscriptionPlanPricesFn(requestCtx, client, id, upfrontID, monthlyID)
			if err != nil {
				return fmt.Errorf("monthly availability ready, but paired price creation failed: %w", err)
			}
			result := webSubscriptionMonthlyCommitmentBootstrapResult{
				SubscriptionID:      id,
				Territory:           territoryID,
				PlanAvailabilityID:  monthlyAvailability.ID,
				PlanAvailabilityNew: created,
				UpfrontPricePointID: prices.UpfrontPricePointID,
				MonthlyPricePointID: prices.MonthlyPricePointID,
				PricesCreated:       true,
			}
			return shared.PrintOutputWithRenderers(
				result,
				*output.Output,
				*output.Pretty,
				func() error { return renderWebMonthlyCommitmentBootstrapTable(result) },
				func() error { return renderWebMonthlyCommitmentBootstrapMarkdown(result) },
			)
		},
	}
}

func findPlanAvailabilityByType(availabilities []webcore.SubscriptionPlanAvailability, planType string) (webcore.SubscriptionPlanAvailability, bool) {
	for _, availability := range availabilities {
		if strings.EqualFold(strings.TrimSpace(availability.PlanType), planType) {
			return availability, true
		}
	}
	return webcore.SubscriptionPlanAvailability{}, false
}

func containsTerritory(territories []string, territory string) bool {
	for _, candidate := range territories {
		if strings.EqualFold(strings.TrimSpace(candidate), territory) {
			return true
		}
	}
	return false
}

func dereferencePlanAvailability(availability *webcore.SubscriptionPlanAvailability, err error) (webcore.SubscriptionPlanAvailability, error) {
	if err != nil {
		return webcore.SubscriptionPlanAvailability{}, err
	}
	if availability == nil || strings.TrimSpace(availability.ID) == "" {
		return webcore.SubscriptionPlanAvailability{}, fmt.Errorf("apple returned an empty plan availability")
	}
	return *availability, nil
}

func renderWebMonthlyCommitmentBootstrapTable(result webSubscriptionMonthlyCommitmentBootstrapResult) error {
	asc.RenderTable(
		[]string{"Subscription ID", "Territory", "Plan Availability ID", "Availability Created", "Prices Created"},
		[][]string{{
			result.SubscriptionID,
			result.Territory,
			result.PlanAvailabilityID,
			fmt.Sprintf("%t", result.PlanAvailabilityNew),
			fmt.Sprintf("%t", result.PricesCreated),
		}},
	)
	return nil
}

func renderWebMonthlyCommitmentBootstrapMarkdown(result webSubscriptionMonthlyCommitmentBootstrapResult) error {
	fmt.Println("| Subscription ID | Territory | Plan Availability ID | Availability Created | Prices Created |")
	fmt.Println("|---|---|---|---|---|")
	fmt.Printf("| %s | %s | %s | %t | %t |\n", result.SubscriptionID, result.Territory, result.PlanAvailabilityID, result.PlanAvailabilityNew, result.PricesCreated)
	return nil
}
