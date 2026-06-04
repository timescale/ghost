package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/util"
)

type PricingInput struct{}

func (PricingInput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[PricingInput](nil))
}

// PricingOutput is the MCP tool's output type. It has the same underlying type
// as api.Pricing so values convert directly, and is redeclared here so the tool
// can attach a Schema() method (matching the pattern other MCP tools use).
type PricingOutput api.Pricing

func (PricingOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[PricingOutput](nil))
	schema.Properties["dedicated"].Description = "Pricing for dedicated (always-on) databases"

	ded := schema.Properties["dedicated"]
	ded.Properties["compute"].Description = "Per-size compute pricing, ordered smallest to largest"
	ded.Properties["storage"].Description = "Storage pricing"

	c := ded.Properties["compute"].Items
	c.Properties["size"].Description = "Size identifier"
	c.Properties["size"].Examples = []any{"1x", "2x", "4x", "8x"}
	c.Properties["milli_cpu"].Description = "CPU allocation in millicores (1000 = 1 vCPU)"
	c.Properties["memory_gib"].Description = "Memory allocation in GiB"
	c.Properties["price_per_hour"].Description = "Price per hour while the database is running"
	c.Properties["price_per_month"].Description = "Price per month while the database is running"

	st := ded.Properties["storage"]
	st.Properties["price_per_gib_hour"].Description = "Price per GiB per hour of provisioned storage above the included amount"
	st.Properties["price_per_gib_month"].Description = "Price per GiB per month of provisioned storage above the included amount"
	st.Properties["included_gib_per_database"].Description = "GiB of storage included per database at no additional charge. Only storage above this amount is billed at price_per_gib_hour"

	std := schema.Properties["standard"]
	std.Description = "Pricing for standard (non-dedicated) databases, which share a space-wide compute allowance"
	sc := std.Properties["compute"]
	sc.Description = "Pricing for standard (shared) compute used beyond the free monthly allowance"
	sc.Properties["price_per_hour"].Description = "Price per compute-hour used beyond the included monthly allowance, metered in 15-minute intervals with at least one query"
	sc.Properties["included_compute_hours_per_month"].Description = "Compute-hours included per month at no additional charge, shared across all non-dedicated databases in the space. Only usage above this amount is billed at price_per_hour"
	return schema
}

func newPricingTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "ghost_pricing",
		Title: "Get Pricing",
		Description: `Get pricing for compute overages and dedicated databases.

Databases share a monthly pool of compute-hours across the space and are reset monthly. Usage is metered in 15-minute intervals with at least one query. Databases are auto-paused when the compute limit is reached. Run 'ghost overages enable' to allow paid usage above the included free allowance.

Dedicated databases are always-on instances separate from the shared compute pool. They are billed by uptime rather than query activity, making them well-suited for production workloads. Pausing stops compute charges, but storage charges continue.`,
		InputSchema:  PricingInput{}.Schema(),
		OutputSchema: PricingOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(true),
			Title:         "Get Pricing",
		},
	}
}

func (s *Server) handlePricing(ctx context.Context, req *mcp.CallToolRequest, input PricingInput) (*mcp.CallToolResult, PricingOutput, error) {
	client, _, err := s.app.GetClient()
	if err != nil {
		return nil, PricingOutput{}, err
	}

	resp, err := client.GetPricingWithResponse(ctx)
	if err != nil {
		return nil, PricingOutput{}, fmt.Errorf("failed to get pricing: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, PricingOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
	}
	if resp.JSON200 == nil {
		return nil, PricingOutput{}, errors.New("empty response from API")
	}

	return nil, PricingOutput(*resp.JSON200), nil
}
