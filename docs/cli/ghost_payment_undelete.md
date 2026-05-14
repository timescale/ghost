---
title: "ghost payment undelete"
slug: "ghost_payment_undelete"
description: "CLI reference for ghost payment undelete"
---

## ghost payment undelete

Cancel a pending payment method deletion

### Synopsis

Cancel the pending deletion of a payment method.

```
ghost payment undelete <payment-id> [flags]
```

### Examples

```
  # Cancel deletion for a specific payment method
  ghost payment undelete pm_xxx
```

### Options

```
  -h, --help   help for undelete
```

### Options inherited from parent commands

```
      --analytics           enable/disable usage analytics (default true)
      --color               enable colored output (default true)
      --config-dir string   config directory (default "~/.config/ghost")
      --version-check       check for updates (default true)
```

### SEE ALSO

* [ghost payment](ghost_payment.md)	 - Manage payment methods
