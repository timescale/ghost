package cmd

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/api/mock"
)

func TestInvoiceListCmd(t *testing.T) {
	invoices := api.InvoicesResponse{
		Invoices: []api.Invoice{
			{
				Id:            "BJ9xX3JDEzMQ9vCx",
				InvoiceNumber: "INV-12345",
				InvoiceDate:   time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
				Total:         27.50,
				Status:        api.InvoiceStatusPaid,
			},
			{
				Id:            "k2mN7pQrS4tLvZwY",
				InvoiceNumber: "INV-12344",
				InvoiceDate:   time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
				Total:         24.10,
				Status:        api.InvoiceStatusPaid,
			},
		},
	}

	experimental := withEnv("GHOST_EXPERIMENTAL", "true")

	tests := []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"invoice", "list"},
			opts:    []runOption{experimental, withClientError(errors.New("authentication required: no credentials found"))},
			wantErr: "authentication required: no credentials found",
		},
		{
			name: "network error",
			args: []string{"invoice", "list"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().ListInvoicesWithResponse(validCtx, "test-project").
					Return(nil, errors.New("connection refused"))
			},
			opts:    []runOption{experimental},
			wantErr: "failed to list invoices: connection refused",
		},
		{
			name: "API error",
			args: []string{"invoice", "list"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().ListInvoicesWithResponse(validCtx, "test-project").
					Return(&api.ListInvoicesResponse{
						HTTPResponse: httpResponse(http.StatusForbidden),
						JSONDefault:  &api.Error{Message: "this endpoint requires user authentication"},
					}, nil)
			},
			opts:    []runOption{experimental},
			wantErr: "this endpoint requires user authentication",
		},
		{
			name: "nil response body",
			args: []string{"invoice", "list"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().ListInvoicesWithResponse(validCtx, "test-project").
					Return(&api.ListInvoicesResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      nil,
					}, nil)
			},
			opts:    []runOption{experimental},
			wantErr: "empty response from API",
		},
		{
			name: "empty list",
			args: []string{"invoice", "list"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().ListInvoicesWithResponse(validCtx, "test-project").
					Return(&api.ListInvoicesResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &api.InvoicesResponse{Invoices: []api.Invoice{}},
					}, nil)
			},
			opts:       []runOption{experimental},
			wantStdout: "No invoices found.\n",
		},
		{
			name: "text output",
			args: []string{"invoice", "list"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().ListInvoicesWithResponse(validCtx, "test-project").
					Return(&api.ListInvoicesResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &invoices,
					}, nil)
			},
			opts:       []runOption{experimental},
			wantStdout: "ID                DATE        TOTAL   STATUS  \nBJ9xX3JDEzMQ9vCx  2026-04-01  $27.50  paid    \nk2mN7pQrS4tLvZwY  2026-03-01  $24.10  paid    \n",
		},
		{
			name: "json output",
			args: []string{"invoice", "list", "--json"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().ListInvoicesWithResponse(validCtx, "test-project").
					Return(&api.ListInvoicesResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &invoices,
					}, nil)
			},
			opts: []runOption{experimental},
			wantStdout: `[
  {
    "id": "BJ9xX3JDEzMQ9vCx",
    "invoice_number": "INV-12345",
    "invoice_date": "2026-04-01",
    "total": 27.5,
    "status": "paid"
  },
  {
    "id": "k2mN7pQrS4tLvZwY",
    "invoice_number": "INV-12344",
    "invoice_date": "2026-03-01",
    "total": 24.1,
    "status": "paid"
  }
]
`,
		},
		{
			name: "yaml output",
			args: []string{"invoice", "list", "--yaml"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().ListInvoicesWithResponse(validCtx, "test-project").
					Return(&api.ListInvoicesResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &invoices,
					}, nil)
			},
			opts: []runOption{experimental},
			wantStdout: `- id: BJ9xX3JDEzMQ9vCx
  invoice_date: "2026-04-01"
  invoice_number: INV-12345
  status: paid
  total: 27.5
- id: k2mN7pQrS4tLvZwY
  invoice_date: "2026-03-01"
  invoice_number: INV-12344
  status: paid
  total: 24.1
`,
		},
		{
			name: "ls alias",
			args: []string{"invoice", "ls"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().ListInvoicesWithResponse(validCtx, "test-project").
					Return(&api.ListInvoicesResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &invoices,
					}, nil)
			},
			opts:       []runOption{experimental},
			wantStdout: "ID                DATE        TOTAL   STATUS  \nBJ9xX3JDEzMQ9vCx  2026-04-01  $27.50  paid    \nk2mN7pQrS4tLvZwY  2026-03-01  $24.10  paid    \n",
		},
	}

	runCmdTests(t, tests)
}
