---
title: "ghost payment delete"
slug: "ghost_payment_delete"
description: "CLI reference for ghost payment delete"
---

## ghost payment delete

Delete a payment method

### Synopsis

Delete a payment method.

```
ghost payment delete <payment-id> [flags]
```

### Examples

```
  # Delete a specific payment method
  ghost payment delete pm_xxx

  # Delete without confirmation prompt
  ghost payment delete pm_xxx --confirm
```

### Options

```
      --confirm   Skip confirmation prompt
  -h, --help      help for delete
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
