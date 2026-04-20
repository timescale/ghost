---
title: "ghost update"
slug: "ghost_update"
description: "CLI reference for ghost update"
---

## ghost update

Update the ghost CLI to the latest version

### Synopsis

Download and install the latest published version of the ghost CLI, replacing the currently running binary.

Uses the same release archives as the install script. If ghost was installed via a package manager (Homebrew, apt, yum/dnf), the update will be refused with a suggestion to use that package manager instead, unless --force is set.

```
ghost update [flags]
```

### Options

```
      --force            reinstall even if the current version already matches, or the binary was installed via a package manager
  -h, --help             help for update
      --version string   specific version to install (e.g. v1.2.3). Defaults to latest.
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

