---
title: "ghost psql"
slug: "ghost_psql"
description: "CLI reference for ghost psql"
---

## ghost psql

Connect to a database using psql

### Synopsis

Connect to a database using psql.

The psql client must already be installed on your machine. The database
password is read from ~/.pgpass. If no password is found, the command
will fail with an error.

Any flags after -- are passed directly to psql.

```
ghost psql <name-or-id> [-- <psql-flags>...] [flags]
```

### Examples

```
  # Connect to a database
  ghost psql my-database

  # Pass additional psql flags
  ghost psql my-database -- --single-transaction --quiet
```

### Options

```
  -h, --help        help for psql
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
