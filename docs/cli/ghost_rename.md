---
title: "ghost rename"
slug: "ghost_rename"
description: "CLI reference for ghost rename"
---

## ghost rename

Rename a database

### Synopsis

Rename a database.

```
ghost rename <name-or-id> <new-name> [flags]
```

### Examples

```
  # Rename a database
  ghost rename my-database my-new-name
```

### Options

```
  -h, --help   help for rename
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
