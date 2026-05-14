---
title: "ghost payment primary"
slug: "ghost_payment_primary"
description: "CLI reference for ghost payment primary"
---

## ghost payment primary

Set the primary payment method

### Synopsis

Set a payment method as the primary payment method for your space.

```
ghost payment primary <payment-id> [flags]
```

### Examples

```
  # Set a specific payment method as primary
  ghost payment primary pm_xxx
```

### Options

```
  -h, --help   help for primary
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
