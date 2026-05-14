---
title: "ghost sql"
slug: "ghost_sql"
description: "CLI reference for ghost sql"
---

## ghost sql

Execute SQL query on a database

### Synopsis

Execute a SQL query against a database and display the results.

If no query is provided as an argument, reads from stdin.

Multi-statement queries (semicolon-separated) are supported. Results from
all statements that return rows will be displayed.

```
ghost sql <name-or-id> [query] [flags]
```

### Examples

```
  # Select data from a table
  ghost sql my-database "SELECT * FROM users LIMIT 5"

  # Execute DDL
  ghost sql my-database "CREATE TABLE todos (id SERIAL PRIMARY KEY, title TEXT)"

  # Multi-statement query
  ghost sql my-database "INSERT INTO users (name) VALUES ('alice'); SELECT * FROM users"

  # Read query from stdin
  echo "SELECT 1" | ghost sql my-database

  # Read query from a file
  ghost sql my-database < schema.sql
```

### Options

```
  -h, --help   help for sql
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
