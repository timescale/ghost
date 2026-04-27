---
title: "ghost share revoke"
slug: "ghost_share_revoke"
description: "CLI reference for ghost share revoke"
---

## ghost share revoke

Revoke a database share

### Synopsis

Revoke a share so its URL can no longer be used to create new databases.

```
ghost share revoke <share-token> [flags]
```

### Options

```
  -h, --help   help for revoke
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

* [ghost share](ghost_share.md)	 - Share a database

