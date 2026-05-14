---
title: "ghost mcp get"
slug: "ghost_mcp_get"
description: "CLI reference for ghost mcp get"
---

## ghost mcp get

Get detailed information about a specific MCP capability

### Synopsis

Get detailed information about a specific MCP tool, prompt, resource, or resource template.

```
ghost mcp get <name> [flags]
```

### Examples

```
  # Get details about a tool
  ghost mcp get ghost_create

  # Get details about a prompt
  ghost mcp get setup-timescaledb-hypertables

  # Get details as JSON
  ghost mcp get ghost_create --json

  # Get details as YAML
  ghost mcp get ghost_create --yaml
```

### Options

```
  -h, --help   help for get
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
