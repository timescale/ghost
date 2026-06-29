// Shapes exchanged with the Go MCP tools over the agent bridge. These mirror
// the structs in internal/mcp/browser_types.go and the SSE event/response
// shapes in internal/serve/agent.go — keep them in sync.

import type { ResultView } from '../components/chart/types';
import type { ChartConfigDiagnostic } from './diagnostics';

export type { ChartConfigDiagnostic } from './diagnostics';

// Events pushed from the server over the SSE stream.
export type AgentServerEvent =
  | { type: 'status'; clientId: string; active: boolean }
  | { type: 'command'; command: AgentCommand }
  // Tells the client to abort the in-flight command with this requestId (the
  // MCP caller canceled, the request timed out, or another tab took over).
  | { type: 'cancel'; requestId: string };

// A unit of work dispatched by an MCP tool for the browser to execute.
export interface AgentCommand {
  id: string;
  type: 'visualize' | 'chart' | 'uiState';
  payload: unknown;
}

// Command payloads (one per command type).
export interface VisualizeCommand {
  databaseRef: string;
  sql: string;
  chartConfig?: string;
  limit: number;
}

export interface ChartCommand {
  chartConfig: string;
}

export interface UIStateCommand {
  limit: number;
}

// A column of a result set returned to the server.
export interface AgentColumn {
  name: string;
  type?: string;
}

// Response shapes posted back to the server (the `data` field of a "result"
// message).
export interface VisualizeResult {
  runId: string;
  columns: AgentColumn[];
  rows: unknown[][];
  rowCount: number;
  // Postgres command-tag count for the run (rows touched by a DML command, or
  // rows returned by a SELECT). Mirrors Go's common.ExecuteQuery RowsAffected.
  rowsAffected: number;
  // Data URL of the rendered chart. Present when the chart rendered
  // successfully; mutually exclusive with chartError.
  image?: string;
  // Message explaining why the chart couldn't be rendered (e.g. an invalid
  // chart config, or data the config can't plot). The run data is still
  // returned alongside it.
  chartError?: string;
  // Type/syntax issues reported by the editor's language service for the chart
  // config (the same red squiggles a human sees). May be present even when the
  // chart renders, since many type errors don't throw at runtime.
  chartDiagnostics?: ChartConfigDiagnostic[];
}

export interface ChartResult {
  // Data URL of the rendered chart. Present when the chart rendered
  // successfully; mutually exclusive with chartError.
  image?: string;
  // Message explaining why the chart couldn't be rendered (e.g. an invalid
  // chart config, or data the config can't plot).
  chartError?: string;
  chartDiagnostics?: ChartConfigDiagnostic[];
}

export interface LastRunState {
  runId?: string;
  status?: string;
  rowCount: number;
  // Postgres command-tag count for the run (see VisualizeResult).
  rowsAffected: number;
  columns?: AgentColumn[];
  rows?: unknown[][];
  error?: string;
}

export interface UIStateResult {
  selectedDatabaseId?: string;
  editorSql?: string;
  chartConfig?: string;
  resultView?: ResultView;
  lastRun?: LastRunState;
  // Data URL of the rendered chart of the last run. Mutually exclusive with
  // chartError.
  image?: string;
  // Message explaining why the chart couldn't be rendered, if applicable.
  chartError?: string;
  // Type/syntax issues reported by the editor's language service for the chart
  // config (the same red squiggles a human sees).
  chartDiagnostics?: ChartConfigDiagnostic[];
}
