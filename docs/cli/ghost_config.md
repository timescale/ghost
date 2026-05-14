---
title: "ghost config"
slug: "ghost_config"
description: "CLI reference for ghost config"
---

## ghost config

List current configuration

### Synopsis

Display the current configuration settings

```
ghost config [flags]
```

### Options

```
      --env    Apply environment variable overrides
  -h, --help   help for config
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
* [ghost config reset](ghost_config_reset.md)	 - Reset to defaults
* [ghost config set](ghost_config_set.md)	 - Set configuration value
* [ghost config unset](ghost_config_unset.md)	 - Remove configuration value
