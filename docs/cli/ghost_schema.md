---
title: "ghost schema"
slug: "ghost_schema"
description: "CLI reference for ghost schema"
---

## ghost schema

Display database schema information

### Synopsis

Display database schema information including tables, views, materialized views,
and enum types with their columns, constraints, and indexes.

```
ghost schema <name-or-id> [flags]
```

### Examples

```
  ghost schema my-database
```

### Options

```
  -h, --help   help for schema
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
