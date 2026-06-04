---
title: "ghost id"
slug: "ghost_id"
description: "CLI reference for ghost id"
---

## ghost id

Show the authenticated user or API key

### Synopsis

Show information about the authenticated caller.

The output depends on how you are authenticated: logging in as a user shows your
user details, while authenticating with an API key shows details about the key
itself, such as its scope and the user who created it.

```
ghost id [flags]
```

### Examples

```
  # Show the authenticated identity
  ghost id

  # Output as JSON
  ghost id --json

  # Output as YAML
  ghost id --yaml
```

### Options

```
  -h, --help   help for id
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
