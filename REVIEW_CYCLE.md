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

---

## Round 4 — model: `openrouter/z-ai/glm-5.1`

Single finding; everything else verified clean.

1. **`stringifyCell` renders booleans as JSON true/false, not Postgres t/f** —
   _fixed_. Booleans fell through to `fmt.Sprintf("%v")` → `"true"`/`"false"`,
   but `common.ExecuteQuery` scans booleans into `*string` as Postgres text
   (`t`/`f`). So visualized boolean columns diverged from the plain `ghost_sql`
   path, breaking the documented invariant. Added an explicit `bool` case
   returning `t`/`f`; updated the existing test expectation.

The reviewer also independently verified that all prior-round fixes (cancel
races, JSON number mangling, unbounded pre-empted set, stale GC pointer,
dangling timer, UI-mutation ordering, readiness check, rows_affected/command_tag)
are correctly addressed, and found no other issues.

**Round 4 result:** 1 fixed (low-severity consistency bug), rest clean.

---

## Round 5 — model: `openai-codex/gpt-5.5`

Both findings valid and fixed.

1. **agent.go cancel can be dropped, letting an abandoned command run** —
   _fixed_. A takeover/disconnect resolved `p.result` and best-effort
   `sendCancel`'d the superseded client. But the dispatch `select` races
   command-send against `p.result`: if send wins after the resolve dropped the
   cancel (client buffer momentarily full), the command is delivered but the
   cancel never is. Made `Request` the single, reliable owner of cancel
   delivery: resolving paths no longer send their own cancel, and `Request`
   sends one on every error-exit from the heartbeat loop (command already
   delivered) via a new `sendCancelReliably` that blocks until enqueued / client
   disconnects / short grace elapses. Regression test fills the superseded
   client's buffer and asserts the cancel is still delivered. Verified clean
   under `-race -count=20`.
2. **screenshot.ts `renderToDataURL` leaks listener/timer on setOption throw** —
   _fixed_. The promise only had a `resolve`; if `chart.setOption` threw on a
   malformed (but object-shaped) option, the promise rejected but left the
   `finished` listener and 10s timeout registered — and `renderChartImage` then
   disposed the chart, so the timer would later call `getDataURL` on a disposed
   instance. Added a `reject` path, centralized cleanup (`off` + `clearTimeout`)
   behind a single-settle guard, and wrapped `setOption`/`getDataURL` so any
   throw rejects cleanly. Regression test added.

**Round 5 result:** 2 fixed (1 concurrency correctness bug, 1 resource leak).

---

## Round 6 — model: `openai-codex/gpt-5.5`

Both findings valid and fixed (both in `dispatch.ts`/its test).

1. **`ghost_chart` charts failed/uncached last runs and mutates UI first** —
   _fixed_. Round 1 reordered validation before mutation, but two gaps
   remained: (a) `handleChart` accepted any last run for the current database,
   including a *failed* one (recorded with `status: 'failed'`, no cached
   results); (b) it still mutated the UI (`setChartConfig`/`setResultView`)
   before `getRunData` proved results were readable, so an evicted-cache run
   would clobber config/view then error. Added a `status === 'success'` check
   and moved the UI mutation to after the `getRunData` read. Regression tests
   for both the failed-run and cache-miss paths.
2. **Two chart tests' `.rejects.toThrow()` not awaited** — _fixed_. Two tests
   called `expect(dispatch(...)).rejects.toThrow(...)` without `await`, creating
   a floating promise that could let the test pass without verifying the
   rejection. Added `await` to both (matching the sibling test).

**Round 6 result:** 2 fixed (1 UI-correctness gap left by round 1's partial fix,
1 test-reliability bug).

---

## Round 7 — model: `openai-codex/gpt-5.5`

Both findings valid and fixed.

1. **SSE drop doesn't abort the in-flight command** — _fixed_. On `onerror`,
   the browser only updated the banner; the server had already ended the
   request (`ErrClientDisconnected`), so no later cancel could reach the
   browser, yet a long `visualize` query kept running and could still be
   mutating the UI when EventSource reconnected with a fresh client and a new
   command. Factored an `abortInFlightCommand` helper, called from `onerror` and
   the effect cleanup.
2. **Chart recorder swallows the user's first authored config** — _fixed_.
   `useChartConfigRecorder` seeded its baseline lazily on the *first successful
   render*. If the user edited before any render (no data yet) or within the
   1.5s record debounce, that first edited config became the baseline and was
   never recorded. Seed the baseline synchronously from the mount-time config
   (passed in from `ChartArea`) so the first edit is always recognized as a
   change. (No unit test added: the project has no React-hook test infra, and
   pulling in `@testing-library/react` for this one hook would be
   disproportionate — the fix is a small, self-evident reordering and the
   store-level recording/dedup is already covered by `store.test.ts`.)

**Round 7 result:** 2 fixed (1 lifecycle/UI-race bug, 1 history-recording bug).

---

## Round 8 — model: `openai-codex/gpt-5.5`

Single finding, valid and fixed.

1. **`ghost_chart` ignores the abort signal, mutating UI after abandonment** —
   _fixed_. `handleChart` received no signal: it validated the run, awaited
   `getRunData`, then unconditionally applied the chart config + view. If the
   command was canceled/superseded or the SSE stream dropped during that read
   (all of which abort the signal and end the server request), or the user
   switched databases (remounting the panel), the abandoned command still
   clobbered the UI. Threaded the signal into `handleChart`; after the read it
   bails if aborted or if the executor/last-run no longer match, before
   mutating. Regression test with a deferred `getRunData` + mid-read abort.

**Round 8 result:** 1 fixed (completes the cancel-safety story for the chart
path, mirroring the visualize path's signal handling).

---

## Round 9 — model: `openai-codex/gpt-5.5`

**No issues found.** gpt reviewed the full diff and returned exactly "No issues
found" — its stream of substantive findings has dropped off. Loop converged.

---

## Convergence

Findings by round: 5 (R1) → 5 (R2) → 1 (R3) → 1 (R4) → 2 (R5) → 2 (R6) → 2 (R7)
→ 1 (R8) → 0 (R9). Rounds 3–4 used other models and trended toward noise;
running gpt-5.5 from round 5 on kept surfacing genuine (if increasingly narrow)
issues that clustered tightly around the `dispatch.ts` cancel-handling area
before drying up entirely in round 9. Per the request ("continue the loop with
just gpt … until its findings drop off"), the cycle is complete.

**Final totals (R1–R9):** 20 fixed, 1 deferred (negligible + risky), 3 invalid.
`./check` (Go) and `bun typecheck`/`lint`/`test` (web) all pass.

---

## Round 10 — model: `anthropic/claude-opus-4-8`

Re-opened the loop with an opus reviewer after two post-convergence commits
landed (`7ff1b73` screenshot long-edge cap, `78eed9a` chartConfigHistory
persistence). Opus found **no correctness bugs** and explicitly verified the
persistence + cancel subsystems clean; it surfaced two low-severity items, both
addressed.

1. **`awaitExecutor` not passed the abort signal** — _fixed_ (`7b60a03`).
   `handleVisualize` awaited `awaitExecutor(databaseId, EXECUTOR_WAIT_MS)`
   without the signal. When a visualize command switches the selected database,
   `QueryPanel` remounts and the wait blocks until the new panel registers its
   executor (or 15s). If the request was canceled/timed-out/superseded or the
   SSE stream dropped during that sub-second remount window, the abort fired but
   `awaitExecutor` kept waiting — the command only bailed once the executor
   finally mounted (then `runQuery` sees the aborted signal) or after the full
   timeout, during which the browser kept sending heartbeats for an abandoned
   request. Threaded an optional `AbortSignal` into `awaitExecutor` (bail if
   already aborted; reject promptly and drop the waiter if it aborts while
   waiting); `handleVisualize` now passes its signal, bringing the pre-run wait
   to parity with `handleChart`'s post-read abort re-check. Added two executor
   tests (already-aborted, aborts-while-waiting).

2. **Stale `preemptedCommandId` race comment in `useAgentBridge.ts`** — _comment
   fixed; slot kept_ (`1cbecb8`). The comment described the pre-round-5 design
   where `Activate`/`removeClient` issued their own best-effort cancel
   concurrently with the command dispatch, so a cancel could race ahead of its
   command. The round-5 refactor made `Bridge.Request` the single owner of
   cancel delivery: `sendCancelReliably` is invoked only from `Request`, always
   *after* the command was enqueued to the same client's FIFO event channel — so
   a cancel can't normally overtake its command, making the `preemptedCommandId`
   path effectively unreachable. Kept the slot as harmless defense-in-depth
   against out-of-order delivery (e.g. an EventSource/proxy reordering quirk;
   the reviewer itself flagged low confidence about EventSource internals, and
   removing the safety net to save ~3 lines would re-open the class of bug
   rounds 5/7/8 fixed), but rewrote the comment to describe current server
   behavior and the slot's now-residual role.

**Round 10 result:** 2 fixed (1 real edge-case fix, 1 stale-comment
correction). No correctness defects found — the branch remains in very good
shape; the findings are consistent with the narrowing trajectory.

**Updated totals (R1–R10):** 22 fixed, 1 deferred, 3 invalid.
