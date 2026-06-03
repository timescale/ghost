---
title: "ghost schema"
slug: "ghost_schema"
description: "CLI reference for ghost schema"
---

## ghost schema

Display database schema information

### Synopsis

Display database schema information including tables, views, materialized views,
enum types, functions, and procedures with their columns, constraints, indexes,
and triggers. By default all user-visible schemas are shown; system schemas
(information_schema, pg_*, _timescaledb_*) and extension-owned objects are
excluded.

```
ghost schema <name-or-id> [flags]
```

### Examples

```
  ghost schema my-database
  ghost schema my-database --schema reporting
  ghost schema my-database --internal
```

### Options

```
  -h, --help            help for schema
      --internal        Include system schemas (information_schema, pg_*, _timescaledb_*) and extension-owned objects
      --schema string   Restrict output to a single Postgres schema
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
