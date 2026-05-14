---
title: "ghost create"
slug: "ghost_create"
description: "CLI reference for ghost create"
---

## ghost create

Create a new Postgres database

```
ghost create [flags]
```

### Examples

```
  # Create a database with auto-generated name
  ghost create

  # Create a database with a custom name
  ghost create --name myapp

  # Create a database from a share token
  ghost create --from-share <token> --name myapp

  # Create and output as JSON
  ghost create --json

  # Create and output as YAML
  ghost create --yaml

  # Create and wait for the database to be ready
  ghost create --wait
```

### Options

```
      --from-share string   Create the database from a share token
  -h, --help                help for create
      --json                Output in JSON format
      --name string         Database name (auto-generated if not provided)
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

* [ghost](ghost.md)	 - CLI for managing Postgres databases
* [ghost create dedicated](ghost_create_dedicated.md)	 - Create a dedicated database
