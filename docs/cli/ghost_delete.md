---
title: "ghost delete"
slug: "ghost_delete"
description: "CLI reference for ghost delete"
---

## ghost delete

Delete a database

### Synopsis

Delete a database permanently.

This operation is irreversible. By default, you will be prompted to confirm
the deletion, unless you use the --confirm flag.

```
ghost delete <name-or-id> [flags]
```

### Examples

```
  # Delete a database (with confirmation prompt)
  ghost delete my-database

  # Delete a database without confirmation prompt
  ghost delete my-database --confirm
```

### Options

```
      --confirm   Skip confirmation prompt
  -h, --help      help for delete
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
