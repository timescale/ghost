# Review Cycle Tracking

Branch: `murrayju/mcp-serve-visualization` vs `origin/main`
Range: `origin/main...HEAD` (26 commits, ~4940 insertions)

This file tracks findings from iterative code reviews. Each claim is recorded as
**fixed**, **deferred**, or **invalid**, with a short rationale.

---

## Round 1 — model: `openai-codex/gpt-5.5`

1. **agent.go `Request` dispatch race — pre-command cancel ignored** — _fixed_.
   Real but narrow race: `Activate` can send a `cancel` and resolve `p.result`
   while the dispatch `select` still has the command-send case ready, so the
   command can be enqueued *after* the cancel. The browser ignores a cancel for
   a not-yet-running command, then runs the abandoned command. Fixed on the
   browser side by remembering pre-empted cancel IDs and skipping the matching
   command.
2. **useAgentBridge cancel cancels the wrong/unrelated query** — _fixed_. A
   cancel for a `chart`/`uiState` command (which run no agent query) blindly
   called `getExecutor()?.cancelQuery()`, aborting an unrelated in-flight user
   query; a cancel during `awaitExecutor` could abort the previous database's
   query while the agent's own query hadn't started. Fixed by threading a
   per-command `AbortController`/`AbortSignal` through dispatch and scoping
   `cancelQuery` to the visualize handler's own running query.
3. **dispatch.ts `handleChart` mutates UI before validating** — _fixed_. It set
   `chartConfig`/`resultView` before checking for a mounted executor and a
   matching last run, so a failed `ghost_chart` still clobbered the user's chart
   config and switched the view. Fixed by validating first, then applying.
4. **ui_state.go drops rows_affected/command_tag for no-row runs** — _fixed_.
   A successful UPDATE/DELETE/DDL run (no columns/rows) produced no `ResultSet`,
   so `rows_affected`/`command_tag` vanished from structured output (the
   visualize path always emits one). Fixed by emitting the result set whenever
   the last run succeeded.
5. **browser_format.go `stringifyCell` mangles JSON objects** — _fixed_.
   JSON/JSONB cells arrive as decoded `map`/`[]any` and were rendered with
   `fmt.Sprintf("%v", ...)` (Go debug format, e.g. `map[a:b]`) instead of valid
   JSON, diverging from the server-side query path. Fixed by `json.Marshal`ing
   maps/slices, falling back to `Sprintf`.

**Round 1 result:** all 5 findings validated as real and fixed. Commits
`ba415b3`, `5e3f536`, `70f6adc`, `c289e98` (findings 1+2 squashed into one
commit since they're one cohesive change to the cancel subsystem).

---

## Round 2 — model: `anthropic/claude-opus-4-8`

All 5 findings are new (no overlap with round 1).

1. **stringifyCell mangles whole/large JSON numbers into exponent form** —
   _fixed_. Browser responses were decoded with plain `json.Unmarshal`, so JSON
   numbers became `float64` and the `fmt.Sprintf("%v")` fallback rendered them
   in exponent notation (e.g. `10000000` → `1e+07`), diverging from the
   server-side text output. Fixed by decoding with `UseNumber()` and adding a
   `json.Number` case (preserving the exact source literal). Regression tests
   added.
2. **Visualize path skips the friendly readiness check** — _fixed (premise
   partly corrected)_. The reviewer claimed the serve handler never calls
   `CheckReady`; it actually does (`connectionStringForService`). But the real
   issue holds: the visualize path opened a browser and waited for a client
   before that error surfaced, and reported it as a generic "visualization
   failed" rather than the friendly "resume it / check status" guidance the
   non-visualize path gives. Fixed by calling `common.CheckReady` up front (via
   a refactored `resolveDatabase` — no extra round-trip) and routing it through
   `handleDatabaseError`.
3. **`preemptedCommandIds` can grow unbounded** — _fixed_. When a takeover
   resolves the request via the dispatch `select` race instead of sending the
   command, the cancel is still enqueued but the matching command never
   arrives, leaking a Set entry per such takeover. Since the server keeps at
   most one command in flight, replaced the Set with a single overwriteable
   `string | null` slot.
4. **Misleading "query already running" on panel teardown** — _fixed_. If the
   panel unmounted between scheduling the deferred `setTimeout` and running it,
   `apiRef.current` was null and `runQuery` rejected with "a query is already
   running" instead of the real teardown cause. Added an explicit null-`apiRef`
   branch with a teardown message.
5. **Dangling 5s diagnostics timer** — _fixed_. `tryGetChartConfigDiagnostics`
   never cleared its `Promise.race` timeout when diagnostics resolved first,
   leaving a timer per call. Cleared it in a `finally` block.

**Round 2 result:** all 5 findings addressed (4 clear bugs/leaks, 1 with a
corrected premise but a real underlying UX issue). Commits since round 1:
`json-number`, `checkready`, `preempted-slot`, `diagnostics+teardown`.

---

## Round 3 — model: `openrouter/google/gemini-3.5-flash`

This round's signal-to-noise was much lower: only 1 of 5 findings held up, and
two were factually wrong (one misquoted the code, one contradicted a correct
round-2 fix).

1. **agent.go `removeClient` leaves a stale pointer in the backing array** —
   _fixed_. `append(clients[:idx], clients[idx+1:]...)` doesn't clear the final
   slot, so removing the last client pins it (and its channels) from GC across
   the bridge's connect/disconnect churn. Minor, but a trivial idiomatic fix:
   `copy` + nil the tail slot + reslice. Verified empirically that the last
   element lingers without it.
2. **agent.go `Request` timer Reset needs draining** — _invalid_. The advice
   ("stop+drain before Reset") applies to pre-Go-1.23 timer semantics. This
   module is `go 1.26.3`; since Go 1.23, `Timer.Reset`/`Stop` guarantee no stale
   value is ever delivered from `timer.C`. Verified empirically with a 1.26
   repro: a Reset after expiry delivers no stale value. No change.
3. **QueryPanel unmount cleanup: `apiRef.current` may be null** — _deferred_.
   Technically possible, but negligible: the cleanup's real job (rejecting
   pending runs so the MCP call doesn't hang) doesn't touch `apiRef`, and the
   widget's own `sessionKey`/unmount teardown cancels the server-side query and
   closes the session regardless. The proposed "capture `apiRef.current` in the
   effect body" fix risks holding a stale handle. Not worth the churn/risk.
4. **ChartView cleanup disposes a stale `chart` local (instance leak)** —
   _invalid_. The finding misquotes the code: the cleanup already does
   `chartRef.current?.dispose()` (not `chart.dispose()`), which is exactly the
   reviewer's own proposed fix. The error-recovery reinit writes the new
   instance back into `chartRef`, so unmount disposes the current one. No bug.
5. **`preemptedCommandId` single slot loses concurrent cancels; use a Set** —
   _invalid_. This contradicts the (correct) round-2 finding #3 fix, which
   replaced a Set *because* it grew unbounded. The server keeps at most one
   command in flight (`sem` of size 1) and each browser tab has its own bridge,
   so two commands are never simultaneously pre-empted on one tab. Reverting to
   a Set would reintroduce the leak. No change.

**Round 3 result:** 1 fixed (minor GC hygiene), 1 deferred (negligible + risky),
3 invalid (1 obsolete advice, 1 misread code, 1 contradicts a correct prior
fix). Signal is dropping off — the substantive issues appear to be exhausted.
