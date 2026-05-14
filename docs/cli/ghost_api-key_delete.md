---
title: "ghost api-key delete"
slug: "ghost_api-key_delete"
description: "CLI reference for ghost api-key delete"
---

## ghost api-key delete

Delete an API key

### Synopsis

Delete an API key from your Ghost space.

```
ghost api-key delete <prefix> [flags]
```

### Examples

```
  # Delete an API key
  ghost api-key delete gt_abc123

  # Delete without confirmation prompt
  ghost api-key delete gt_abc123 --confirm
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

* [ghost api-key](ghost_api-key.md)	 - Manage API keys
