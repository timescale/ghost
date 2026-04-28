---
title: "ghost upgrade"
slug: "ghost_upgrade"
description: "CLI reference for ghost upgrade"
---

## ghost upgrade

Upgrade the ghost CLI to the latest version

### Synopsis

Download and install the latest published version of the ghost CLI, replacing the currently running binary.

If ghost was installed via a package manager (Homebrew, apt, yum/dnf), the upgrade will be refused with a suggestion to use that package manager instead.

```
ghost upgrade [flags]
```

### Options

```
  -h, --help   help for upgrade
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

