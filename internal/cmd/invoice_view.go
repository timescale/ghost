package cmd

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/util"
)

// InvoiceLineItemOutput represents the output format for a single invoice line item.
type InvoiceLineItemOutput struct {
	ProductType  string  `json:"product_type"`
	DatabaseID   string  `json:"database_id,omitempty"`
	DetailedSpec string  `json:"detailed_spec,omitempty"`
	Quantity     float64 `json:"quantity"`
	UnitPrice    float64 `json:"unit_price"`
	LineTotal    float64 `json:"line_total"`
}

// InvoiceDetailOutput represents the output format for an invoice detail.
type InvoiceDetailOutput struct {
	LineItems []InvoiceLineItemOutput `json:"line_items"`
}

func buildInvoiceViewCmd(app *common.App) *cobra.Command {
	var jsonOutput bool
	var yamlOutput bool

	cmd := &cobra.Command{
		Use:   "view <invoice-id>",
		Short: "View invoice detail",
		Long: `View the line-item breakdown for a single invoice.

The invoice ID is the opaque ID from 'ghost invoice list'.`,
		Aliases:           []string{"get", "details", "show"},
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: invoiceIDCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, projectID, err := app.GetClient()
			if err != nil {
				return err
			}

			resp, err := client.GetInvoiceWithResponse(cmd.Context(), projectID, args[0])
			if err != nil {
				return fmt.Errorf("failed to get invoice: %w", err)
			}

			if resp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
			}

			if resp.JSON200 == nil {
				return errors.New("empty response from API")
			}

			lineItems := resp.JSON200.LineItems
			output := InvoiceDetailOutput{
				LineItems: make([]InvoiceLineItemOutput, len(lineItems)),
			}
			for i, li := range lineItems {
				item := InvoiceLineItemOutput{
					ProductType: li.ProductType,
					Quantity:    li.Quantity,
					UnitPrice:   li.UnitPrice,
					LineTotal:   li.LineTotal,
				}
				if li.DatabaseId != nil {
					item.DatabaseID = *li.DatabaseId
				}
				if li.DetailedSpec != nil {
					item.DetailedSpec = *li.DetailedSpec
				}
				output.LineItems[i] = item
			}

			switch {
			case jsonOutput:
				return util.SerializeToJSON(cmd.OutOrStdout(), output)
			case yamlOutput:
				return util.SerializeToYAML(cmd.OutOrStdout(), output)
			default:
				if len(output.LineItems) == 0 {
					cmd.Println("No line items on this invoice.")
					return nil
				}
				return outputInvoiceDetail(cmd.OutOrStdout(), output)
			}
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&yamlOutput, "yaml", false, "Output in YAML format")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml")

	return cmd
}

func outputInvoiceDetail(w io.Writer, detail InvoiceDetailOutput) error {
	table := common.NewTable(w)

	table.Header("PRODUCT", "DATABASE ID", "QTY", "UNIT PRICE", "TOTAL")
	for _, li := range detail.LineItems {
		qty := formatInvoiceQuantity(li.Quantity)
		unitPrice := fmt.Sprintf("$%g", li.UnitPrice)
		total := fmt.Sprintf("$%.2f", li.LineTotal)
		table.Append(li.ProductType, li.DatabaseID, qty, unitPrice, total)
	}

	return table.Render()
}

// formatInvoiceQuantity formats a float for display in the quantity column,
// trimming any trailing ".0" for whole numbers so small integers render
// without a decimal.
func formatInvoiceQuantity(v float64) string {
	return fmt.Sprintf("%g", v)
}
