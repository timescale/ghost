---
title: "ghost tutorial"
slug: "ghost_tutorial"
description: "CLI reference for ghost tutorial"
---

## ghost tutorial

Run an interactive Ghost tutorial

### Synopsis

Run an interactive tutorial that demonstrates the core Ghost workflow.

The tutorial creates a temporary database, inserts
sample data, forks the database, mutates the fork, compares the original and
fork, and then asks whether to delete or keep the tutorial databases. Each step
explains and echoes the equivalent Ghost CLI command before running it.

```
ghost tutorial [flags]
```

### Examples

```
  ghost tutorial
```

### Options

```
  -h, --help   help for tutorial
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
