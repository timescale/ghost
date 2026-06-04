package cmd

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/api/mock"
)

func TestIDCmd(t *testing.T) {
	userSetup := func(m *mock.MockClientWithResponsesInterface) {
		m.EXPECT().AuthInfoWithResponse(validCtx).
			Return(&api.AuthInfoResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200: &api.AuthInfo{
					Type: api.AuthInfoTypeUser,
					User: &api.UserInfo{
						Id:    "usr_123",
						Name:  "Jane Doe",
						Email: "jane@example.com",
					},
				},
			}, nil)
	}

	apiKeySetup := func(m *mock.MockClientWithResponsesInterface) {
		m.EXPECT().AuthInfoWithResponse(validCtx).
			Return(&api.AuthInfoResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200: &api.AuthInfo{
					Type: api.AuthInfoTypeApiKey,
					ApiKey: &api.ApiKeyInfo{
						Prefix:    "gt_abc123",
						Name:      "CI Key",
						CreatedAt: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
						UserId:    "usr_123",
						UserName:  "Jane Doe",
						UserEmail: "jane@example.com",
						SpaceId:   "spc_456",
						SpaceName: "my-space",
					},
				},
			}, nil)
	}

	tests := []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"id"},
			opts:    []runOption{withClientError(errors.New("authentication required: no credentials found"))},
			wantErr: "authentication required: no credentials found",
		},
		{
			name: "network error",
			args: []string{"id"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().AuthInfoWithResponse(validCtx).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to get auth info: connection refused",
		},
		{
			name: "API error",
			args: []string{"id"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().AuthInfoWithResponse(validCtx).
					Return(&api.AuthInfoResponse{
						HTTPResponse: httpResponse(http.StatusInternalServerError),
						JSONDefault:  &api.Error{Message: "internal error"},
					}, nil)
			},
			wantErr: "internal error",
		},
		{
			name: "nil response body",
			args: []string{"id"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().AuthInfoWithResponse(validCtx).
					Return(&api.AuthInfoResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      nil,
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:  "user text output",
			args:  []string{"id"},
			setup: userSetup,
			wantStdout: `Type: OAuth
User: Jane Doe (jane@example.com)
`,
		},
		{
			name:  "user json output",
			args:  []string{"id", "--json"},
			setup: userSetup,
			wantStdout: `{
  "type": "user",
  "user": {
    "email": "jane@example.com",
    "id": "usr_123",
    "name": "Jane Doe"
  }
}
`,
		},
		{
			name:  "user yaml output",
			args:  []string{"id", "--yaml"},
			setup: userSetup,
			wantStdout: `type: user
user:
  email: jane@example.com
  id: usr_123
  name: Jane Doe
`,
		},
		{
			name:  "api key text output",
			args:  []string{"id"},
			setup: apiKeySetup,
			wantStdout: `Type: API Key
Name: CI Key
Prefix: gt_abc123
Space: my-space (spc_456)
User: Jane Doe (jane@example.com)
Created: 2024-01-15 10:30:00 +0000 UTC
`,
		},
		{
			name:  "api key json output",
			args:  []string{"id", "--json"},
			setup: apiKeySetup,
			wantStdout: `{
  "api_key": {
    "created_at": "2024-01-15T10:30:00Z",
    "name": "CI Key",
    "prefix": "gt_abc123",
    "space_id": "spc_456",
    "space_name": "my-space",
    "user_email": "jane@example.com",
    "user_id": "usr_123",
    "user_name": "Jane Doe"
  },
  "type": "api_key"
}
`,
		},
		{
			name:  "api key yaml output",
			args:  []string{"id", "--yaml"},
			setup: apiKeySetup,
			wantStdout: `api_key:
  created_at: "2024-01-15T10:30:00Z"
  name: CI Key
  prefix: gt_abc123
  space_id: spc_456
  space_name: my-space
  user_email: jane@example.com
  user_id: usr_123
  user_name: Jane Doe
type: api_key
`,
		},
		{
			name:  "identity alias",
			args:  []string{"identity"},
			setup: userSetup,
			wantStdout: `Type: OAuth
User: Jane Doe (jane@example.com)
`,
		},
		{
			name:  "whoami alias",
			args:  []string{"whoami"},
			setup: userSetup,
			wantStdout: `Type: OAuth
User: Jane Doe (jane@example.com)
`,
		},
		{
			name:  "who alias",
			args:  []string{"who"},
			setup: userSetup,
			wantStdout: `Type: OAuth
User: Jane Doe (jane@example.com)
`,
		},
	}

	runCmdTests(t, tests)
}
