---
title: "ghost schema"
slug: "ghost_schema"
description: "CLI reference for ghost schema"
---

## ghost schema

Display database schema information

### Synopsis

Display database schema information including tables (regular, partitioned, and
foreign), views, materialized views, enum types, functions, and procedures with
their columns, constraints, indexes, and triggers. Only objects the connecting user can access are listed. By default
system schemas (information_schema, pg_*, _timescaledb_*) and extension-owned
objects are excluded; use --schema to target a specific schema (including a
system schema such as pg_catalog) or --internal to include everything.

Object definitions (view SELECT statements and function/procedure bodies) are
omitted by default to keep the output concise; pass --definitions to include
them. Object comments (COMMENT ON text for schemas, tables, views, columns,
enums, functions, and procedures) are likewise omitted by default; pass
--comments to include them.

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
      --comments        Include object comments (COMMENT ON text for schemas, tables, views, columns, enums, functions, and procedures)
      --definitions     Include full object definitions (view SELECT statements and function/procedure bodies)
  -h, --help            help for schema
      --internal        Include system schemas (information_schema, pg_*, _timescaledb_*) and extension-owned objects
      --schema string   Restrict output to a single Postgres schema (may be a system schema; only objects you can access are shown)
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
