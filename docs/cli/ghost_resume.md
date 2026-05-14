---
title: "ghost resume"
slug: "ghost_resume"
description: "CLI reference for ghost resume"
---

## ghost resume

Resume a paused database

### Synopsis

Resume a paused database to accept connections again.

```
ghost resume <name-or-id> [flags]
```

### Examples

```
  # Resume a database
  ghost resume my-database

  # Resume and wait for the database to be ready
  ghost resume my-database --wait
```

### Options

```
  -h, --help   help for resume
      --wait   Wait for the database to be ready before returning
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
