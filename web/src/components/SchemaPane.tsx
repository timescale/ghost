import { useQueryClient } from '@tanstack/react-query';
import { useCallback, useEffect, useMemo, useState } from 'react';

import { useSchemaQuery } from '../hooks/useSchemaQuery';
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

  // Shared with the autocomplete index (useAutocompletePlugin) so both consume
  // a single cached /api/schema fetch per database + internal-objects toggle.
  const query = useSchemaQuery(databaseId);

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
