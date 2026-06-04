package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/api/mock"
)

func TestInvoiceViewCmd(t *testing.T) {
	detail := api.InvoiceDetail{
		LineItems: []api.InvoiceLineItem{
			{
				ProductType: "storage",
				DatabaseId:  new("svc-abc123"),
				Quantity:    20,
				UnitPrice:   0.25,
				LineTotal:   5.00,
			},
			{
				ProductType: "compute",
				DatabaseId:  new("svc-abc123"),
				Quantity:    40,
				UnitPrice:   0.50,
				LineTotal:   20.00,
			},
		},
	}

	experimental := withEnv("GHOST_EXPERIMENTAL", "true")

	tests := []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"invoice", "view", "inv_123"},
			opts:    []runOption{experimental, withClientError(errors.New("authentication required: no credentials found"))},
			wantErr: "authentication required: no credentials found",
		},
		{
			name: "network error",
			args: []string{"invoice", "view", "inv_123"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().GetInvoiceWithResponse(validCtx, "test-project", "inv_123").
					Return(nil, errors.New("connection refused"))
			},
			opts:    []runOption{experimental},
			wantErr: "failed to get invoice: connection refused",
		},
		{
			name: "forbidden (cross-tenant)",
			args: []string{"invoice", "view", "inv_bad"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().GetInvoiceWithResponse(validCtx, "test-project", "inv_bad").
					Return(&api.GetInvoiceResponse{
						HTTPResponse: httpResponse(http.StatusForbidden),
						JSONDefault:  &api.Error{Message: "insufficient permissions to view the target invoice"},
					}, nil)
			},
			opts:    []runOption{experimental},
			wantErr: "insufficient permissions to view the target invoice",
		},
		{
			name: "nil response body",
			args: []string{"invoice", "view", "inv_123"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().GetInvoiceWithResponse(validCtx, "test-project", "inv_123").
					Return(&api.GetInvoiceResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      nil,
					}, nil)
			},
			opts:    []runOption{experimental},
			wantErr: "empty response from API",
		},
		{
			name: "empty line items",
			args: []string{"invoice", "view", "inv_123"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().GetInvoiceWithResponse(validCtx, "test-project", "inv_123").
					Return(&api.GetInvoiceResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &api.InvoiceDetail{LineItems: []api.InvoiceLineItem{}},
					}, nil)
			},
			opts:       []runOption{experimental},
			wantStdout: "No line items on this invoice.\n",
		},
		{
			name: "text output",
			args: []string{"invoice", "view", "inv_123"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().GetInvoiceWithResponse(validCtx, "test-project", "inv_123").
					Return(&api.GetInvoiceResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &detail,
					}, nil)
			},
			opts:       []runOption{experimental},
			wantStdout: "PRODUCT  DATABASE ID  QTY  UNIT PRICE  TOTAL   \nstorage  svc-abc123   20   $0.25       $5.00   \ncompute  svc-abc123   40   $0.5        $20.00  \n",
		},
		{
			name: "json output",
			args: []string{"invoice", "view", "inv_123", "--json"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().GetInvoiceWithResponse(validCtx, "test-project", "inv_123").
					Return(&api.GetInvoiceResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &detail,
					}, nil)
			},
			opts: []runOption{experimental},
			wantStdout: `{
  "line_items": [
    {
      "product_type": "storage",
      "database_id": "svc-abc123",
      "quantity": 20,
      "unit_price": 0.25,
      "line_total": 5
    },
    {
      "product_type": "compute",
      "database_id": "svc-abc123",
      "quantity": 40,
      "unit_price": 0.5,
      "line_total": 20
    }
  ]
}
`,
		},
		{
			name: "get alias",
			args: []string{"invoice", "get", "inv_123"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().GetInvoiceWithResponse(validCtx, "test-project", "inv_123").
					Return(&api.GetInvoiceResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &detail,
					}, nil)
			},
			opts:       []runOption{experimental},
			wantStdout: "PRODUCT  DATABASE ID  QTY  UNIT PRICE  TOTAL   \nstorage  svc-abc123   20   $0.25       $5.00   \ncompute  svc-abc123   40   $0.5        $20.00  \n",
		},
		{
			name: "details alias",
			args: []string{"invoice", "details", "inv_123"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().GetInvoiceWithResponse(validCtx, "test-project", "inv_123").
					Return(&api.GetInvoiceResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &detail,
					}, nil)
			},
			opts:       []runOption{experimental},
			wantStdout: "PRODUCT  DATABASE ID  QTY  UNIT PRICE  TOTAL   \nstorage  svc-abc123   20   $0.25       $5.00   \ncompute  svc-abc123   40   $0.5        $20.00  \n",
		},
		{
			name: "show alias",
			args: []string{"invoice", "show", "inv_123"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().GetInvoiceWithResponse(validCtx, "test-project", "inv_123").
					Return(&api.GetInvoiceResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &detail,
					}, nil)
			},
			opts:       []runOption{experimental},
			wantStdout: "PRODUCT  DATABASE ID  QTY  UNIT PRICE  TOTAL   \nstorage  svc-abc123   20   $0.25       $5.00   \ncompute  svc-abc123   40   $0.5        $20.00  \n",
		},
	}

	runCmdTests(t, tests)
}
