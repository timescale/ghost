package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/api/mock"
)

func TestPricingCmd(t *testing.T) {
	pricing := api.Pricing{
		Dedicated: api.DedicatedPricing{
			Compute: []api.ComputePrice{
				{Size: "1x", MilliCpu: 500, MemoryGib: 2, PricePerHour: 0.0137, PricePerMonth: 10},
				{Size: "2x", MilliCpu: 1000, MemoryGib: 4, PricePerHour: 0.0274, PricePerMonth: 20},
				{Size: "4x", MilliCpu: 2000, MemoryGib: 8, PricePerHour: 0.0548, PricePerMonth: 40},
				{Size: "8x", MilliCpu: 4000, MemoryGib: 16, PricePerHour: 0.1096, PricePerMonth: 80},
			},
			Storage: api.StoragePrice{
				PricePerGibHour:        0.0003424657534,
				PricePerGibMonth:       0.25,
				IncludedGibPerDatabase: 10,
			},
		},
		Standard: api.StandardPricing{
			Compute: api.StandardComputePrice{
				PricePerHour:                 0.075,
				IncludedComputeHoursPerMonth: 100,
			},
		},
	}

	wantText := "First 100 compute-hours per month included; $0.0750/hour above that.\n" +
		"\n" +
		"Compute-hours are shared across all non-dedicated databases in the space and\n" +
		"are reset monthly. Usage is metered in 15-minute intervals with at least one\n" +
		"query. Databases are auto-paused when the compute limit is reached. Run 'ghost\n" +
		"overages enable' to allow paid usage above the included free allowance.\n" +
		"\n" +
		"Dedicated\n" +
		"Always-on databases for production workloads. Separate from the shared compute\n" +
		"pool and billed by uptime rather than query activity. Pausing stops compute\n" +
		"charges, but storage charges continue. The first 10 GiB of storage per database\n" +
		"is included; $0.000342/GiB/hour ($0.25/GiB/month) above that.\n" +
		"\n" +
		"SIZE  VCPU  MEMORY  $/HOUR   $/MONTH  \n" +
		"1x    0.5   2 GiB   $0.0137  $10.00   \n" +
		"2x    1.0   4 GiB   $0.0274  $20.00   \n" +
		"4x    2.0   8 GiB   $0.0548  $40.00   \n" +
		"8x    4.0   16 GiB  $0.1096  $80.00   \n"

	tests := []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"pricing"},
			opts:    []runOption{withClientError(errors.New("authentication required: no credentials found"))},
			wantErr: "authentication required: no credentials found",
		},
		{
			name: "network error",
			args: []string{"pricing"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().GetPricingWithResponse(validCtx).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to get pricing: connection refused",
		},
		{
			name: "API error",
			args: []string{"pricing"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().GetPricingWithResponse(validCtx).
					Return(&api.GetPricingResponse{
						HTTPResponse: httpResponse(http.StatusInternalServerError),
						JSONDefault:  &api.Error{Message: "internal server error"},
					}, nil)
			},
			wantErr: "internal server error",
		},
		{
			name: "nil response body",
			args: []string{"pricing"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().GetPricingWithResponse(validCtx).
					Return(&api.GetPricingResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      nil,
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name: "text output",
			args: []string{"pricing"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().GetPricingWithResponse(validCtx).
					Return(&api.GetPricingResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &pricing,
					}, nil)
			},
			wantStdout: wantText,
		},
		{
			name: "json output",
			args: []string{"pricing", "--json"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().GetPricingWithResponse(validCtx).
					Return(&api.GetPricingResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &pricing,
					}, nil)
			},
			wantStdout: `{
  "dedicated": {
    "compute": [
      {
        "memory_gib": 2,
        "milli_cpu": 500,
        "price_per_hour": 0.0137,
        "price_per_month": 10,
        "size": "1x"
      },
      {
        "memory_gib": 4,
        "milli_cpu": 1000,
        "price_per_hour": 0.0274,
        "price_per_month": 20,
        "size": "2x"
      },
      {
        "memory_gib": 8,
        "milli_cpu": 2000,
        "price_per_hour": 0.0548,
        "price_per_month": 40,
        "size": "4x"
      },
      {
        "memory_gib": 16,
        "milli_cpu": 4000,
        "price_per_hour": 0.1096,
        "price_per_month": 80,
        "size": "8x"
      }
    ],
    "storage": {
      "included_gib_per_database": 10,
      "price_per_gib_hour": 0.0003424657534,
      "price_per_gib_month": 0.25
    }
  },
  "standard": {
    "compute": {
      "included_compute_hours_per_month": 100,
      "price_per_hour": 0.075
    }
  }
}
`,
		},
		{
			name: "yaml output",
			args: []string{"pricing", "--yaml"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().GetPricingWithResponse(validCtx).
					Return(&api.GetPricingResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &pricing,
					}, nil)
			},
			wantStdout: `dedicated:
  compute:
    - memory_gib: 2
      milli_cpu: 500
      price_per_hour: 0.0137
      price_per_month: 10
      size: 1x
    - memory_gib: 4
      milli_cpu: 1000
      price_per_hour: 0.0274
      price_per_month: 20
      size: 2x
    - memory_gib: 8
      milli_cpu: 2000
      price_per_hour: 0.0548
      price_per_month: 40
      size: 4x
    - memory_gib: 16
      milli_cpu: 4000
      price_per_hour: 0.1096
      price_per_month: 80
      size: 8x
  storage:
    included_gib_per_database: 10
    price_per_gib_hour: 0.0003424657534
    price_per_gib_month: 0.25
standard:
  compute:
    included_compute_hours_per_month: 100
    price_per_hour: 0.075
`,
		},
		{
			name: "price alias",
			args: []string{"price"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().GetPricingWithResponse(validCtx).
					Return(&api.GetPricingResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &pricing,
					}, nil)
			},
			wantStdout: wantText,
		},
		{
			name: "prices alias",
			args: []string{"prices"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().GetPricingWithResponse(validCtx).
					Return(&api.GetPricingResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &pricing,
					}, nil)
			},
			wantStdout: wantText,
		},
	}

	runCmdTests(t, tests)
}
