package cmd

import (
	"testing"
)

func TestConfigCmd(t *testing.T) {
	tests := []cmdTest{
		{
			name: "text output defaults",
			args: []string{"config"},
			wantStdout: "analytics      true   \n" +
				"color          true   \n" +
				"read_only      false  \n" +
				"version_check  true   \n",
		},
		{
			name: "json output defaults",
			args: []string{"config", "--json"},
			wantStdout: `{
  "analytics": true,
  "color": true,
  "read_only": false,
  "version_check": true
}
`,
		},
		{
			name: "yaml output defaults",
			args: []string{"config", "--yaml"},
			wantStdout: `analytics: true
color: true
read_only: false
version_check: true
`,
		},
		{
			name: "env flag text output",
			args: []string{"config", "--env"},
			opts: []runOption{withEnv("GHOST_COLOR", "false")},
			wantStdout: "analytics      true   \n" +
				"color          false  \n" +
				"read_only      false  \n" +
				"version_check  true   \n",
		},
		{
			name: "env flag json output",
			args: []string{"config", "--env", "--json"},
			opts: []runOption{withEnv("GHOST_COLOR", "false")},
			wantStdout: `{
  "analytics": true,
  "color": false,
  "read_only": false,
  "version_check": true
}
`,
		},
		{
			name: "env flag yaml output",
			args: []string{"config", "--env", "--yaml"},
			opts: []runOption{withEnv("GHOST_COLOR", "false")},
			wantStdout: `analytics: true
color: false
read_only: false
version_check: true
`,
		},
		{
			name: "all flag text output",
			args: []string{"config", "--all"},
			wantStdout: "analytics      true                            \n" +
				"color          true                            \n" +
				"read_only      false                           \n" +
				"version_check  true                            \n" +
				"api_url        https://api.ghost.build/v0      \n" +
				"docs_mcp_url   https://mcp.tigerdata.com/docs  \n" +
				"releases_url   https://install.ghost.build     \n" +
				"share_url      https://ghost.build/share       \n",
		},
		{
			name: "all flag json output",
			args: []string{"config", "--all", "--json"},
			wantStdout: `{
  "api_url": "https://api.ghost.build/v0",
  "analytics": true,
  "color": true,
  "docs_mcp_url": "https://mcp.tigerdata.com/docs",
  "read_only": false,
  "releases_url": "https://install.ghost.build",
  "share_url": "https://ghost.build/share",
  "version_check": true
}
`,
		},
		{
			name: "all flag yaml output",
			args: []string{"config", "--all", "--yaml"},
			wantStdout: `analytics: true
api_url: https://api.ghost.build/v0
color: true
docs_mcp_url: https://mcp.tigerdata.com/docs
read_only: false
releases_url: https://install.ghost.build
share_url: https://ghost.build/share
version_check: true
`,
		},
		{
			name: "env and all flags text output",
			args: []string{"config", "--env", "--all"},
			opts: []runOption{withEnv("GHOST_COLOR", "false")},
			wantStdout: "analytics      true                            \n" +
				"color          false                           \n" +
				"read_only      false                           \n" +
				"version_check  true                            \n" +
				"api_url        https://api.ghost.build/v0      \n" +
				"docs_mcp_url   https://mcp.tigerdata.com/docs  \n" +
				"releases_url   https://install.ghost.build     \n" +
				"share_url      https://ghost.build/share       \n",
		},
		{
			name: "env and all flags json output",
			args: []string{"config", "--env", "--all", "--json"},
			opts: []runOption{withEnv("GHOST_COLOR", "false")},
			wantStdout: `{
  "api_url": "https://api.ghost.build/v0",
  "analytics": true,
  "color": false,
  "docs_mcp_url": "https://mcp.tigerdata.com/docs",
  "read_only": false,
  "releases_url": "https://install.ghost.build",
  "share_url": "https://ghost.build/share",
  "version_check": true
}
`,
		},
		{
			name: "env and all flags yaml output",
			args: []string{"config", "--env", "--all", "--yaml"},
			opts: []runOption{withEnv("GHOST_COLOR", "false")},
			wantStdout: `analytics: true
api_url: https://api.ghost.build/v0
color: false
docs_mcp_url: https://mcp.tigerdata.com/docs
read_only: false
releases_url: https://install.ghost.build
share_url: https://ghost.build/share
version_check: true
`,
		},
	}

	runCmdTests(t, tests)
}
