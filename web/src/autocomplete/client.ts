import type {
  SchemaFetchResponse,
  SuggestionRequest,
} from '@timescale/popsql-query-widget-cdn';

import type { DatabaseSchema } from '../schema';
import type { WorkerResponse } from './protocol';

// Main-thread RPC wrapper around the schema-search worker. Owns the worker
// lifecycle and turns the postMessage protocol into promises. Searches are
// latest-wins: a search whose schema version is older than the current index
// resolves empty, so stale keystrokes never surface results from a replaced
// schema.
export class SchemaSearchClient {
  private worker: Worker;
  private version = 0;
  private nextId = 1;
  private pending = new Map<
    number,
    { resolve: (results: SchemaFetchResponse[]) => void; version: number }
  >();

  constructor() {
    this.worker = new Worker(
      new URL('./schemaSearch.worker.ts', import.meta.url),
      { type: 'module' },
    );
    this.worker.onmessage = (event: MessageEvent<WorkerResponse>) => {
      const message = event.data;
      if (message.type === 'result') {
        const entry = this.pending.get(message.id);
        if (!entry) return;
        this.pending.delete(message.id);
        // Drop results computed against a schema older than the latest index.
        entry.resolve(message.version === this.version ? message.results : []);
      }
    };
  }

  // index (re)builds the worker's search index from a new schema. Bumping the
  // version invalidates any in-flight searches against the previous schema.
  index(schema: DatabaseSchema): void {
    this.version += 1;
    this.worker.postMessage({ type: 'init', version: this.version, schema });
  }

  // search runs a fuzzy schema search in the worker. Resolves empty if no
  // schema has been indexed yet.
  search(
    query: string,
    requests: SuggestionRequest[],
  ): Promise<SchemaFetchResponse[]> {
    if (this.version === 0) return Promise.resolve([]);
    const id = this.nextId++;
    return new Promise((resolve) => {
      this.pending.set(id, { resolve, version: this.version });
      this.worker.postMessage({ type: 'search', id, query, requests });
    });
  }

  dispose(): void {
    for (const { resolve } of this.pending.values()) resolve([]);
    this.pending.clear();
    this.worker.terminate();
  }
}
