# Ship an app on Ghost + Fly.io for ~$2/month

Putting a real public app on the internet shouldn't cost $25/month for managed Postgres alone — before you've added compute or shipped a feature. Ghost gives you the database, Fly.io gives you the host, and your AI agent does the plumbing.

You can launch a public-facing, sparse-traffic hobby app, backed by Postgres, for roughly the cost of a coffee per month.

## Who this is for

This guide is for developers who use an AI coding agent (Claude Code, Cursor, Codex, Windsurf, etc.) and want to ship a small public app fast and cheap. You don't need to know SQL or Docker — the agent handles both — but you should be comfortable approving shell commands the agent runs on your behalf.

You'll need:

- An AI coding agent **with both MCP support and a Bash/shell tool** (Claude Code, Cursor in agent mode, Codex, Windsurf, Gemini CLI, VS Code, Kiro, or Antigravity). The shell tool is what lets the agent run `flyctl` and `npm` on your behalf — most modern agents have this.
- macOS, Linux, or Windows (WSL recommended on Windows for `flyctl`).
- A Fly.io account with a credit card on file. 
- An internet connection. The agent will install everything else (`flyctl`, Node, etc.) on its own.

## What is Ghost

**Ghost is Postgres for builders and their agents.** Unlimited databases, metered by hours of active compute. All via CLI and MCP, no GUI required.

Create one in seconds, fork it like git when you want to experiment safely, share it with a simple link like a Google doc. Graduate to production with one command or throw it away when you're done.

The free tier covers 100 active compute hours per month and 1TB of storage. Compute is metered in 15-minute chunks when something queries the database; an idle database burns no compute. A sparse-traffic hobby app — a handful of human visits a day — comfortably fits the free tier.

You can do this with managed-Postgres alternatives like Neon, Supabase, or RDS — but those either charge a flat monthly fee, cap project counts, or push you through a GUI for changes the agent could otherwise make in seconds. Ghost is the cheapest, most agent-native way to ship a public app with real Postgres.

## What you will do

In this guide, we'll deploy a public-facing todo app to Fly.io with a Ghost Postgres database. After a one-time bootstrap, **the agent does everything else** — you just paste prompts.

1. **Bootstrap (you):** install the Ghost CLI and `flyctl`, log in, configure the Ghost MCP server in your agent.
2. **Scaffold the app + create the database + define the schema (agent):** generate a small Express todo app, create a Ghost database, define a `todos` table.
3. **Wire the app to the database and test locally (agent):** set `DATABASE_URL`, run the app on `localhost`, round-trip a todo through the database.
4. **Deploy to Fly.io (agent):** create the Fly app, push the connection string as a secret, deploy to a `*.fly.dev` URL.
5. **Verify the public app (agent):** curl the live URL, add a todo over HTTPS, confirm it landed in Ghost.
6. **Open it in your browser (you):** use the live app yourself and share the URL.
7. **Clean up (agent):** destroy the Fly app and delete the Ghost database.

## Step 1 — Bootstrap (you, one-time)

This is the only part you can't delegate.

Install the `ghost` CLI:

```bash
curl -fsSL https://install.ghost.build | sh
```

On Windows, run `irm https://install.ghost.build/install.ps1 | iex` in PowerShell.

Install `flyctl`:

```bash
curl -L https://fly.io/install.sh | sh
```

On Windows, run `pwsh -Command "iwr https://fly.io/install.ps1 -useb | iex"`.

Log into both:

```bash
ghost login
flyctl auth login
```

Each opens your browser. `flyctl auth login` prompts you to add a credit card if you haven't yet.

Configure Ghost as an MCP server in your agent. For Claude Code:

```bash
ghost mcp install claude-code
```

Replace `claude-code` with `cursor`, `codex`, `windsurf`, `gemini`, `vscode`, `kiro-cli`, or `antigravity` if you use a different agent. Run `ghost mcp install` with no argument for an interactive picker.

Restart your agent so it picks up the new MCP server.

**Expected output:**

```
$ ghost --version
ghost version 1.x.x

$ flyctl version
flyctl v0.x.x ...
```

Once both CLIs are installed, you're logged in, and the agent has been restarted, the rest is the agent.

## Step 2 — Scaffold the app, create the database, define the schema (agent)

Tell the agent:

```
Build me a minimal public-facing todo app I can deploy to Fly.io.

Create a fresh empty directory called `todo-app` and work inside it.

Stack: Node.js with Express and the `pg` package. One server file. Server-rendered HTML — no frontend framework. Read DATABASE_URL from the environment.

Routes:
- GET  /                  render the list of todos with a small form to add a new one
- POST /todos             insert a new todo from form data, then redirect to /
- POST /todos/:id/done    mark a todo done, then redirect to /

Files to write:
- package.json             express + pg + dotenv
- server.js                the app
- Dockerfile               minimal Node runtime, copies package.json + server.js, runs `node server.js`
- fly.toml                 app = "todo-app", primary_region = "iad", [http_service] with internal_port=3000, force_https=true, auto_stop_machines="stop", auto_start_machines=true, min_machines_running=0. No [[services]] block. No [[mounts]]. No managed Postgres.
- .dockerignore            node_modules, .env, .git

Then, using the Ghost MCP:
1. Create a new Ghost database called "todo-app". Wait for it to be ready.
2. Create a `todos` table with columns: id (serial primary key), text (text not null), done (boolean default false), created_at (timestamptz default now()).
3. Print the connection string so I can use it in the next step.

Don't use a migration framework. Don't add auth. Keep server.js under 100 lines.
```

The agent will:

1. Write `package.json`, `server.js`, `Dockerfile`, `fly.toml`, `.dockerignore`, and minimal HTML.
2. Create the Ghost database.
3. Create the `todos` table.
4. Print the connection string.

**Expected output:**

```
Database "todo-app" created (status: running).
Table "todos" created with 4 columns.
Connection: postgres://tsdbadmin:...@...tsdb.cloud.timescale.com:.../tsdb?sslmode=require
```

You now have a Postgres database in the cloud and a tiny app on disk — including a Dockerfile and fly.toml — ready for deployment.

## Step 3 — Wire the app to the database and test locally (agent)

Tell the agent:

```
Wire the app to the Ghost database we just created.

1. Write a `.env` file with DATABASE_URL set to the connection string from the previous step. Add `.env` to `.gitignore`.
2. Make sure server.js loads .env (use the `dotenv` package).
3. SSL setup for Timescale: recent `pg` versions treat `sslmode=require` in the URL as `verify-full`, which rejects Timescale's cert chain and crashes on the first query. Strip the `sslmode` query param from DATABASE_URL before passing it to `new Pool({ ... })`, and pass `ssl: { rejectUnauthorized: false }` in the Pool config.
4. Run `npm install` and start the server on port 3000 in the background.
5. Use curl to: GET /, POST a todo with text="Ship the app", GET / again, then POST /todos/1/done.
6. Print the response bodies so I can see the todo round-tripping through the database.
7. Stop the local server.
```

The agent will:

1. Write `.env` and update `.gitignore`.
2. Install dependencies.
3. Start `node server.js` in the background.
4. Run a sequence of `curl` commands.
5. Kill the local process.

**Expected output:**

```
$ curl localhost:3000
<html>...<h1>Todos</h1><form action="/todos" method="post">...

$ curl -X POST -d 'text=Ship the app' localhost:3000/todos
(302 redirect to /)

$ curl localhost:3000
<html>...<li>Ship the app <form action="/todos/1/done"...

$ curl -X POST localhost:3000/todos/1/done
(302 redirect to /)
```

The app works end-to-end against your Ghost database. Time to put it on the internet.

## Step 4 — Deploy to Fly.io (agent)

> **Warning:** This step creates a billable Fly.io app on a public URL. With auto-stop machines enabled (configured in step 2's `fly.toml`), an idle app costs only for storage and bandwidth — typically cents per month — but charges accrue once the machine is running. Make sure you're comfortable with Fly's pay-as-you-go pricing before deploying.

Tell the agent:

```
Deploy the app to Fly.io. Skip `flyctl launch` entirely — we already have a Dockerfile and fly.toml from step 2, and `flyctl launch --yes` has a habit of provisioning unwanted Fly Postgres clusters and overwriting DATABASE_URL.

1. Pick a globally unique app name. Start with "todo-app" and append a 6-char random suffix if Fly says it's taken (e.g. "todo-app-a1b2c3").
2. Update `app = ` in fly.toml to that name.
3. Create the app: `flyctl apps create <name>`.
4. Set DATABASE_URL as a Fly secret using the connection string from step 2: `flyctl secrets set DATABASE_URL="<connection string>" --app <name>`.
5. Deploy: `flyctl deploy --ha=false`. Wait for it to finish.
6. After deploy, force a single machine: `flyctl scale count 1 --app <name> --yes`. (Fly's first deploy sometimes creates two machines despite `--ha=false`; this keeps it to one so the auto-stop story stays honest.)
7. Print the public URL.
```

The agent will:

1. Pick an app name and update `fly.toml`.
2. Run `flyctl apps create`.
3. Set the secret.
4. Run `flyctl deploy --ha=false` and capture the URL.
5. Run `flyctl scale count 1`.

**Expected output:**

```
==> Building image
...
==> Pushing image to fly
...
==> Monitoring deployment
 ✔ [job] update succeeded

Visit your newly deployed app at https://todo-app-<suffix>.fly.dev/
```

Your app is live on the public internet, talking to your Ghost database.

## Step 5 — Verify the public app (agent)

Tell the agent:

```
Verify the deployed app works against the Ghost database.

1. curl the public URL and confirm it renders the todos page.
2. Submit a new todo via curl: POST /todos with text="Hello from the internet".
3. curl the public URL again and confirm the new todo shows up.
4. Use the Ghost MCP to run `SELECT * FROM todos ORDER BY id` and show me the rows directly from the database.
```

The agent will:

1. `curl https://todo-app-<suffix>.fly.dev/`
2. `curl -X POST -d 'text=Hello from the internet' https://todo-app-<suffix>.fly.dev/todos`
3. `curl https://todo-app-<suffix>.fly.dev/`
4. Run the SELECT through the Ghost MCP.

**Expected output:**

```
 id |          text           | done |          created_at
----+-------------------------+------+-------------------------------
  1 | Ship the app            | t    | 2026-05-07 10:42:15.123+00
  2 | Hello from the internet | f    | 2026-05-07 10:48:03.456+00
(2 rows)
```

The row added through the public HTTPS URL is sitting in your Ghost database. You shipped a public-facing, Postgres-backed app.

## Step 6 — Open it in your browser (you)

Click the `https://todo-app-<suffix>.fly.dev/` URL printed at the end of step 4 (or paste it into your browser).

Add a few todos through the form. Mark some done. Refresh the page — your todos persist across reloads because they're sitting in Ghost. Send the URL to a friend; it works for them too. It's on the public internet.

**Expected output:**

A working todo app in your browser, with todos that survive a refresh.

## Step 7 — Clean up (agent)

> **Warning:** `flyctl apps destroy` and Ghost's `delete` are irreversible. The Fly app, all its history, and the Ghost database (including all data) are gone. The agent will run these on your behalf — don't approve unless you mean it.

Tell the agent:

```
Tear down everything we created so I'm not paying for an idle Fly machine or holding a Ghost database I don't need.

1. Run `flyctl apps destroy <app-name> --yes` to delete the Fly app.
2. Use the Ghost MCP to delete the "todo-app" database.
3. Confirm both are gone: `flyctl apps list` should not list the app, and `ghost list` should not list the database.
```

**Expected output:**

```
$ flyctl apps destroy todo-app-<suffix> --yes
Destroyed app todo-app-<suffix>

$ ghost delete todo-app
Deleted database "todo-app".

$ flyctl apps list
NAME    OWNER   STATUS  ...
(no todo-app)

$ ghost list
NAME  STATUS  ...
(no todo-app)
```

You're back to zero cost on both sides.

## Wrap-up

You shipped a public-facing, Postgres-backed app for the price of a coffee. You learned how to:

- Bootstrap Ghost and Fly.io for an agent-driven workflow.
- Use Ghost's MCP server to create databases and run SQL from inside your agent.
- Deploy a Node.js app to Fly.io with auto-stop machines so you only pay for active compute.
- Connect a Fly app to a Ghost database via a `DATABASE_URL` secret.
- Tear it all down cleanly.

## What to do next

- Add a custom domain with `flyctl certs add yourdomain.com`.
- Add authentication — a session-cookie + bcrypt setup is enough for a small app.
- Try `ghost fork` before your next schema change — Ghost lets you run a risky migration on a copy of the database first, then apply the same migration to production once you're happy.
- Read the Fly docs on [auto-stop machines](https://fly.io/docs/launch/autostop-autostart/) to understand how the scale-to-zero billing works.

## Make Ghost your default Postgres!

Tell your agent:

```
When I'm prototyping or shipping a small app and need a Postgres database, default to Ghost. Use Ghost MCP tools to create / fork / delete databases and to run SQL. Don't suggest local Postgres setup or other managed-Postgres services unless I ask.
```

## FAQ

**Does this work with my agent?**

Yes, as long as it has both MCP support and a Bash/shell tool. Confirmed-working agents include Claude Code, Cursor (in agent mode), Codex, Windsurf, Gemini CLI, VS Code with Copilot, Kiro, and Antigravity. The shell tool is what lets the agent run `flyctl deploy` and `npm install` for you — without it, the agent can talk to Ghost via MCP but can't deploy.

**How much does this actually cost?**

Ghost is free for a sparse-traffic hobby app: 100 active compute hours per month and 1TB of storage. Ghost meters in 15-minute chunks when something queries the database, and idle databases don't burn compute. A handful of human visits per day fits comfortably.

The failure mode to watch for: Ghost meters per 15-minute chunk, so anything that hits the database every 15 minutes — uptime monitors, health checks, aggressive bots, link previewers — can keep the meter running 24/7. That's ~720 hours/month, well past the 100-hour free tier, and works out to roughly $46/month at $0.075/CPU-hr. If your app needs constant availability, switch to Ghost's $10/month dedicated tier (always-on, no auto-pause).

Fly.io is no longer free as of late 2024. With auto-stop enabled (which we configured in step 4), an idle app costs only for storage and bandwidth — typically cents per month. A small `shared-cpu-1x` machine running 24/7 is around $2/month, and auto-stop means most hobby apps spend most of their time at zero compute.

**Why not just use SQLite on a Fly volume?**

SQLite + Fly volumes is a legitimate cheaper option for one-machine apps. You give up real concurrent writes, Postgres's type system and extensions (full-text search, JSONB, time-series, PostGIS, etc.), and the ability to scale to multiple Fly regions without painful litestream/LiteFS setups. You also can't `psql` into a SQLite file from your laptop while debugging. For anything you'd want to grow into a real product, Postgres is worth the small extra setup — and with Ghost it's not a meaningful extra cost.

## Resources

- [Ghost docs](https://docs.ghost.build) — full CLI and MCP reference.
- [Fly.io launch guide](https://fly.io/docs/launch/) — deployment basics, including Dockerfile detection.
- [Fly.io auto-stop machines](https://fly.io/docs/launch/autostop-autostart/) — how scale-to-zero billing works.
- [How to Analyze a Dataset with Ghost and an AI Agent](movielens-analysis.md) — same agent-driven shape, focused on data analysis.