package cmd

import (
	"testing"
)

func TestMCPListCmd(t *testing.T) {
	experimental := withEnv("GHOST_EXPERIMENTAL", "true")

	tests := []cmdTest{
		{
			name: "text output",
			args: []string{"mcp", "list"},
			opts: []runOption{experimental},
			wantStdout: "TYPE    NAME                                    \n" +
				"prompt  design-postgis-tables                   \n" +
				"prompt  design-postgres-tables                  \n" +
				"prompt  find-hypertable-candidates              \n" +
				"prompt  migrate-postgres-tables-to-hypertables  \n" +
				"prompt  pgvector-semantic-search                \n" +
				"prompt  postgres                                \n" +
				"prompt  postgres-hybrid-text-search             \n" +
				"prompt  setup-timescaledb-hypertables           \n" +
				"tool    ghost_connect                           \n" +
				"tool    ghost_create                            \n" +
				"tool    ghost_create_dedicated                  \n" +
				"tool    ghost_delete                            \n" +
				"tool    ghost_feedback                          \n" +
				"tool    ghost_fork                              \n" +
				"tool    ghost_fork_dedicated                    \n" +
				"tool    ghost_invoice                           \n" +
				"tool    ghost_invoice_list                      \n" +
				"tool    ghost_list                              \n" +
				"tool    ghost_login                             \n" +
				"tool    ghost_logs                              \n" +
				"tool    ghost_password                          \n" +
				"tool    ghost_pause                             \n" +
				"tool    ghost_rename                            \n" +
				"tool    ghost_resume                            \n" +
				"tool    ghost_schema                            \n" +
				"tool    ghost_share                             \n" +
				"tool    ghost_share_list                        \n" +
				"tool    ghost_share_revoke                      \n" +
				"tool    ghost_sql                               \n" +
				"tool    ghost_status                            \n" +
				"tool    search_docs                             \n" +
				"tool    view_skill                              \n",
		},
		{
			// JSON output is 1000+ lines (full tool schemas), so we just verify
			// it doesn't error. The text test above validates the capability list.
			name: "json output",
			args: []string{"mcp", "list", "--json"},
			opts: []runOption{experimental},
		},
		{
			// Same rationale as JSON.
			name: "yaml output",
			args: []string{"mcp", "list", "--yaml"},
			opts: []runOption{experimental},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runCommand(t, tt.args, tt.setup, tt.opts...)

			if tt.wantErr != "" {
				if result.err == nil {
					t.Fatal("expected error, got nil")
				}
				assertOutput(t, result.err.Error(), tt.wantErr)
			} else if result.err != nil {
				t.Fatalf("unexpected error: %v", result.err)
			}

			if tt.wantStdout != "" {
				assertOutput(t, result.stdout, tt.wantStdout)
			} else if result.stdout == "" {
				t.Error("expected non-empty stdout")
			}
		})
	}
}
