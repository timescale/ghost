import { useQuery } from '@tanstack/react-query';

import type { DatabaseSchema } from '../schema';
import { useServeStore } from '../store';

// schemaQueryKey is the shared TanStack Query key for a database's schema. The
// schema pane and the autocomplete index both fetch through useSchemaQuery, so
// they hit one cached request; the `showInternalObjects` toggle is part of the
// key so both stay consistent with what the tree displays.
export function schemaQueryKey(
  databaseId: string,
  showInternalObjects: boolean,
) {
  return ['schema', databaseId, showInternalObjects] as const;
}

// useSchemaQuery loads a database's full schema (with object definitions and
// comments) from GET /api/schema. Shared by the schema pane and the
// autocomplete index so the fetch is only made once per database + toggle.
export function useSchemaQuery(databaseId: string) {
  const showInternalObjects = useServeStore((s) => s.showInternalObjects);

  return useQuery({
    queryKey: schemaQueryKey(databaseId, showInternalObjects),
    queryFn: async () => {
      // Request object definitions and comments so the tree's View/Copy
      // actions have data and autocomplete can surface them. The server omits
      // both by default to keep the payload light, so these opt-ins are
      // required.
      const params = new URLSearchParams({
        databaseId,
        definitions: 'true',
        comments: 'true',
      });
      if (showInternalObjects) params.set('internal', 'true');
      const res = await fetch(`/api/schema?${params}`);
      if (!res.ok) {
        throw new Error(`/api/schema: ${res.status} ${await res.text()}`);
      }
      return res.json() as Promise<DatabaseSchema>;
    },
    staleTime: 60_000,
  });
}
