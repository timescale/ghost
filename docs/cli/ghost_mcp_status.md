---
title: "ghost mcp status"
slug: "ghost_mcp_status"
description: "CLI reference for ghost mcp status"
---

## ghost mcp status

Show Ghost MCP configuration status for supported clients

### Synopsis

Show whether the Ghost MCP server is configured for supported MCP clients.

The command checks the selected client, or all supported clients when no client is specified.
A configured client must have a Ghost MCP server entry named "ghost" that runs "ghost mcp start".

```
ghost mcp status [client] [flags]
```

### Examples

```
  # Check all supported clients
  ghost mcp status

  # Check Cursor only
  ghost mcp status cursor

  # Output as JSON
  ghost mcp status --json
```

### Options

```
  -h, --help   help for status
      --json   Output in JSON format
      --yaml   Output in YAML format
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

