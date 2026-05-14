---
title: "ghost invoice view"
slug: "ghost_invoice_view"
description: "CLI reference for ghost invoice view"
---

## ghost invoice view

View invoice detail

### Synopsis

View the line-item breakdown for a single invoice.

The invoice ID is the opaque ID from 'ghost invoice list'.

```
ghost invoice view <invoice-id> [flags]
```

### Options

```
  -h, --help   help for view
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

* [ghost invoice](ghost_invoice.md)	 - View invoices
