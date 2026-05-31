package ads

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"strings"

	"github.com/peterbourgon/ff/v3/ffcli"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/appleads"
	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/cli/shared"
)

type campaignStatusWorkflowFlags struct {
	common   commonFlags
	output   shared.OutputFlags
	campaign *string
	confirm  *bool
}

func workflowSubcommands(path []string) []*ffcli.Command {
	if len(path) == 1 && path[0] == "campaigns" {
		return []*ffcli.Command{
			campaignStatusWorkflowCommand("pause", "PAUSED", "Pause a campaign."),
			campaignStatusWorkflowCommand("resume", "ENABLED", "Resume a campaign."),
		}
	}
	return nil
}

func campaignStatusWorkflowCommand(name, status, shortHelp string) *ffcli.Command {
	fs := flag.NewFlagSet("campaigns "+name, flag.ExitOnError)
	flags := campaignStatusWorkflowFlags{
		common: commonFlags{
			AdsProfile: fs.String("ads-profile", "", "Use named Apple Ads authentication profile"),
			Org:        fs.String("org", "", "Apple Ads organization ID (or ASC_ADS_ORG_ID env)"),
		},
		output:   shared.BindOutputFlags(fs),
		campaign: fs.String("campaign", "", "campaignId (required)"),
		confirm:  fs.Bool("confirm", false, "Confirm this Apple Ads campaign status change"),
	}
	return &ffcli.Command{
		Name:       name,
		ShortUsage: "asc ads campaigns " + name + " [flags]",
		ShortHelp:  shortHelp,
		LongHelp: fmt.Sprintf(`%s

Endpoint: PUT v5/campaigns/{campaignId}

Examples:
  asc ads campaigns %s --campaign CAMPAIGN --confirm --org ORG_ID`, shortHelp, name),
		FlagSet:   fs,
		UsageFunc: shared.DefaultUsageFunc,
		Exec: func(ctx context.Context, args []string) error {
			if err := rejectUnexpectedArgs(args); err != nil {
				return err
			}
			return executeCampaignStatusWorkflow(ctx, status, flags)
		},
	}
}

func executeCampaignStatusWorkflow(ctx context.Context, status string, flags campaignStatusWorkflowFlags) error {
	campaignID := value(flags.campaign)
	if campaignID == "" {
		return shared.UsageError("--campaign is required")
	}
	if err := validateAdsIntegerFlag("campaign", campaignID); err != nil {
		return err
	}
	if flags.confirm == nil || !*flags.confirm {
		return shared.UsageError("--confirm is required")
	}

	spec, ok := appleads.EndpointByCommandPath("campaigns", "update")
	if !ok {
		return fmt.Errorf("ads campaigns status workflow: missing campaigns update endpoint")
	}

	client, err := resolveClient(ctx, flags.common, spec.RequiresOrg)
	if err != nil {
		return fmt.Errorf("ads campaigns %s: %w", strings.ToLower(status), err)
	}

	requestCtx, cancel := requestContext(ctx)
	defer cancel()

	body, err := json.Marshal(map[string]map[string]string{
		"campaign": {"status": status},
	})
	if err != nil {
		return err
	}
	result, err := client.Do(requestCtx, spec, map[string]string{"campaignId": campaignID}, nil, body)
	if err != nil {
		return fmt.Errorf("ads campaigns %s: %w", strings.ToLower(status), err)
	}
	return shared.PrintOutput(result, *flags.output.Output, *flags.output.Pretty)
}

func validateAdsIntegerFlag(name, raw string) error {
	spec := appleads.EndpointSpec{
		PathParams: []appleads.ParamSpec{{
			Name:     name,
			Flag:     name,
			Type:     appleads.ParamInt,
			Required: true,
		}},
	}
	flags := endpointFlagValues{
		pathStrings: map[string]*string{name: &raw},
	}
	_, err := collectPathParams(spec, flags)
	if err != nil {
		return shared.UsageError(err.Error())
	}
	return nil
}
