# `ghost serve` — local web UI for running SQL against ghost databases

## Goal

Add a `ghost serve` command that launches a local web server on `127.0.0.1:<port>` and opens the browser to a small React UI. The UI shows a database picker plus the unmodified PopSQL query widget, so the user can run ad-hoc SQL against any of their ghost databases. No new auth layer: the local server reuses the CLI's existing `gt_…` API key for ghost-api metadata calls, and the query path connects directly to the user's PostgreSQL databases using credentials the CLI already resolves (via `common.GetPassword`).

```
ghost serve [--port <n>] [--host 127.0.0.1] [--no-open]
```

This is patterned after `memory-engine`'s `me serve` (PR #47), but query execution runs **in-process** inside the Go binary — there is no proxy to ghost-api or the savannah gateway. The CLI binary becomes a self-contained "query gateway" that speaks the widget's wire protocol.

---

## Architecture

```
                  ┌──────────────────────────────────────────┐
   browser  ──▶   │  ghost serve  (single Go binary)         │
   (Vite-built    │  • 127.0.0.1:<port>                      │
    SPA, served   │  • embed.FS static                       │
    from same     │  • /api/databases   → ghost-api (REST)   │  ──▶ ghost-api
    origin)       │  • /api/bootstrap   → local config       │      (only used
                  │  • /api/executeQuery, /api/arrowResults, │      for listing
                  │    /api/cancelRun   → run pg queries     │      databases &
                  │                       in-process         │      fetching
                  │  • Apache Arrow IPC encoder              │      passwords)
                  │  • pgx/v5 connections                    │
                  │      │                                   │
                  └──────┼───────────────────────────────────┘
                         │
                         ▼
                  user's ghost Postgres DBs (TLS, direct)
```

No ghost-api changes are required. No new endpoints in any other repo. The widget thinks it's talking to a savannah gateway; in reality the gateway is a few hundred lines of Go in this binary.

---

## The widget's wire protocol (what we have to implement)

This is what `@popsql/query-client`'s `TimescaleQueryClient` actually sends. Source: `popsql/packages/popsql-query-client/src/{TimescaleQueryClient.ts,client.ts}`.

Auth: `credentials: 'include'` (cookies — but we have none on localhost) plus optional `Authorization: Bearer <accessToken>`. We don't need either — the listener is loopback-only.

### One-shot mode (no `sessionKey` prop on `<QueryWidget>`) — **MVP scope**

Two endpoints participate in a single query run. The widget fires these in sequence as it streams the response.

**`POST /api/executeQuery`**
Request body:
```json
{
  "projectId": "<space id>",
  "serviceId": "<database id>",
  "query": "select 1",
  "runId": "<uuid v4 from widget>",
  "stream": true,
  "persist": false,
  "statements": null,
  "timeout": null
}
```
Response: `Content-Type: application/x-ndjson` (or `application/json` — the widget just reads lines), one JSON object per line, in order:
1. `{ "columns": [{ "name": "…", "type": "…", … }], "meta": { … } }` — emitted as soon as the column metadata is known.
2. As soon as the widget reads the `columns` line, it fires `POST /api/arrowResults` in parallel (see below). The original `executeQuery` response stays open.
3. Terminator: exactly one of
   - `{ "success": true, "rowCount": 42, "duration": 123, … }`
   - `{ "success": false, "error": { "message": "…", "cancel"?: true, "timeout"?: true, "fatal"?: true } }`

`fatal: true` causes the widget to mark the session as broken (only meaningful in session mode); we'll never set it. `cancel: true` is set when the run was cancelled via `/api/cancelRun`. `timeout: true` for query timeouts.

**`POST /api/arrowResults`**
Request body:
```json
{
  "projectId": "<space id>",
  "serviceId": "<database id>",
  "runId": "<same runId from executeQuery>"
}
```
Response: `Content-Type: application/vnd.apache.arrow.stream`, body is a raw Apache Arrow **IPC stream** (schema message + zero or more record-batch messages). The widget parses with `RecordBatchReader.from(response)` and renders incrementally.

**`POST /api/cancelRun`** (used for "stop the running query")
```json
{ "projectId": "…", "serviceId": "…", "runId": "…" }
```
Soft-cancel: the widget mostly relies on `AbortController` to drop the executeQuery connection. This endpoint is wired but rarely called (see `TimescaleQueryClient.cancelQuery` comment). We'll implement it as best-effort `pg_cancel_backend()` against the run's connection.

### Session mode — **deferred to follow-up**

The widget also supports a "session" mode (`sessionKey` prop set on `<QueryWidget>`) where a long-lived PG connection is reused across multiple `executeSessionQuery` calls. This adds four more endpoints (`createSession`, `sessionEvents` [NDJSON status stream], `executeSessionQuery`, `closeSession`) and a session lifecycle manager.

For MVP we will **not pass `sessionKey`** to `<QueryWidget>`. Each query opens its own connection and closes it. Sessions can be a phase-2 add — the architecture leaves room for them.

---

## Apache Arrow encoding (PG row → Arrow IPC)

The only non-trivial new piece. Plan:

- Add `github.com/apache/arrow-go/v18` (or whatever the current canonical Go Arrow module is). It's well-maintained and supports the IPC stream writer (`ipc.NewWriter` over an `io.Writer`).
- Use **`github.com/jackc/pgx/v5`** for the Postgres connection (already idiomatic; the CLI may already have it transitively). `pgx` exposes column OIDs which we map to Arrow types cleanly. `database/sql` only gives Go reflect types, which is lossier.

Type mapping (MVP — narrow enough to cover the common Postgres/Timescale surface):

| Postgres OID                       | Arrow type                          |
|------------------------------------|-------------------------------------|
| `bool`                             | `Boolean`                           |
| `int2`                             | `Int16`                             |
| `int4`                             | `Int32`                             |
| `int8`                             | `Int64`                             |
| `float4`                           | `Float32`                           |
| `float8`                           | `Float64`                           |
| `numeric`                          | `Utf8` (lose precision-aware decimal for now) |
| `text`, `varchar`, `name`, `bpchar`| `Utf8`                              |
| `bytea`                            | `Binary`                            |
| `date`                             | `Date32`                            |
| `timestamp`                        | `Timestamp[us]` (no tz)             |
| `timestamptz`                      | `Timestamp[us, UTC]`                |
| `uuid`                             | `Utf8` (16-byte FixedSizeBinary later) |
| `json`, `jsonb`                    | `Utf8`                              |
| arrays of the above                | `List<…>`                           |
| anything else                      | `Utf8` (via pgx's text protocol)    |

Batching: send a single record batch for MVP (the table widget is happy with one batch; we can split later for very large results).

---

## Repository changes (this repo)

```
ghost_serve/
  internal/cmd/
    serve.go                          NEW – Cobra command
    serve_test.go                     NEW
  internal/serve/
    server.go                         NEW – net/http server + mux + listener
    assets.go                         NEW – embed.FS resolver (SPA fallback, cache headers)
    bootstrap.go                      NEW – GET /api/bootstrap (projectId, version)
    databases.go                      NEW – GET /api/databases (ghost-api passthrough)
    execute.go                        NEW – POST /api/executeQuery (NDJSON producer)
    arrow.go                          NEW – POST /api/arrowResults (Arrow IPC producer)
    cancel.go                         NEW – POST /api/cancelRun
    runs.go                           NEW – in-memory Run store keyed by runId
    pgtypes.go                        NEW – PG OID → Arrow type mapping + value coercion
    wire.go                           NEW – Go types matching widget request/response shapes
    server_test.go                    NEW
    assets_test.go                    NEW
    execute_test.go                   NEW – uses pgx + a real test PG or pgmock
    arrow_test.go                     NEW – validates IPC output via apache-arrow Go reader
    web/                              NEW – embed root; web/dist contents land here
      .gitkeep
  web/                                NEW – Vite app workspace (not Go)
    package.json
    vite.config.ts
    tsconfig.json
    index.html
    .gitignore                        (dist/, node_modules/)
    src/
      main.tsx
      app.tsx
      styles.css
      components/
        DatabasePicker.tsx
        QueryPanel.tsx
        Header.tsx
      api/
        bootstrap.ts                  (fetch /api/bootstrap once)
        databases.ts                  (fetch /api/databases)
      lib/
        url-state.ts                  (?db=<id>)
  scripts/
    build-web.sh                      NEW – npm ci && npm run build && copy to internal/serve/web/
  check                               UPDATED – run build-web.sh before go install
  .github/workflows/*.yml             UPDATED – build web before go build/release
  Dockerfile                          UPDATED – multi-stage node + go build
  docs/cli/ghost_serve.md             AUTO-GENERATED via cmd/generate-docs
  README.md                           UPDATED – Commands table + Usage examples
  CLAUDE.md                           UPDATED – describe internal/serve/ and web/
```

---

## The CLI command (`ghost serve`)

`internal/cmd/serve.go`, all standard patterns from `CLAUDE.md`:

- `SilenceUsage: true`, `RunE`, `ValidArgs: cobra.NoFileCompletions`.
- Gated behind `GHOST_EXPERIMENTAL` until polished, then promoted.
- Flags:
  - `--port int` — explicit port; default `0` → kernel picks via `net.Listen("tcp", "127.0.0.1:0")`.
  - `--host string` — bind address, default `127.0.0.1`. Warn if user overrides to non-loopback.
  - `--no-open` — skip browser open.
- Loads `App` via `PersistentPreRunE`. Uses `app.GetAll()` (config + client + projectID).
- Browser open: reuse `common.OpenBrowserAsync(url)` (already used by `login` / `payment add`). Honors `--no-open`.
- Shutdown on `cmd.Context().Done()` via `srv.Shutdown(ctx)`.
- Analytics: add `serve` to `wrapCommands` in `root.go`. No sensitive args.

Output (stderr to keep stdout free for future scripting use):
```
Listening on http://127.0.0.1:54321
Opened browser. Press Ctrl+C to stop.
```

---

## The web app (`web/`)

### Stack

- **React 19**, **Vite 7**, **TypeScript 5**.
- **Tailwind v3.3** — required because `@timescale/popsql-query-widget` is pinned to v3 (its config re-exports `@popsql/lollipop/tailwind.config`).
- **TanStack Query v5** for `/api/databases` and `/api/bootstrap` polling.
- `@timescale/popsql-query-widget` (private npm) + its peer deps: `react@19`, `react-dom@19`, `framer-motion@^12`.
- Single CSS import: `@timescale/popsql-query-widget/index.css` (precompiled by the widget's `bin/package.sh`).

### Layout

```
┌────────────────────────────────────────────────────────────┐
│  ghost                                       [ db-name ▾ ] │
├────────────────────────────────────────────────────────────┤
│                                                            │
│  <QueryWidget id="…" query={query} … />                    │
│                                                            │
└────────────────────────────────────────────────────────────┘
```

- Header: ghost logo + database `<select>` populated from `GET /api/databases`. Selection mirrored to `?db=<id>`.
- Body: full-height `<QueryWidget>`. Empty state when no DB selected.
- Auto-select if exactly one ready database. Non-ready statuses shown but disabled.

### Wiring the widget

```tsx
<TimescaleResultsCacheContextProvider baseUrl={window.location.origin}>
  <QueryWidgetProvider theme="light">
    <ContextMenuProvider>
      <QueryWidget
        id={`ghost-${databaseId}`}
        query={query}
        onQueryChange={setQuery}
        // sessionKey intentionally omitted → one-shot ephemeral mode
        getExecuteQueryData={({ runId, query }) => ({
          engine: ExecuteQueryEngine.timescaleQuery,
          params: { projectId, serviceId: databaseId, query, runId },
        })}
      />
      <ContextMenuContext.Consumer>{({ render }) => render()}</ContextMenuContext.Consumer>
    </ContextMenuProvider>
  </QueryWidgetProvider>
</TimescaleResultsCacheContextProvider>
```

`projectId` is fetched from `GET /api/bootstrap` at app boot; `databaseId` comes from the picker (and URL state). The widget hits `${origin}/api/{executeQuery,arrowResults,cancelRun}` — all served by the Go server in-process.

### Vite config

- `server.proxy` forwards `/api/*` to a configurable local port for dev mode (`ghost serve --port 5174` while Vite runs on `:5173`).
- Copy Monaco workers + DuckDB workers/wasm into the build output adjacent to `index.js` (port the relevant `Vite.config.ts` plugin from `web-cloud/vite.config.ts:30-203`).
- Exclude `@timescale/popsql-query-widget` from `optimizeDeps` so workers resolve sibling chunks correctly.
- Output to `web/dist/`.

---

## The local HTTP server (`internal/serve/`)

### Routes

```
GET   /healthz             → {"ok":true}
GET   /api/bootstrap       → { projectId, version }
GET   /api/databases       → ghost-api passthrough (list user's databases)
POST  /api/executeQuery    → NDJSON producer; runs query in-process
POST  /api/arrowResults    → Arrow IPC producer; consumes runId state
POST  /api/cancelRun       → best-effort pg_cancel_backend
*     everything else      → embedded SPA (SPA fallback to index.html)
```

`/api/databases` is the only ghost-api hop. Implemented as a thin call: read the API client from `App`, hit `ListDatabases`, return the JSON shape the SPA expects (probably the API's own DTO — defer trimming until UI work). Password lookup happens lazily inside `executeQuery` via `common.GetPassword`, not in `/api/databases`.

### Static asset serving (`assets.go`)

- `//go:embed all:web` rooted at `internal/serve/web/`. Built by `scripts/build-web.sh`.
- Resolver:
  - `/` → `/index.html`.
  - Exact FS match → serve with detected `Content-Type`.
  - Path with no extension in last segment → `index.html` (SPA fallback).
  - Otherwise → 404.
- Cache headers:
  - `Cache-Control: public, max-age=31536000, immutable` for `/assets/*`.
  - `Cache-Control: no-cache` for everything else.
- Empty FS handling: if `index.html` is absent, serve a placeholder HTML linking to `scripts/build-web.sh`. Lets `go build` / `go test` work without a JS build.

### `Run` store (`runs.go`)

```go
type Run struct {
  ID          string
  ProjectID   string
  ServiceID   string
  Schema      *arrow.Schema
  Records     <-chan arrow.Record   // closed when query finishes
  Done        chan struct{}         // closed on terminal status
  Err         error                 // set before Done is closed
  RowCount    int64
  Cancel      context.CancelFunc    // cancels the pg query context
  StartedAt   time.Time
  FinishedAt  time.Time
}
```

- `runs.Store` is a `sync.Map[string]*Run` keyed by `runId`.
- Runs older than N minutes (e.g. 10) are reaped by a background goroutine after `Done` closes.
- `executeQuery` registers a `Run`, kicks off the PG query in a goroutine, and returns the NDJSON stream from the same handler. The goroutine pushes record batches into `Run.Records`.
- `arrowResults` looks up the `Run`, waits for `Schema` to be set, writes the IPC schema, then range-reads from `Run.Records` and writes each batch to the response with `Flush()`.
- `cancelRun` calls `Run.Cancel()`.

### `executeQuery` flow (`execute.go`)

1. Decode request body into `wire.ExecuteQueryRequest`.
2. Validate `projectId` matches the CLI's active project; reject mismatches with 403 (the binary is single-user but defense in depth).
3. Resolve `serviceId` → `(host, port, database, sslmode, ...)` via `app.GetClient().GetDatabaseWithResponse(ctx, serviceID)` cache.
4. Resolve password via `common.GetPassword(database)`. If `common.ErrPasswordNotFound`, terminate the stream with `{ "success": false, "error": { "message": "no password found — run 'ghost password <db>' or add to ~/.pgpass" } }`.
5. `common.CheckReady(database)` — if not ready, return the same shape with the actionable message.
6. Open a `pgx.Conn` (per-query, closed on goroutine exit).
7. `conn.Query(ctx, sql)` — capture `FieldDescriptions` once available.
8. Build `arrow.Schema` from the `FieldDescriptions` (`pgtypes.go`).
9. Write `{ "columns": [...] }` NDJSON line to the executeQuery response writer + `http.Flusher.Flush()`. Register `Run.Schema` so `arrowResults` can proceed.
10. Stream rows → `array.RecordBuilder` (chunked at e.g. 1024 rows per batch) → push each `arrow.Record` to `Run.Records`.
11. On `rows.Err()`: close `Run.Records`, write `{ "success": false, "error": ... }`, close.
12. On clean finish: close `Run.Records`, write `{ "success": true, "rowCount": N, ... }`, close.
13. On `ctx.Done()` (client aborted or `/api/cancelRun` invoked): `pg_cancel_backend` via a side channel connection, mark `Run` with `error.cancel: true`.

### `arrowResults` flow (`arrow.go`)

1. Decode request body, look up `Run`.
2. `Content-Type: application/vnd.apache.arrow.stream`.
3. `ipc.NewWriter(w, ipc.WithSchema(run.Schema))`.
4. Range over `Run.Records` — `writer.Write(record)` + `Flush()` after each.
5. `writer.Close()` when the channel closes.
6. If `Run.Err` was set before any batch arrived, return HTTP 500 with `{ error: { message: ... } }`.

### `cancelRun` flow

Look up `Run`, call `Run.Cancel()`. Return `204 No Content`. `executeQuery`'s goroutine sees `ctx.Done()` and terminates with `error.cancel: true`.

### Port selection / browser open / shutdown

Same as memory-engine's pattern, ported to Go:
- `net.Listen("tcp", host+":0")` for kernel-assigned port (no probe/release race).
- Explicit `--port` is strict — bind failure surfaces directly.
- `common.OpenBrowserAsync(url)` (existing helper).
- Graceful shutdown on `cmd.Context().Done()` via `srv.Shutdown(ctx)` with a 5s deadline.

---

## Asset embedding

Same as the previous plan version — `embed.FS` rooted at `internal/serve/web/`, populated by `scripts/build-web.sh` (`cd web && npm ci && npm run build && cp -r dist/* ../internal/serve/web/`). `web/dist/` is git-ignored; `internal/serve/web/.gitkeep` makes `//go:embed` happy.

Why not the memory-engine base64-into-TS approach: Go's `embed.FS` is the natural fit, keeps assets raw (no 4/3× inflation), and eliminates a build step.

---

## ghost-api changes

**None.** This was the main motivation for the lean approach. The only ghost-api call from the local server is the read-only `ListDatabases` (and `GetDatabase` for password fetch), which already exist.

---

## Build / CI

- `scripts/build-web.sh` runs `npm ci` + `npm run build` and syncs `web/dist/` → `internal/serve/web/`.
- `check` script runs `scripts/build-web.sh` before `go install`.
- CI: in `.github/workflows/*.yml`, install Node 22, run `scripts/build-web.sh` before any Go build step (test, lint, release).
- `Dockerfile`: multi-stage — `node:22` stage builds `web/dist/` and copies into `internal/serve/web/`, then `golang:1.x` stage builds the binary.
- Release pipeline (GoReleaser) needs Node available — easiest is to invoke `scripts/build-web.sh` as a `before` hook in `.goreleaser.yaml`.
- Binary size impact: rough order-of-magnitude based on memory-engine (~7MB compressed assets, mostly Monaco + DuckDB-WASM). Acceptable.

---

## Testing

### Go side (`CLAUDE.md` patterns)

- `internal/cmd/serve_test.go` — `runCommand` harness with mock API client; assert `--no-open`, auto-port, explicit port collision error, graceful shutdown on context cancel.
- `internal/serve/server_test.go` — full-stack server, `/healthz`, `/api/bootstrap`, asset serving, SPA fallback, 404 on missing static asset.
- `internal/serve/assets_test.go` — covers cache headers (`/assets/*` immutable, others no-cache).
- `internal/serve/execute_test.go` — fires a real PG query against a `pgmock` or a containerized Postgres (CI), asserts NDJSON output: `columns` line, then `success` line; verifies the parallel `/api/arrowResults` returns a valid IPC stream that round-trips through Apache Arrow Go's reader and matches the expected rows.
- `internal/serve/arrow_test.go` — unit tests for PG OID → Arrow type mapping, value coercion edge cases (null, numeric, array, json, timestamptz).
- `internal/serve/cancel_test.go` — verifies `/api/cancelRun` interrupts a long-running query and surfaces `error.cancel: true` on executeQuery.

### Web side

- One Vitest unit per logic module: `lib/url-state.ts` round-trip, `api/databases.ts` selector behavior.
- No widget integration tests; the widget is treated as a black box.

### End-to-end (optional, follow-up)

- Playwright smoke: spin up `ghost serve --no-open --port 5599` against a test ghost database, drive the UI to run `select 1`, assert the result table shows `1`.

---

## Out of scope (for MVP)

- Session mode (`createSession`, `sessionEvents`, `executeSessionQuery`, `closeSession`).
- Saving / loading queries; per-DB query history.
- Schema browser sidebar.
- Multi-tab queries.
- Numeric precision-preserving Arrow `Decimal128` (we use `Utf8` for `numeric`).
- Query timeout enforcement beyond the existing `HTTPClient.Timeout`.
- Auth on the localhost listener (loopback bind is the boundary).
- Embedding into the docker image as a primary use case — Dockerfile still works but `ghost serve` from inside a container isn't a target.

---

## Decisions (locked in)

| # | Topic | Decision |
|---|-------|----------|
| 1 | Apache Arrow Go dep | Yes — keep the widget unmodified, ship the Arrow dep. |
| 2 | DB driver | `pgx/v5` — copy popsql-query's approach verbatim. |
| 3 | Browser open | Open by default; `--no-open` flag opts out. |
| 4 | Web app dir | `web/` at repo root. |
| 5 | JS package manager | **Bun**, via a self-bootstrapping `./bun` wrapper script (copied from `../ox/bun`). No new system dependencies. |
| 6 | Private npm registry | `@timescale/*` is on GitHub Packages. Translate `web-cloud/.yarnrc.yml` into `bunfig.toml`'s `[install.scopes]` block; CI gets the token from `GITHUB_TOKEN` or a fine-grained PAT, mirroring web-cloud's deploy-to-dev workflow. |
| 7 | Tailwind | v3 — widget is pinned to v3. |
| 8 | Not-logged-in handling | Fail fast with the standard `ghost login` hint. |
| 9 | Session mode | **In MVP.** `sessionKey` is derived from the selected database ID — switching DBs invalidates the session; page reload mints a new one; `SessionError` triggers the widget's built-in re-create flow. |
| 10 | Project scope | No project switcher; SPA inherits the CLI's active space. |
| 11 | Localhost auth | No URL token; bind to `127.0.0.1` only. |
| 12 | Type handling | Copy popsql-query's PG OID → Arrow type + value coercion logic verbatim. Don't redesign. |
| 13 | E2E testing | Playwright + Chrome DevTools MCP. Throwaway test DB via `ghost create`. |

---

## Progress

- [x] Research: widget exports + wire protocol
- [x] Research: web-cloud integration patterns
- [x] Research: memory-engine `me serve` implementation
- [x] Plan v1 — proxy-through-ghost-api architecture
- [x] Plan v2 — in-process query execution architecture (this document)
- [x] Discovery: popsql-query type mapping + Arrow writer + cancellation
- [x] Discovery: `../ox/bun` self-bootstrap wrapper
- [x] Discovery: `web-cloud/.yarnrc.yml` + deploy-to-dev GH workflow → bun equivalent
- [x] Discovery: Arrow Go module path + pgx version pinning
- [x] Step 1 — `ghost serve` skeleton + static SPA + `/api/databases` (validated end-to-end)
- [x] Step 2 — query execution path (executeQuery, arrowResults, sessions, cancel) — widget integration + 6-type smoke test
- [x] Step 3 — polish, docs, ungate from `GHOST_EXPERIMENTAL`
- [x] E2E test pass (manual via Chrome DevTools MCP against a live Timescale Postgres)

### Resolved: `@timescale/popsql-query-widget` private-registry auth

Initial `bun install` got 403 from `https://npm.pkg.github.com/@timescale%2fpopsql-query-widget` because the default `gh auth` token only had `repo` / `read:org`. Resolution: `gh auth refresh -h github.com -s read:packages` adds the missing scope; `scripts/build-web.sh` then resolves the widget cleanly. CI uses `secrets.GITHUB_TOKEN` which already has `read:packages` for owner-controlled packages.

---

## Discovery findings

### popsql-query — what to port verbatim

Layout (`/Users/murrayju/dev/timescale/popsql-query/internal/`):

| Path | Lines | Action |
|------|-------|--------|
| `types/{binary,date,guid,json,numeric,types}.go` | 290 | **Port verbatim** — custom scan types preserving precision/special values (NaN, ±Inf, Postgres `bytea` hex format, plain Date/DateTime without TZ, Numeric, JSON, GUID). |
| `driver/adapter.go` | 219 | **Port partial** — keep `baseAdapter` / `baseDriver` / `Rows` / `QueryArgs` / `Columns` machinery and `cancelContext` helper; drop the multi-driver registry (we only need PG). |
| `driver/postgres.go` | ~250 | **Port verbatim** — `pgx/v5/stdlib.OpenDB` integration, `postgresQueryTracer` for `CommandTag.RowsAffected()`, `scanType` overrides for JSON/JSONB/NUMERIC/BYTEA/DATE/TIMESTAMP/TIMESTAMPTZ, `NormalizeError` for `pgconn.PgError` (fatal flag, code/detail/hint/position, multi-statement guard). |
| `writer/arrow.go` | ~340 | **Port verbatim** — `RecordBuilder` wrapper around `array.RecordBuilder`; appends `__popsql_row_num__` Int64 column to every schema (preserves original row order); attaches `__popsql_columns__` JSON metadata to the schema (original column descriptors round-trip to the frontend); `arrowType()` + `builderFn` mapping by `ScanType`. |
| `writer/record.go` | 307 | **Port partial** — the row-iteration loop + record-flush logic. |
| `writer/result.go` | 393 | **Port partial** — only the success/error result envelopes; drop Parquet/CSV/TSV writers. |
| `handler.go`, `run.go`, `session.go`, `store.go` | 1311 | **Do NOT port** — we write fresh handlers that match the widget's wire protocol (different URL shape, different request bodies, single-user, no Redis clustering, no S3 upload). |

Key patterns inherited:

- **Cancellation**: `pgConn.CancelRequest(ctx)` is the canonical pg cancel — captured via `b.conn.Raw(func(driverConn any) error { pgConn = driverConn.(*stdlib.Conn).Conn().PgConn(); return nil })` after `stdlib.OpenDB`. Wired through `cancelContext()` in `adapter.go`.
- **Row order**: Arrow batches don't guarantee row order across batches, so popsql-query appends a synthetic `__popsql_row_num__` Int64 column. We adopt the same convention so the widget's table component renders in query order.
- **Schema metadata**: original column descriptors (with PG type name) are JSON-encoded and stashed in the Arrow schema's metadata under `__popsql_columns__`. The frontend uses this for type-aware rendering.
- **Application name**: connections set `application_name` to a constant for server-side identification (port the constant; rename it to `"ghost-cli"` or similar).
- **SSL handling**: pgx's default `sslmode=prefer` is used unless an explicit TLS config is set, in which case `pgxCfg.Fallbacks = nil` to require encryption. Ghost cloud DBs require TLS, so we'll set explicit TLS config.

### `../ox/bun` self-bootstrap wrapper

431-byte bash script. Pinned to `bun-v1.3.14`. Downloads to `./download/bun/<version>/bin/bun` on first invocation via the official `https://bun.sh/install` script with `BUN_INSTALL` env override. `exec`s the downloaded binary with all forwarded args.

**Action**: copy verbatim to `./bun` in this repo; add `download/` to `.gitignore`.

### GitHub Packages registry auth

Web-cloud (`deploy-to-dev.yml`) uses the auto-provisioned `secrets.GITHUB_TOKEN`:

```yaml
- name: Setup .yarnrc.yml
  run: yarn config set npmScopes.timescale.npmAuthToken $NPM_AUTH_TOKEN
  env:
    NPM_AUTH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

Bun reads `.npmrc` natively (and `bunfig.toml` for bun-specific options). Plan:

- `web/.npmrc` (gitignored, committed-template version is `web/.npmrc.example`):
  ```
  @timescale:registry=https://npm.pkg.github.com/
  //npm.pkg.github.com/:_authToken=${NPM_AUTH_TOKEN}
  ```
- CI sets `NPM_AUTH_TOKEN: ${{ secrets.GITHUB_TOKEN }}` on the bun-install step.
- Local devs export `NPM_AUTH_TOKEN` to a GitHub PAT with `read:packages` scope (documented in README).

### Module versions (locked to match popsql-query)

- `github.com/apache/arrow-go/v18 v18.5.2`
- `github.com/jackc/pgx/v5 v5.8.0` (`stdlib`, `pgconn`)
- Indirect deps already pulled by popsql-query (`pgpassfile`, `pgservicefile`, `puddle/v2`).

---

## Sequencing (as delivered)

Three commits on `murrayju/serve`, in this order:

1. `ae9e4f9` — Add ghost serve skeleton. Cobra command behind `GHOST_EXPERIMENTAL`, embed.FS asset handler with SPA fallback + cache headers, `/api/bootstrap` + `/api/databases`, Vite/React workspace with picker + empty body, bun self-bootstrap wrapper, scripts/build-web.sh.
2. `2df095e` — Fix build-web.sh and pin widget version (`--cwd` doesn't apply to bun's `run`; widget pinned to `0.0.0-dev.156`).
3. `43921bd` — Wire the popsql query widget into ghost serve. Ported dbtypes + dbdriver + arrow encoder from popsql-query, implemented executeQuery + arrowResults + sessions + cancel handlers, wired `<QueryWidget>` into the SPA with Vite worker/wasm asset emission + node polyfills, React 18 pin.
4. `d741355` — Polish ghost serve and ungate from GHOST_EXPERIMENTAL. CLAUDE.md + README updates, generated `docs/cli/ghost_serve.md`, tests for wire / assets / store / cmd, favicon + form-name a11y tidy-ups.
5. `045261e` — Build the web bundle in CI before Go builds. Both `.github/workflows/{test,release}.yaml` now run `./scripts/build-web.sh` before any Go command, sourcing the widget from GitHub Packages via the auto-provisioned `secrets.GITHUB_TOKEN`.

## Follow-ups (out of scope for this branch)

- **Decimal128 for NUMERIC — declined.** Postgres `NUMERIC` accepts NaN, ±Infinity, and unbounded precision; Arrow `Decimal128` is 38 digits and can't carry any of those, so an implementation would need per-row fallback branching and an upstream change in popsql-query (which uses the same string encoding for the same reasons). The widget's table already gets `isNumeric: true` for right-align + sort, so the on-wire string is a non-issue for UX. Not worth doing.
- Query timeout enforcement (the widget sends `timeout` but we ignore it).
- Server-side `slog` logging + log levels (currently silent by design).
