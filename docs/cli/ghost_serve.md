---
title: "ghost serve"
slug: "ghost_serve"
description: "CLI reference for ghost serve"
---

## ghost serve

Launch a local web UI for running SQL queries

### Synopsis

Start a local web server and open a browser to a UI that lets you run SQL
queries against your ghost databases. The server runs only for the duration
of this command — press Ctrl+C to stop it.

```
ghost serve [flags]
```

### Examples

```
  # Launch on an auto-picked port and open the browser
  ghost serve

  # Pin a port and skip the browser
  ghost serve --port 5174 --no-open
```

### Options

```
  -h, --help          help for serve
      --host string   interface to bind (loopback by default) (default "127.0.0.1")
      --no-open       do not open the browser
      --port int      TCP port to listen on (0 = auto)
```

### Options inherited from parent commands

```
      --analytics           enable/disable usage analytics (default true)
      --color               enable colored output (default true)
      --config-dir string   config directory (default "~/.config/ghost")
      --version-check       check for updates (default true)
```

### SEE ALSO

* [ghost](ghost.md)	 - CLI for managing Postgres databases
