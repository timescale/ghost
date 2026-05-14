---
title: "ghost mcp install"
slug: "ghost_mcp_install"
description: "CLI reference for ghost mcp install"
---

## ghost mcp install

Install and configure Ghost MCP server for a client

### Synopsis

Install and configure the Ghost MCP server for a specific MCP client or AI assistant.

This command automates the configuration process by modifying the appropriate
configuration files for the specified client.

Supported Clients:
  claude-code              Configure for Claude Code
  codex                    Configure for Codex
  cursor                   Configure for Cursor
  gemini                   Configure for Gemini CLI
  antigravity              Configure for Google Antigravity
  kiro-cli                 Configure for Kiro CLI
  vscode                   Configure for VS Code
  windsurf                 Configure for Windsurf

The command will:
- Automatically detect the appropriate configuration file location
- Create the configuration directory if it doesn't exist
- Create a backup of existing configuration by default
- Merge with existing MCP server configurations (doesn't overwrite other servers)
- Validate the configuration after installation

Pass "all" to configure every supported client. If no client is specified, you'll be prompted to pick one or more clients interactively.

```
ghost mcp install [client] [flags]
```

### Examples

```
  # Interactive client selection (multi-select)
  ghost mcp install

  # Install for Claude Code (User scope - available in all projects)
  ghost mcp install claude-code

  # Install for Cursor IDE
  ghost mcp install cursor

  # Install for all supported clients
  ghost mcp install all

  # Install without creating backup
  ghost mcp install claude-code --no-backup
```

### Options

```
  -h, --help        help for install
      --json        Output in JSON format
      --no-backup   Skip creating backup of existing configuration (default: create backup)
      --yaml        Output in YAML format
```

### Options inherited from parent commands

```
      --analytics           enable/disable usage analytics (default true)
      --color               enable colored output (default true)
      --config-dir string   config directory (default "~/.config/ghost")
      --version-check       check for updates (default true)
```

### SEE ALSO

* [ghost mcp](ghost_mcp.md)	 - Ghost Model Context Protocol (MCP) server
