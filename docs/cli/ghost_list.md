---
title: "ghost list"
slug: "ghost_list"
description: "CLI reference for ghost list"
---

## ghost list

List all databases

### Synopsis

List all databases, including each database's current status, storage usage, and compute hours used in the current billing cycle.

```
ghost list [flags]
```

### Examples

```
  # List all databases
  ghost list

  # List as JSON
  ghost list --json

  # List as YAML
  ghost list --yaml
```

### Options

```
  -h, --help   help for list
      --json   Output in JSON format
      --yaml   Output in YAML format
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
