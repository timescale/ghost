# Ghost CLI

The official CLI for [Ghost](https://ghost.build) — the first database built for agents. Offers unlimited Postgres databases you can create, fork, and discard freely.

## Installation

Multiple installation methods are provided. If you aren't sure, use the first one.

### Install Script (macOS/Linux/WSL)

```
curl -fsSL https://install.ghost.build | sh
```

### Install Script (Windows PowerShell)

```powershell
irm https://install.ghost.build/install.ps1 | iex
```

### Homebrew (macOS/Linux)

```bash
brew install timescale/tap/ghost
```

### Debian/Ubuntu

```bash
curl -s https://packagecloud.io/install/repositories/timescale/ghost/script.deb.sh | sudo os=any dist=any bash
sudo apt-get install ghost
```

### Red Hat/Fedora

```bash
curl -s https://packagecloud.io/install/repositories/timescale/ghost/script.rpm.sh | sudo os=rpm_any dist=rpm_any bash
sudo yum install ghost
```

### npm

```bash
npm install -g @ghost.build/cli
```

## Usage

```bash
ghost init        # Interactively configure Ghost (PATH, login, MCP, completions)
ghost create      # Create a new Postgres database
ghost list        # List all databases
```

## Commands

| Command | Description |
|---------|-------------|
| `api-key` | Manage API keys |
| `completion` | Generate the autocompletion script for the specified shell |
| `config` | List current configuration |
| `connect` | Get connection string for a database |
| `create` | Create a new Postgres database |
| `create dedicated` | Create a dedicated database |
| `delete` | Delete a database |
| `feedback` | Submit feedback, a bug report, or a support request |
| `fork` | Fork a database |
| `fork dedicated` | Fork a database as dedicated |
| `help` | Help about any command |
| `init` | Interactively configure Ghost |
| `invoice` | View invoices |
| `list` | List all databases |
| `logs` | View logs for a database |
| `login` | Authenticate with GitHub OAuth |
| `logout` | Remove stored credentials |
| `mcp` | Ghost Model Context Protocol (MCP) server |
| `password` | Reset the password for a database |
| `pause` | Pause a running database |
| `payment` | Manage payment methods |
| `pricing` | Show dedicated database pricing |
| `psql` | Connect to a database using psql |
| `rename` | Rename a database |
| `resume` | Resume a paused database |
| `schema` | Display database schema information |
| `share` | Share a database |
| `sql` | Execute SQL query on a database |
| `status` | Show space usage |
| `upgrade` | Upgrade the ghost CLI to the latest version |
| `version` | Show version information |

Run `ghost [command] --help` for more information about a command.

## MCP

The `ghost mcp` command installs a [Model Context Protocol](https://modelcontextprotocol.io) server so AI assistants like Claude can manage and query your databases directly.

| Tool | Description |
|------|-------------|
| `ghost_connect` | Get a connection string for a database |
| `ghost_create` | Create a new database |
| `ghost_create_dedicated` | Create a new dedicated database |
| `ghost_delete` | Delete a database permanently |
| `ghost_feedback` | Submit feedback, a bug report, or a support request |
| `ghost_fork` | Fork a database |
| `ghost_fork_dedicated` | Fork a database as a new dedicated instance |
| `ghost_invoice` | Get invoice line-item detail |
| `ghost_invoice_list` | List invoices |
| `ghost_list` | List all databases |
| `ghost_login` | Authenticate with GitHub OAuth |
| `ghost_logs` | View logs for a database |
| `ghost_password` | Reset the password for a database |
| `ghost_pause` | Pause a running database |
| `ghost_pricing` | Get dedicated database pricing |
| `ghost_rename` | Rename a database |
| `ghost_resume` | Resume a paused database |
| `ghost_schema` | Display database schema information |
| `ghost_share` | Share a database |
| `ghost_share_list` | List database shares |
| `ghost_share_revoke` | Revoke a database share |
| `ghost_sql` | Execute a SQL query against a database |
| `ghost_status` | Show space usage |
| `search_docs` | Search PostgreSQL, PostGIS, and TimescaleDB documentation |
| `view_skill` | Retrieve skills for PostgreSQL and TimescaleDB best practices |

Run `ghost mcp list` to see the full list of available tools and prompts, or `ghost mcp get <name>` for details on a specific one.

## Contributing

Bug reports and feature requests are welcome — please [open an issue](../../issues).

## License

Licensed under [Apache 2.0](./LICENSE).
