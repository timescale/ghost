---
title: "ghost feedback"
slug: "ghost_feedback"
description: "CLI reference for ghost feedback"
---

## ghost feedback

Submit feedback, a bug report, or a support request

### Synopsis

Submit feedback, a bug report, or a support request to the Ghost team.

If no message is provided as an argument, reads from stdin.

```
ghost feedback [message] [flags]
```

### Examples

```
  # Submit feedback as an argument
  ghost feedback "I can't connect to my database after resuming it"

  # Submit feedback from stdin
  echo "Great tool!" | ghost feedback

  # Submit feedback interactively
  ghost feedback
  # → Enter your feedback (press Ctrl+D when done):
```

### Options

```
  -h, --help   help for feedback
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
