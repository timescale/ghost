---
title: "ghost share"
slug: "ghost_share"
description: "CLI reference for ghost share"
---

## ghost share

Share a database

### Synopsis

Share a database so a recipient can create their own fork from a snapshot.

The share URL can be handed to anyone — they don't need access to this space.
Whoever opens the URL gets instructions to run 'ghost create --from-share-token <token>'
(or 'ghost create dedicated --from-share-token <token>'), which spins up a new database
in their own space from the shared snapshot.

```
ghost share <name-or-id> [flags]
```

### Examples

```
  # Share a database (no expiry)
  ghost share my-database

  # Share for 24 hours
  ghost share my-database --expires-in 24h

  # Share until a specific time
  ghost share my-database --expires-at 2026-05-01T00:00:00Z

  # Output as JSON
  ghost share my-database --json

  # The recipient creates their own database from the share token
  ghost create --from-share-token <token>
```

### Options

```
      --expires-at string     Absolute expiry as RFC3339 (e.g. 2026-05-01T00:00:00Z)
      --expires-in duration   Relative expiry (e.g. 30m, 24h)
  -h, --help                  help for share
      --json                  Output in JSON format
      --yaml                  Output in YAML format
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
* [ghost share list](ghost_share_list.md)	 - List database shares
* [ghost share revoke](ghost_share_revoke.md)	 - Revoke a database share

