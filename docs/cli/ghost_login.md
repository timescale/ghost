---
title: "ghost login"
slug: "ghost_login"
description: "CLI reference for ghost login"
---

## ghost login

Authenticate with GitHub OAuth

### Synopsis

Authenticate via GitHub OAuth. Opens your browser to complete authentication.

Use --headless for environments without a browser (Docker containers, SSH
sessions, CI/CD, etc.).

```
ghost login [flags]
```

### Examples

```
  # Login via browser (default)
  ghost login

  # Login from a headless environment (Docker, SSH, CI/CD)
  ghost login --headless
```

### Options

```
      --headless   Use device authorization flow (for environments without a browser)
  -h, --help       help for login
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
