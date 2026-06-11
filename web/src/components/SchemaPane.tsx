import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useCallback, useEffect, useMemo, useState } from 'react';

import type { DatabaseSchema } from '../schema';
import { useServeStore } from '../store';
import { debounce } from '../util/debounce';
import { Icon } from './Icon';
import { SchemaTree } from './schema-tree/SchemaTree';

interface Props {
  databaseId: string;
}

export function SchemaPane({ databaseId }: Props) {
  const showInternalObjects = useServeStore((s) => s.showInternalObjects);
  const setShowInternalObjects = useServeStore((s) => s.setShowInternalObjects);

  const query = useQuery({
    queryKey: ['schema', databaseId, showInternalObjects],
    queryFn: async () => {
      // Request object definitions and comments so the tree's View/Copy
      // definition and View/Copy comment actions have data. The server
      // omits both by default to keep the payload light, so these opt-ins
      // are required.
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

  const queryClient = useQueryClient();
  const refresh = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: ['schema', databaseId] });
  }, [queryClient, databaseId]);

  const [searchInput, setSearchInput] = useState('');
  const [searchTerm, setSearchTerm] = useState('');
  const debouncedSetSearchTerm = useMemo(
    () => debounce(setSearchTerm, 150),
    [],
  );
  useEffect(() => debouncedSetSearchTerm.cancel, [debouncedSetSearchTerm]);

  return (
    <div className="flex h-full min-w-0 flex-col">
      <div className="flex items-center gap-1 border-b border-slate-200 bg-slate-50 px-2 py-1.5">
        <input
          type="search"
          value={searchInput}
          onChange={(e) => {
            setSearchInput(e.target.value);
            debouncedSetSearchTerm(e.target.value.trim());
          }}
          placeholder="Search schema…"
          className="min-w-0 flex-auto rounded border border-slate-300 bg-white px-2 py-1 text-sm focus:border-slate-500 focus:outline-none"
          aria-label="Search schema"
        />
        <button
          type="button"
          onClick={() => setShowInternalObjects(!showInternalObjects)}
          aria-pressed={showInternalObjects}
          className={`rounded p-1 ${
            showInternalObjects
              ? 'bg-blue-50 text-blue-600 hover:bg-blue-100 hover:text-blue-700'
              : 'text-slate-400 hover:bg-slate-200 hover:text-slate-900'
          }`}
          aria-label="Toggle internal objects"
          title={
            showInternalObjects
              ? 'Hide internal objects (system schemas and extension-owned objects)'
              : 'Show internal objects (system schemas and extension-owned objects)'
          }
        >
          <Icon name={showInternalObjects ? 'eye' : 'eye-off'} size={14} />
        </button>
        <button
          type="button"
          onClick={refresh}
          disabled={query.isFetching}
          className="rounded p-1 text-slate-500 hover:bg-slate-200 hover:text-slate-900 disabled:cursor-not-allowed disabled:opacity-50"
          aria-label="Refresh schema"
          title="Refresh schema"
        >
          <Icon
            name="refresh"
            size={14}
            className={query.isFetching ? 'animate-spin' : ''}
          />
        </button>
      </div>
      <div className="flex-auto overflow-auto">
        {query.isError ? (
          <div className="p-4 text-sm text-red-600">
            {(query.error as Error).message}
          </div>
        ) : !query.data ? (
          <div className="p-4 text-sm text-slate-500">Loading…</div>
        ) : !query.data.schemas?.length ? (
          <div className="p-4 text-sm text-slate-500">
            No user-visible schemas.
          </div>
        ) : (
          <SchemaTree
            databaseId={databaseId}
            schemas={query.data.schemas}
            searchTerm={searchTerm}
          />
        )}
      </div>
    </div>
  );
}
