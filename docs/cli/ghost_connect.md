---
title: "ghost connect"
slug: "ghost_connect"
description: "CLI reference for ghost connect"
---

## ghost connect

Get connection string for a database

### Synopsis

Get a PostgreSQL connection string for a database.

Includes the password from ~/.pgpass if available.

```
ghost connect <name-or-id> [flags]
```

### Examples

```
  # Get connection string for a database
  ghost connect my-database
  ghost connect a2x6xoj0oz

  # Get a read-only connection string
  ghost connect --read-only my-database
```

### Options

```
  -h, --help        help for connect
      --read-only   Connect in read-only mode
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
