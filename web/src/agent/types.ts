// Shapes exchanged with the Go MCP tools over the agent bridge. These mirror
// the structs in internal/mcp/browser_types.go and the SSE event/response
// shapes in internal/serve/agent.go — keep them in sync.

import type { ResultView } from '../components/chart/types';

// Events pushed from the server over the SSE stream.
export type AgentServerEvent =
  | { type: 'status'; clientId: string; active: boolean }
  | { type: 'command'; command: AgentCommand };

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
  view: Extract<ResultView, 'table' | 'chart'>;
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
  // Data URL of the rendered chart, present only for view='chart'.
  image?: string;
}

export interface ChartResult {
  image: string;
}

export interface LastRunState {
  runId?: string;
  status?: string;
  rowCount: number;
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
  // Data URL of the currently-visible chart, if any.
  image?: string;
}
