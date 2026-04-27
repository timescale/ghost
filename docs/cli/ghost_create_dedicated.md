---
title: "ghost create dedicated"
slug: "ghost_create_dedicated"
description: "CLI reference for ghost create dedicated"
---

## ghost create dedicated

Create a dedicated database

### Synopsis

Create a new dedicated database. Dedicated databases are always-on,
billed instances that are not subject to space compute or storage limits.
A payment method must be on file.

```
ghost create dedicated [flags]
```

### Examples

```
  # Create a dedicated database (default size: 1x)
  ghost create dedicated

  # Create with a specific size
  ghost create dedicated --size 2x

  # Create with a custom name
  ghost create dedicated --name myapp --size 4x

  # Create a dedicated database from a share token
  ghost create dedicated --from-share <token>

  # Create and output as JSON
  ghost create dedicated --json

  # Create and wait for the database to be ready
  ghost create dedicated --size 2x --wait
```

### Options

```
      --from-share string   Create the database from a share token
  -h, --help                help for dedicated
      --json                Output in JSON format
      --name string         Database name (auto-generated if not provided)
      --size string         Database size (1x, 2x, 4x, 8x) (default "1x")
      --wait                Wait for the database to be ready before returning
      --yaml                Output in YAML format
```

### Options inherited from parent commands

```
      --analytics           enable/disable usage analytics (default true)
      --color               enable colored output (default true)
      --config-dir string   config directory (default "~/.config/ghost")
      --version-check       check for updates (default true)
```

### SEE ALSO

* [ghost create](ghost_create.md)	 - Create a new Postgres database

