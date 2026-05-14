---
title: "ghost mcp start http"
slug: "ghost_mcp_start_http"
description: "CLI reference for ghost mcp start http"
---

## ghost mcp start http

Start MCP server with HTTP transport

### Synopsis

Start the MCP server using the Streamable HTTP transport.

```
ghost mcp start http [flags]
```

### Examples

```
  # Start HTTP server on default port 8080
  ghost mcp start http

  # Start HTTP server on custom port
  ghost mcp start http --port 3001

  # Start HTTP server on all interfaces
  ghost mcp start http --host 0.0.0.0 --port 8080

  # Start server and bind to specific interface
  ghost mcp start http --host 192.168.1.100 --port 9000
```

### Options

```
  -h, --help          help for http
      --host string   Host to bind to (default "localhost")
      --port int      Port to run HTTP server on (default 8080)
```

### Options inherited from parent commands

```
      --analytics           enable/disable usage analytics (default true)
      --color               enable colored output (default true)
      --config-dir string   config directory (default "~/.config/ghost")
      --version-check       check for updates (default true)
```

### SEE ALSO

* [ghost mcp start](ghost_mcp_start.md)	 - Start the Ghost MCP server
