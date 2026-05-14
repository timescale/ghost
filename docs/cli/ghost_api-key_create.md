---
title: "ghost api-key create"
slug: "ghost_api-key_create"
description: "CLI reference for ghost api-key create"
---

## ghost api-key create

Create a new API key

### Synopsis

Create a new API key for your Ghost space.

The API key is only shown once — make sure to save it.
API keys can be used to authenticate with Ghost by setting the
GHOST_API_KEY environment variable.

```
ghost api-key create [flags]
```

### Examples

```
  # Create an API key with auto-generated name
  ghost api-key create

  # Create an API key with a custom name
  ghost api-key create --name "CI/CD Key"

  # Output as environment variables (useful for .env files)
  ghost api-key create --env > .env

  # Output as JSON
  ghost api-key create --json
```

### Options

```
      --env           Output as environment variables
  -h, --help          help for create
      --json          Output in JSON format
      --name string   API key name (auto-generated if not provided)
      --yaml          Output in YAML format
```

### Options inherited from parent commands

```
      --analytics           enable/disable usage analytics (default true)
      --color               enable colored output (default true)
      --config-dir string   config directory (default "~/.config/ghost")
      --version-check       check for updates (default true)
```

### SEE ALSO

* [ghost api-key](ghost_api-key.md)	 - Manage API keys
