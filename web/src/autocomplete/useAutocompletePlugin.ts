import {
  type AutocompletePluginConfig,
  createAutocompletePlugin,
  type SchemaFetchResponse,
} from '@timescale/popsql-query-widget-cdn';
import { useEffect, useMemo, useRef } from 'react';
import { useSchemaQuery } from '../hooks/useSchemaQuery';
import type { DatabaseSchema } from '../schema';
import { SchemaSearchClient } from './client';
import { flattenSchema } from './flatten';
import { findTable } from './search';

type AutocompletePlugin = ReturnType<typeof createAutocompletePlugin>;

// Main-thread schema data derived from the cached schema response, used to serve
// the cheap autocomplete callbacks (everything except the fuzzy search) without
// a worker round-trip. `raw` is retained so the worker can be (re-)indexed.
interface SchemaData {
  databaseName: string;
  schemas: string[];
  defaultSchema: string | undefined;
  responses: SchemaFetchResponse[];
  raw: DatabaseSchema;
}

// useAutocompletePlugin builds the query widget's autocomplete plugin for a
// database. The heavy fuzzy search runs in a dedicated worker (via
// SchemaSearchClient); the schema is shared with the schema pane through
// useSchemaQuery, so no extra fetch is made. The returned plugin is
// referentially stable — its callbacks read the latest schema/index from refs,
// so a schema refresh re-indexes the worker without re-registering the plugin.
export function useAutocompletePlugin(databaseId: string): AutocompletePlugin {
  const { data: schema } = useSchemaQuery(databaseId);

  const clientRef = useRef<SchemaSearchClient | null>(null);
  const schemaDataRef = useRef<SchemaData | null>(null);

  // Derive main-thread schema data and (re-)index the worker whenever the
  // schema changes.
  useEffect(() => {
    if (!schema) {
      schemaDataRef.current = null;
      return;
    }
    const schemas = schema.schemas?.map((ns) => ns.name) ?? [];
    schemaDataRef.current = {
      databaseName: schema.name,
      schemas,
      defaultSchema: schemas.includes('public') ? 'public' : schemas[0],
      responses: flattenSchema(schema),
      raw: schema,
    };
    clientRef.current?.index(schema);
  }, [schema]);

  // Own the worker lifecycle in an effect (not render) so it is correctly
  // recreated after React StrictMode's mount/unmount/remount cycle. Index the
  // schema we already have, in case it loaded before this effect ran.
  useEffect(() => {
    const client = new SchemaSearchClient();
    clientRef.current = client;
    if (schemaDataRef.current) {
      client.index(schemaDataRef.current.raw);
    }
    return () => {
      client.dispose();
      clientRef.current = null;
    };
  }, []);

  return useMemo(
    () =>
      createAutocompletePlugin({
        fetchCurrentDatabase: async () =>
          schemaDataRef.current?.databaseName ?? '',
        fetchDatabases: async () =>
          schemaDataRef.current ? [schemaDataRef.current.databaseName] : [],
        fetchDefaultSchema: async () => schemaDataRef.current?.defaultSchema,
        fetchSchemas: async () => schemaDataRef.current?.schemas ?? [],
        fetchSuggestions: async (query, requests) =>
          clientRef.current?.search(query, requests) ?? [],
        fetchTable: async (name, database, schemaName) =>
          findTable(
            schemaDataRef.current?.responses ?? [],
            name,
            database,
            schemaName,
          ),
      } satisfies AutocompletePluginConfig),
    [],
  );
}
