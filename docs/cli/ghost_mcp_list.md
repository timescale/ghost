---
title: "ghost mcp list"
slug: "ghost_mcp_list"
description: "CLI reference for ghost mcp list"
---

## ghost mcp list

List available MCP tools, prompts, and resources

### Synopsis

List all MCP tools, prompts, and resources exposed via the Ghost MCP server.

The output can be formatted as a table, JSON, or YAML.

```
ghost mcp list [flags]
```

### Examples

```
  # List all capabilities in table format (default)
  ghost mcp list

  # List as JSON
  ghost mcp list --json

  # List as YAML
  ghost mcp list --yaml
```

### Options

```
  -h, --help   help for list
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
