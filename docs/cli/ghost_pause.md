---
title: "ghost pause"
slug: "ghost_pause"
description: "CLI reference for ghost pause"
---

## ghost pause

Pause a running database

### Synopsis

Pause a running database. This terminates active connections.

```
ghost pause <name-or-id> [flags]
```

### Examples

```
  # Pause a database
  ghost pause my-database
```

### Options

```
  -h, --help   help for pause
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
