---
title: "ghost logs"
slug: "ghost_logs"
description: "CLI reference for ghost logs"
---

## ghost logs

View logs for a database

### Synopsis

View logs for a database.

Fetches and displays logs from the specified database. By default, shows the
last 500 log entries. Log lines are displayed in chronological order with the
most recent entries at the bottom.

```
ghost logs <name-or-id> [flags]
```

### Examples

```
  # View last 500 logs
  ghost logs my-database

  # View last 50 lines
  ghost logs my-database --tail 50

  # View logs before a specific time
  ghost logs my-database --until 2024-01-15T10:00:00Z

  # View logs as JSON
  ghost logs my-database --json
```

### Options

```
  -h, --help         help for logs
      --json         Output in JSON format
      --tail int     Number of log lines to show (default 500)
      --until time   Fetch logs before this timestamp (RFC3339 format, e.g. 2024-01-15T10:00:00Z)
      --yaml         Output in YAML format
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
