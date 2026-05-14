---
title: "ghost password"
slug: "ghost_password"
description: "CLI reference for ghost password"
---

## ghost password

Reset the password for a database

### Synopsis

Reset the password for the default database user.

This changes the password on the server itself. This operation is irreversible.
Existing connections using the old password will fail to reconnect.

The new password can be provided as a positional argument, entered interactively,
or automatically generated using the --generate flag.

The password will be saved to your ~/.pgpass file for use with psql and other
PostgreSQL tools.

```
ghost password <name-or-id> [new-password] [flags]
```

### Examples

```
  # Update password (interactive prompt)
  ghost password my-database

  # Update password with explicit value
  ghost password my-database "my-new-secure-password"

  # Generate a secure password
  ghost password my-database --generate
```

### Options

```
      --generate   Automatically generate a secure password
  -h, --help       help for password
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
