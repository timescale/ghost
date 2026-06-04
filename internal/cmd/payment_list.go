package cmd

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/util"
)

func buildPaymentListCmd(app *common.App) *cobra.Command {
	var jsonOutput bool
	var yamlOutput bool

	cmd := &cobra.Command{
		Use:               "list",
		Short:             "List payment methods",
		Long:              `List payment methods for your Ghost space.`,
		Args:              cobra.NoArgs,
		Aliases:           []string{"ls"},
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, projectID, err := app.GetClient()
			if err != nil {
				return err
			}

			resp, err := client.ListPaymentMethodsWithResponse(cmd.Context(), projectID)
			if err != nil {
				return fmt.Errorf("failed to list payment methods: %w", err)
			}

			if resp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
			}

			if resp.JSON200 == nil {
				return errors.New("empty response from API")
			}

			methods := resp.JSON200.PaymentMethods
			if len(methods) == 0 {
				cmd.Println("No payment methods on file.")
				cmd.Println("Run 'ghost payment add' to add a payment method.")
				return nil
			}

			switch {
			case jsonOutput:
				return util.SerializeToJSON(cmd.OutOrStdout(), methods)
			case yamlOutput:
				return util.SerializeToYAML(cmd.OutOrStdout(), methods)
			default:
				return outputPaymentMethods(cmd.OutOrStdout(), methods)
			}
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&yamlOutput, "yaml", false, "Output in YAML format")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml")

	return cmd
}

func outputPaymentMethods(w io.Writer, methods []api.PaymentMethod) error {
	table := common.NewTable(w)

	table.Header("ID", "BRAND", "LAST 4", "EXPIRES", "PRIMARY", "PENDING DELETION")
	for _, pm := range methods {
		expires := fmt.Sprintf("%02d/%d", pm.ExpMonth, pm.ExpYear)
		primary := "no"
		if pm.Primary {
			primary = "yes"
		}
		pendingDeletion := "no"
		if pm.PendingDeletion {
			pendingDeletion = "yes"
		}
		table.Append(pm.Id, pm.Brand, pm.Last4, expires, primary, pendingDeletion)
	}

	return table.Render()
}
