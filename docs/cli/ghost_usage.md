---
title: "ghost usage"
slug: "ghost_usage"
description: "CLI reference for ghost usage"
---

## ghost usage

Show space usage

```
ghost usage [flags]
```

### Examples

```
  # Show space usage
  ghost usage

  # Output as JSON
  ghost usage --json

  # Output as YAML
  ghost usage --yaml
```

### Options

```
  -h, --help   help for usage
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
