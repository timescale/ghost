package cmd

import (
	"testing"
)

func TestConfigSetCmd(t *testing.T) {
	tests := []cmdTest{
		{
			name:    "unknown key",
			args:    []string{"config", "set", "nonexistent", "value"},
			wantErr: "failed to set config: unknown configuration key: nonexistent",
		},
		{
			name:    "invalid bool value",
			args:    []string{"config", "set", "analytics", "notabool"},
			wantErr: "failed to set config: invalid analytics value: notabool (must be true or false)",
		},
		{
			name:    "invalid int value",
			args:    []string{"config", "set", "ui_query_history_limit", "abc"},
			wantErr: "failed to set config: invalid ui_query_history_limit value: abc (must be a positive integer)",
		},
		{
			name:    "non-positive int value",
			args:    []string{"config", "set", "ui_query_history_limit", "0"},
			wantErr: "failed to set config: invalid ui_query_history_limit value: 0 (must be a positive integer)",
		},
		{
			name:       "set analytics false",
			args:       []string{"config", "set", "analytics", "false"},
			wantStdout: "Set analytics = false\n",
		},
		{
			name:       "set color false",
			args:       []string{"config", "set", "color", "false"},
			wantStdout: "Set color = false\n",
		},
		{
			name:       "set read_only true",
			args:       []string{"config", "set", "read_only", "true"},
			wantStdout: "Set read_only = true\n",
		},
		{
			name:       "set ui_query_history_limit",
			args:       []string{"config", "set", "ui_query_history_limit", "25"},
			wantStdout: "Set ui_query_history_limit = 25\n",
		},
	}

	runCmdTests(t, tests)
}
