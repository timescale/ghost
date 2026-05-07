# How to Analyze a Dataset with Ghost and an AI Agent

Analyzing data is usually a chore: load the CSV into a spreadsheet, clean up the data, and then do custom analyses.

Or, if you were more ambitious: create a database, define a schema, load the data into that database, and then write and run your own SQL against that data. Or even write a python script and use pandas on top of that database.

Luckily today an AI agent can do all of this work if it has the right database to talk to. Ghost gives it one.

## Who this is for

This guide is for developers and analysts who use an AI coding agent (Claude Code, Cursor, Codex, Windsurf, etc.) and want to point it at a real Postgres database for analysis. You do not need to be comfortable reading SQL, but it helps if you are.

You'll need:

- An AI coding agent **with both MCP support and a Bash/shell tool** (Claude Code, Cursor in agent mode, Codex, Windsurf, Gemini CLI, VS Code, Kiro, or Antigravity). The shell tool is what lets the agent download the data and run `psql` on your behalf — most modern agents have this.
- macOS, Linux, or Windows
- An internet connection. The agent will install everything else (`psql`, `unzip` if needed) on its own.

## What is Ghost

**Ghost is Postgres for builders and their agents.** Unlimited databases, metered by hours of active compute. All via CLI and MCP, no GUI required.

Create one in seconds, fork it like git when you want to experiment safely, share it with a simple link like a Google doc. Graduate to production with one command or throw it away when you're done.

The free tier covers 100 active compute hours per month and 1TB of storage. Compute is metered when something actually queries the database, in 15 minute intervals. Unused databases automatically stop burning compute hours. 

## What you will do

In this guide, we will analyze MovieLens movie-ratings data using Ghost and an AI agent. After a one-time bootstrap, **the agent does everything else** — you just paste prompts.

1. **Bootstrap (you):** install Ghost, log in, configure the MCP server in your agent.
2. **Download the dataset (agent):** fetch and extract the MovieLens 100K CSVs.
3. **Install `psql` (agent):** check for it and install it if missing.
4. **Create the database and load the data (agent):** create a Ghost database, infer a schema from the CSVs, and load them with `psql \copy`.
5. **Analyze (agent):** answer three analytical questions in natural language.
6. **Clean up (agent):** delete the database.

## Step 1 — Bootstrap Ghost (you, one-time)

Install the `ghost` CLI:

```bash
curl -fsSL https://install.ghost.build | sh
```

On Windows, run `irm https://install.ghost.build/install.ps1 | iex` in PowerShell.

Log in:

```bash
ghost login
```

This opens your browser to authenticate. Once you're back at the terminal, configure Ghost as an MCP server in your agent. For Claude Code:

```bash
ghost mcp install claude-code
```

Replace `claude-code` with `cursor`, `codex`, `windsurf`, `gemini`, `vscode`, `kiro-cli`, or `antigravity` if you use a different agent. Run `ghost mcp install` with no argument for an interactive picker.

**Expected output:**

```
Installed Ghost MCP server for claude-code
Backup saved to ~/.claude.json.bak
```

**Restart your agent** so it picks up the new MCP server. The agent now has tools available, including `ghost_create`, `ghost_list`, `ghost_connect`, `ghost_sql`, `ghost_schema`, `ghost_fork`, and `ghost_delete`. From here on the agent does the work — you paste prompts and approve tool calls.

## Step 2 — Download the dataset

Tell the agent:

```
Make a working directory called `movielens-tutorial` and `cd` into it. Download the MovieLens 100K dataset from `https://files.grouplens.org/datasets/movielens/ml-latest-small.zip` with `curl`, then extract it. If `unzip` isn't available, fall back to `python3 -m zipfile -e ml-latest-small.zip .`. Confirm the four CSVs (`movies.csv`, `ratings.csv`, `tags.csv`, `links.csv`) are present.
```

The agent will use its Bash tool to run `curl -O ...` and `unzip ...` (or the Python fallback), then `ls` to confirm.

**Expected output** (the agent's final `ls`):

```
README.txt  links.csv  movies.csv  ratings.csv  tags.csv
```

This is the small MovieLens snapshot: 100,836 ratings of 9,742 movies by 610 users. Four CSVs, no API key, no signup.

## Step 3 — Make sure `psql` is installed

You'll need `psql`, the Postgres command-line client, for the load step.

Tell the agent:

```
Check if `psql` is installed by running `psql --version`. If it's missing, detect my platform (use `uname -s`, plus `/etc/os-release` if Linux) and install the right client:

- macOS: `brew install libpq && brew link --force libpq`
- Debian/Ubuntu: `sudo apt update && sudo apt install -y postgresql-client`
- Fedora/RHEL: `sudo dnf install -y postgresql`
- Windows: `winget install PostgreSQL.PostgreSQL`

Re-run `psql --version` to confirm.
```

**Expected output** (the version doesn't matter — anything 13 or newer works):

```
psql (PostgreSQL) 16.4
```

Why `psql` and not just the agent's MCP tools? `psql`'s `\copy` meta-command streams a local file to the server in one operation. The agent's `ghost_sql` tool runs SQL on the server, so it can't see files on your local disk.

## Step 4 — Create the database and load the data

Tell the agent:

```
Create a new Ghost database called `movielens` and wait for it to be ready. Then inspect the four CSVs in the `movielens-tutorial/` directory (`movies.csv`, `ratings.csv`, `tags.csv`, `links.csv`), infer an appropriate Postgres schema for each, and create the tables. Once the schema is in place, get the connection string with `ghost connect movielens` and load each CSV using `psql \copy ... WITH (FORMAT csv, HEADER true)` (run psql from inside `movielens-tutorial/` so the file paths resolve). Show me the row counts when you're done.
```

The agent will:

1. Call `ghost_create` to provision the database.
2. Read the CSV headers (and a few sample rows) to figure out column types.
3. Call `ghost_sql` four times to run the `CREATE TABLE` statements.
4. Get the connection string with `ghost connect movielens`.
5. Run four `psql \copy` commands to load the data.

Approve each tool call.

**Expected output** (abbreviated, in order):

```
ghost_create  → created database 'movielens' (status: running)
ghost_sql     → CREATE TABLE
ghost_sql     → CREATE TABLE
ghost_sql     → CREATE TABLE
ghost_sql     → CREATE TABLE
COPY 9742
COPY 100836
COPY 3683
COPY 9742
```

The four `COPY N` lines are your row counts: 9,742 movies, 100,836 ratings, 3,683 tags, 9,742 links. You can confirm independently in another terminal with `ghost list`:

```
NAME        STATUS    AGE
movielens   running   1m
```

## Step 5 — Ask the agent three analytical questions

This is the payoff. Each question is a separate prompt; the agent translates to SQL, runs it via `ghost_sql`, and prints the results.

### Question 1 — Top 10 movies (with at least 50 ratings)

Tell the agent:

```
What are the top 10 movies by average rating, considering only movies with at least 50 ratings?
```

The agent will run something like:

```sql
SELECT m.title,
       ROUND(AVG(r.rating)::numeric, 2) AS avg_rating,
       COUNT(*) AS n_ratings
FROM movies m
JOIN ratings r ON r.movieId = m.movieId
GROUP BY m.movieId, m.title
HAVING COUNT(*) >= 50
ORDER BY avg_rating DESC, n_ratings DESC
LIMIT 10;
```

**Expected output** (abbreviated):

```
              title              | avg_rating | n_ratings
---------------------------------+------------+-----------
 Shawshank Redemption, The (1994)|       4.43 |       317
 Godfather, The (1972)           |       4.29 |       192
 Fight Club (1999)               |       4.27 |       218
 ...
```

### Question 2 — Most polarizing movies

Tell the agent:

```
Find the 10 most polarizing movies — the ones with the highest standard deviation in rating, with at least 100 ratings.
```

The agent will run something like:

```sql
SELECT m.title,
       ROUND(STDDEV_SAMP(r.rating)::numeric, 2) AS stddev,
       COUNT(*) AS n_ratings
FROM movies m
JOIN ratings r ON r.movieId = m.movieId
GROUP BY m.movieId, m.title
HAVING COUNT(*) >= 100
ORDER BY stddev DESC
LIMIT 10;
```

**Expected output** (abbreviated):

```
              title              | stddev | n_ratings
---------------------------------+--------+-----------
 Pulp Fiction (1994)             |   1.18 |       307
 Fight Club (1999)               |   1.15 |       218
 ...
```

A high standard deviation means viewers split sharply for and against the movie — useful when you want to find debate-worthy films, not just universally-loved ones.

### Question 3 — Co-rated pairs (the complicated one)

Tell the agent:

```
Find the top 5 movie pairs that are most often rated 4 or higher by the same user. Return both titles and the count of users who rated both highly.
```

This is the hardest of the three — it requires a self-join on ratings, deduplicated so each pair (A, B) doesn't also show up as (B, A). The agent should produce something like:

```sql
WITH high_ratings AS (
  SELECT userId, movieId
  FROM ratings
  WHERE rating >= 4.0
)
SELECT m1.title AS movie_a,
       m2.title AS movie_b,
       COUNT(*) AS users_who_rated_both_highly
FROM high_ratings r1
JOIN high_ratings r2
  ON r1.userId = r2.userId
 AND r1.movieId < r2.movieId
JOIN movies m1 ON m1.movieId = r1.movieId
JOIN movies m2 ON m2.movieId = r2.movieId
GROUP BY m1.title, m2.title
ORDER BY users_who_rated_both_highly DESC
LIMIT 5;
```

The `r1.movieId < r2.movieId` predicate is what keeps each pair from being counted twice.

**Expected output** (abbreviated):

```
            movie_a              |             movie_b              | users_who_rated_both_highly
---------------------------------+----------------------------------+-----------------------------
 Pulp Fiction (1994)             | Shawshank Redemption, The (1994) |                         128
 Forrest Gump (1994)             | Pulp Fiction (1994)              |                         117
 ...
```

If a result looks off, ask the agent to explain its query — that's part of the point of using an agent for analysis.

## Step 6 — Delete the database

> **Warning:** Deletion is irreversible. The database, all loaded data, and any forks are deleted permanently. If you want to keep the data but stop paying for active compute, ask the agent to run `ghost pause movielens` instead.

Tell the agent:

```
Delete the `movielens` Ghost database. Confirm with me first before running it.
```

The agent will use `ghost_delete` with `confirm: true` after you OK the tool call.

**Expected output:**

```
Deleted database 'movielens'.
```

You're done.

## Wrap-up

You now have a repeatable workflow for letting an AI agent do real Postgres analysis end-to-end. After a one-time bootstrap, the agent handles everything. You learned how to:

- Install Ghost and configure its MCP server in your agent
- Ask analytical questions in plain English and have the agent run the SQL
- Clean up cleanly when you're done

## What to do next

- Try a larger dataset — the [full MovieLens dataset](https://grouplens.org/datasets/movielens/) is 33M ratings, or try [NYC taxi trips](https://www.nyc.gov/site/tlc/about/tlc-trip-record-data.page) for time-series and geo work.
- Tell the agent to `ghost_fork` the database with a new name to try transformations on a copy without touching the original.
- Ask the agent to build a materialized view or index for one of the slower queries above and measure the speedup.

## Make Ghost your default database!

Tell your agent:

```
Default Postgres to Ghost. Whenever you need a database for prototyping, tests, analysis, or schema experiments, create one in Ghost, fork it before risky changes, and delete it when you're done.
```

## FAQ

**Is a Ghost database real Postgres?**

Yes — full Postgres, not a wrapper or simulation. You can connect with `psql`, JDBC, `pg` (Node), SQLAlchemy, or any other client. The MCP server is one way in; it isn't the only way. `ghost connect <name>` gives you a standard `postgres://` connection string.

**Why use `psql \copy` instead of letting the agent INSERT?**

`\copy` is a `psql` client meta-command that streams a local file to the server in one operation. The agent's `ghost_sql` tool runs SQL on the server, so it can't see files on your local disk. The agent could generate 100,000 INSERT statements, but that's slow, expensive in tokens, and unnecessary. Bridging the load step with `psql \copy` is the right tool for the job.

**Does this work with Cursor, Codex, Windsurf, or other agents?**

Yes. `ghost mcp install` supports `claude-code`, `cursor`, `codex`, `windsurf`, `gemini`, `vscode`, `kiro-cli`, and `antigravity`. Run it with no argument for an interactive picker. The MCP tools the agent gets are the same regardless of which client you use. The one extra requirement for this tutorial is that your agent has a Bash/shell tool too, so it can run `curl`, `unzip`, and `psql` on your behalf.

**What if I want to keep the data instead of deleting?**

Skip step 6. Or ask the agent to run `ghost pause movielens` to stop charging compute while keeping the data on disk; resume with `ghost resume movielens` when you want it back. Ghost's free tier covers 100 active compute hours per month and 1TB of storage. Paused databases burn storage only, not compute hours, and Ghost meters by active compute hour with hard spending caps you can set via `ghost config` — so a forgotten paused database can't run up a surprise bill.

## Resources

- [Ghost docs](https://ghost.build/docs/)
- [MovieLens dataset](https://grouplens.org/datasets/movielens/) (GroupLens, University of Minnesota)
- [Ghost MCP install reference](https://ghost.build/docs/cli/ghost_mcp_install/) — `ghost mcp install --help` also works locally
- [Ghost free tier and pricing](https://ghost.build/)