---
title: "ghost payment"
slug: "ghost_payment"
description: "CLI reference for ghost payment"
---

## ghost payment

Manage payment methods

### Synopsis

Manage payment methods for your Ghost space. Opens an interactive menu when run from a terminal, or lists payment methods otherwise.

```
ghost payment [flags]
```

### Options

```
  -h, --help   help for payment
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
* [ghost payment add](ghost_payment_add.md)	 - Add a payment method
* [ghost payment delete](ghost_payment_delete.md)	 - Delete a payment method
* [ghost payment list](ghost_payment_list.md)	 - List payment methods
* [ghost payment primary](ghost_payment_primary.md)	 - Set the primary payment method
* [ghost payment undelete](ghost_payment_undelete.md)	 - Cancel a pending payment method deletion
