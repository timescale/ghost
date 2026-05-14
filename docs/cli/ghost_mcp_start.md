---
title: "ghost mcp start"
slug: "ghost_mcp_start"
description: "CLI reference for ghost mcp start"
---

## ghost mcp start

Start the Ghost MCP server

### Synopsis

Start the Ghost MCP server. Uses stdio transport by default.

```
ghost mcp start [flags]
```

### Examples

```
  # Start with stdio transport (default)
  ghost mcp start

  # Start with stdio transport (explicit)
  ghost mcp start stdio

  # Start with HTTP transport
  ghost mcp start http
```

### Options

```
  -h, --help   help for start
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
* [ghost mcp start http](ghost_mcp_start_http.md)	 - Start MCP server with HTTP transport
* [ghost mcp start stdio](ghost_mcp_start_stdio.md)	 - Start MCP server with stdio transport
