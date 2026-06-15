/// <reference lib="webworker" />
import { flattenSchema } from './flatten';
import type { WorkerRequest, WorkerResponse } from './protocol';
import { buildIndex, type SchemaIndex, searchIndex } from './search';

// Dedicated worker that owns the trigram search index and runs every fuzzy
// schema search off the UI thread. It holds the index built from the latest
// schema; the main thread serves cheap lookups (table kind, schema names,
// current database) itself, so only `search` round-trips here.
//
// The search engine lives behind this message boundary on purpose: it can be
// swapped for a duckdb-wasm (fts/trgm) implementation later without touching
// the client, the plugin, or any wiring.

let index: SchemaIndex | null = null;
let version = 0;

function post(message: WorkerResponse): void {
  self.postMessage(message);
}

self.onmessage = (event: MessageEvent<WorkerRequest>) => {
  const message = event.data;
  switch (message.type) {
    case 'init':
      index = buildIndex(flattenSchema(message.schema));
      version = message.version;
      return;
    case 'search':
      post({
        type: 'result',
        id: message.id,
        version,
        results: index
          ? searchIndex(index, message.query, message.requests)
          : [],
      });
      return;
  }
};
