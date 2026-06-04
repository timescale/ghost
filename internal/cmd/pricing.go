package cmd

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/util"
)

func buildPricingCmd(app *common.App) *cobra.Command {
	var jsonOutput, yamlOutput bool

	cmd := &cobra.Command{
		Use:               "pricing",
		Aliases:           []string{"price", "prices"},
		Short:             "Show pricing",
		Long:              `Show pricing for compute overages and dedicated databases.`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// The /pricing endpoint is public, but we reuse the standard
			// authenticated client builder for simplicity — login is cheap
			// and there's no existing unauthenticated-client path.
			client, _, err := app.GetClient()
			if err != nil {
				return err
			}

			resp, err := client.GetPricingWithResponse(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to get pricing: %w", err)
			}
			if resp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
			}
			if resp.JSON200 == nil {
				return errors.New("empty response from API")
			}
			pricing := resp.JSON200

			switch {
			case jsonOutput:
				return util.SerializeToJSON(cmd.OutOrStdout(), pricing)
			case yamlOutput:
				return util.SerializeToYAML(cmd.OutOrStdout(), pricing)
			default:
				return outputPricing(cmd.OutOrStdout(), pricing)
			}
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&yamlOutput, "yaml", false, "Output in YAML format")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml")
	return cmd
}

func outputPricing(w io.Writer, pricing *api.Pricing) error {
	overage := pricing.Standard.Compute
	fmt.Fprintf(w, `First %d compute-hours per month included; $%.4f/hour above that.

Compute-hours are shared across all non-dedicated databases in the space and
are reset monthly. Usage is metered in 15-minute intervals with at least one
query. Databases are auto-paused when the compute limit is reached. Run 'ghost
overages enable' to allow paid usage above the included free allowance.
`, overage.IncludedComputeHoursPerMonth, overage.PricePerHour)

	fmt.Fprintln(w)
	fmt.Fprintln(w, lipgloss.NewStyle().Bold(true).Render("Dedicated"))

	storage := pricing.Dedicated.Storage
	fmt.Fprintf(w, `Always-on databases for production workloads. Separate from the shared compute
pool and billed by uptime rather than query activity. Pausing stops compute
charges, but storage charges continue. The first %d GiB of storage per database
is included; $%.6f/GiB/hour ($%.2f/GiB/month) above that.
`, storage.IncludedGibPerDatabase, storage.PricePerGibHour, storage.PricePerGibMonth)
	fmt.Fprintln(w)

	table := common.NewTable(w)
	table.Header("SIZE", "VCPU", "MEMORY", "$/HOUR", "$/MONTH")
	for _, c := range pricing.Dedicated.Compute {
		table.Append(
			string(c.Size),
			fmt.Sprintf("%.1f", float64(c.MilliCpu)/1000),
			fmt.Sprintf("%d GiB", c.MemoryGib),
			fmt.Sprintf("$%.4f", c.PricePerHour),
			fmt.Sprintf("$%.2f", c.PricePerMonth),
		)
	}
	return table.Render()
}
