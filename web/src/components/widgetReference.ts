// Constant referenceId passed to every serve-UI QueryWidget. The widget only
// auto-evicts the previous run ("only keep the current run in memory") when no
// referenceId is set; passing a constant id disables that, so every run's
// results stay cached and ghost owns eviction (enforcing
// ui_query_history_limit). It also stops the read-only query-history detail
// widget from evicting a run just by browsing to a different one. For the
// timescale engine this id is never forwarded to the cache worker, so it has no
// aliasing side effects — it's purely the flag that keeps prior runs.
export const WIDGET_REFERENCE_ID = 'ghost-serve';
