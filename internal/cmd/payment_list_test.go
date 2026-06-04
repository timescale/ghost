package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/api/mock"
)

func TestPaymentListCmd(t *testing.T) {
	methods := api.PaymentMethodsResponse{
		PaymentMethods: []api.PaymentMethod{
			{Id: "pm_123", Brand: "Visa", Last4: "4242", ExpMonth: 12, ExpYear: 2025, Primary: true},
			{Id: "pm_456", Brand: "Mastercard", Last4: "5555", ExpMonth: 6, ExpYear: 2026, PendingDeletion: true},
		},
	}

	tests := []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"payment", "list"},
			opts:    []runOption{withClientError(errors.New("authentication required: no credentials found"))},
			wantErr: "authentication required: no credentials found",
		},
		{
			name: "network error",
			args: []string{"payment", "list"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().ListPaymentMethodsWithResponse(validCtx, "test-project").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to list payment methods: connection refused",
		},
		{
			name: "API error",
			args: []string{"payment", "list"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().ListPaymentMethodsWithResponse(validCtx, "test-project").
					Return(&api.ListPaymentMethodsResponse{
						HTTPResponse: httpResponse(http.StatusInternalServerError),
						JSONDefault:  &api.Error{Message: "internal error"},
					}, nil)
			},
			wantErr: "internal error",
		},
		{
			name: "nil response body",
			args: []string{"payment", "list"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().ListPaymentMethodsWithResponse(validCtx, "test-project").
					Return(&api.ListPaymentMethodsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      nil,
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name: "empty list",
			args: []string{"payment", "list"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().ListPaymentMethodsWithResponse(validCtx, "test-project").
					Return(&api.ListPaymentMethodsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &api.PaymentMethodsResponse{PaymentMethods: []api.PaymentMethod{}},
					}, nil)
			},
			wantStdout: "No payment methods on file.\nRun 'ghost payment add' to add a payment method.\n",
		},
		{
			name: "text output",
			args: []string{"payment", "list"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().ListPaymentMethodsWithResponse(validCtx, "test-project").
					Return(&api.ListPaymentMethodsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &methods,
					}, nil)
			},
			wantStdout: "ID      BRAND       LAST 4  EXPIRES  PRIMARY  PENDING DELETION  \npm_123  Visa        4242    12/2025  yes      no                \npm_456  Mastercard  5555    06/2026  no       yes               \n",
		},
		{
			name: "json output",
			args: []string{"payment", "list", "--json"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().ListPaymentMethodsWithResponse(validCtx, "test-project").
					Return(&api.ListPaymentMethodsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &methods,
					}, nil)
			},
			wantStdout: `[
  {
    "brand": "Visa",
    "exp_month": 12,
    "exp_year": 2025,
    "id": "pm_123",
    "last4": "4242",
    "pending_deletion": false,
    "primary": true
  },
  {
    "brand": "Mastercard",
    "exp_month": 6,
    "exp_year": 2026,
    "id": "pm_456",
    "last4": "5555",
    "pending_deletion": true,
    "primary": false
  }
]
`,
		},
		{
			name: "yaml output",
			args: []string{"payment", "list", "--yaml"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().ListPaymentMethodsWithResponse(validCtx, "test-project").
					Return(&api.ListPaymentMethodsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &methods,
					}, nil)
			},
			wantStdout: `- brand: Visa
  exp_month: 12
  exp_year: 2025
  id: pm_123
  last4: "4242"
  pending_deletion: false
  primary: true
- brand: Mastercard
  exp_month: 6
  exp_year: 2026
  id: pm_456
  last4: "5555"
  pending_deletion: true
  primary: false
`,
		},
		{
			name: "ls alias",
			args: []string{"payment", "ls"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().ListPaymentMethodsWithResponse(validCtx, "test-project").
					Return(&api.ListPaymentMethodsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &methods,
					}, nil)
			},
			wantStdout: "ID      BRAND       LAST 4  EXPIRES  PRIMARY  PENDING DELETION  \npm_123  Visa        4242    12/2025  yes      no                \npm_456  Mastercard  5555    06/2026  no       yes               \n",
		},
	}

	runCmdTests(t, tests)
}
