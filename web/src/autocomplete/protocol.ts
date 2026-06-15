import type {
  SchemaFetchResponse,
  SuggestionRequest,
} from '@timescale/popsql-query-widget-cdn';

import type { DatabaseSchema } from '../schema';

// Message protocol between the main thread and the schema-search worker.
// Only the heavy fuzzy search crosses this boundary; cheap exact lookups and
// name listings are served on the main thread from the cached schema.

export interface InitMessage {
  type: 'init';
  // Monotonic index version, echoed back on results so the client can drop
  // results computed against a stale schema after a re-index.
  version: number;
  schema: DatabaseSchema;
}

export interface SearchMessage {
  type: 'search';
  id: number;
  query: string;
  requests: SuggestionRequest[];
}

export type WorkerRequest = InitMessage | SearchMessage;

export interface ResultMessage {
  type: 'result';
  id: number;
  version: number;
  results: SchemaFetchResponse[];
}

export type WorkerResponse = ResultMessage;
