---
title: "ghost mcp uninstall"
slug: "ghost_mcp_uninstall"
description: "CLI reference for ghost mcp uninstall"
---

## ghost mcp uninstall

Uninstall Ghost MCP server configuration from a client

### Synopsis

Uninstall the Ghost MCP server configuration from a supported MCP client.

Pass "all" to uninstall from all supported clients. If no client is specified, you'll be prompted to select one or more interactively.
Only the Ghost MCP server entry named "ghost" is removed; other MCP server entries are left untouched.

```
ghost mcp uninstall [client] [flags]
```

### Examples

```
  # Interactive client selection (multi-select)
  ghost mcp uninstall

  # Uninstall from Cursor
  ghost mcp uninstall cursor

  # Uninstall from all supported clients
  ghost mcp uninstall all

  # Skip backups when modifying config files
  ghost mcp uninstall cursor --no-backup
```

### Options

```
  -h, --help        help for uninstall
      --json        Output in JSON format
      --no-backup   Skip creating backup of existing configuration files (default: create backup)
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
