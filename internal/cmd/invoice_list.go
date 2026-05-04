package cmd

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/util"
)

// InvoiceOutput represents the output format for a single invoice.
type InvoiceOutput struct {
	ID            string  `json:"id"`
	InvoiceNumber string  `json:"invoice_number"`
	InvoiceDate   string  `json:"invoice_date"`
	Total         float64 `json:"total"`
	Status        string  `json:"status"`
}

func buildInvoiceListCmd(app *common.App) *cobra.Command {
	var jsonOutput bool
	var yamlOutput bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List invoices",
		Long: `List invoices for your Ghost space.

Returns the most recent invoices.`,
		Aliases:           []string{"ls"},
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, projectID, err := app.GetClient()
			if err != nil {
				return err
			}

			resp, err := client.ListInvoicesWithResponse(cmd.Context(), projectID)
			if err != nil {
				return fmt.Errorf("failed to list invoices: %w", err)
			}

			if resp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
			}

			if resp.JSON200 == nil {
				return errors.New("empty response from API")
			}

			invoices := resp.JSON200.Invoices

			output := make([]InvoiceOutput, len(invoices))
			for i, inv := range invoices {
				output[i] = InvoiceOutput{
					ID:            inv.Id,
					InvoiceNumber: inv.InvoiceNumber,
					InvoiceDate:   inv.InvoiceDate.Format("2006-01-02"),
					Total:         inv.Total,
					Status:        string(inv.Status),
				}
			}

			switch {
			case jsonOutput:
				return util.SerializeToJSON(cmd.OutOrStdout(), output)
			case yamlOutput:
				return util.SerializeToYAML(cmd.OutOrStdout(), output)
			default:
				if len(output) == 0 {
					cmd.Println("No invoices found.")
					return nil
				}
				return outputInvoices(cmd.OutOrStdout(), output)
			}
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&yamlOutput, "yaml", false, "Output in YAML format")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml")

	return cmd
}

func outputInvoices(w io.Writer, invoices []InvoiceOutput) error {
	table := tablewriter.NewTable(w,
		tablewriter.WithHeaderAlignment(tw.AlignLeft),
		tablewriter.WithPadding(tw.Padding{Left: "", Right: "  ", Overwrite: true}),
		tablewriter.WithRendition(tw.Rendition{
			Borders: tw.Border{
				Left:   tw.Off,
				Right:  tw.Off,
				Top:    tw.Off,
				Bottom: tw.Off,
			},
			Settings: tw.Settings{
				Separators: tw.Separators{
					ShowHeader:     tw.Off,
					ShowFooter:     tw.Off,
					BetweenRows:    tw.Off,
					BetweenColumns: tw.Off,
				},
				Lines: tw.Lines{
					ShowHeaderLine: tw.Off,
				},
			},
		}),
	)

	table.Header("ID", "DATE", "TOTAL", "STATUS")
	for _, inv := range invoices {
		total := fmt.Sprintf("$%.2f", inv.Total)
		table.Append(inv.ID, inv.InvoiceDate, total, inv.Status)
	}

	return table.Render()
}

func invoiceIDCompletion(app *common.App) func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		invoices, err := listInvoices(cmd, app)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		results := make([]string, 0, len(invoices))
		for _, inv := range invoices {
			if strings.HasPrefix(inv.Id, toComplete) {
				desc := fmt.Sprintf("%s (%s)", inv.InvoiceNumber, inv.InvoiceDate.Format("2006-01-02"))
				results = append(results, cobra.CompletionWithDesc(inv.Id, desc))
			}
		}
		return results, cobra.ShellCompDirectiveNoFileComp
	}
}

func listInvoices(cmd *cobra.Command, app *common.App) ([]api.Invoice, error) {
	client, projectID, err := app.GetClient()
	if err != nil {
		return nil, err
	}

	resp, err := client.ListInvoicesWithResponse(cmd.Context(), projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list invoices: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
	}

	if resp.JSON200 == nil {
		return nil, errors.New("empty response from API")
	}

	return resp.JSON200.Invoices, nil
}
