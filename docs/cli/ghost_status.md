---
title: "ghost status"
slug: "ghost_status"
description: "CLI reference for ghost status"
---

## ghost status

Show space usage

```
ghost status [flags]
```

### Examples

```
  # Show space usage
  ghost status

  # Output as JSON
  ghost status --json

  # Output as YAML
  ghost status --yaml
```

### Options

```
  -h, --help   help for status
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
