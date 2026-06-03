import { useQuery, useQueryClient } from '@tanstack/react-query';
import {
  type MouseEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';

import {
  type DatabaseSchema,
  type NamespacedSchema,
  qualifiedName,
  quoteIdent,
  type Routine,
  selectAllSql,
  type TableColumn,
  type TableSchema,
  type ViewSchema,
} from '../schema';
import { useServeStore } from '../store';

interface SchemaPaneProps {
  databaseId: string;
}

async function fetchSchema(databaseId: string): Promise<DatabaseSchema> {
  const params = new URLSearchParams({ databaseId });
  const res = await fetch(`/api/schema?${params}`);
  if (!res.ok) {
    throw new Error(`/api/schema: ${res.status} ${await res.text()}`);
  }
  return res.json() as Promise<DatabaseSchema>;
}

// SchemaPane renders the schema tree for a single database. Data is fetched
// on demand via /api/schema (cached by TanStack Query); the user can refresh
// manually with the Refresh button. The tree's expansion state is persisted
// per-database in the Zustand store.
export function SchemaPane({ databaseId }: SchemaPaneProps) {
  const query = useQuery({
    queryKey: ['schema', databaseId],
    queryFn: () => fetchSchema(databaseId),
    staleTime: 60_000,
  });

  const queryClient = useQueryClient();
  const refresh = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: ['schema', databaseId] });
  }, [queryClient, databaseId]);

  const [searchInput, setSearchInput] = useState('');
  const [searchTerm, setSearchTerm] = useState('');

  // Debounce the search input so we don't re-filter on every keystroke.
  useEffect(() => {
    const id = setTimeout(() => setSearchTerm(searchInput.trim()), 150);
    return () => clearTimeout(id);
  }, [searchInput]);

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center gap-1 border-b border-slate-200 bg-slate-50 px-2 py-1.5">
        <input
          type="search"
          value={searchInput}
          onChange={(e) => setSearchInput(e.target.value)}
          placeholder="Search schema…"
          className="flex-auto rounded border border-slate-300 bg-white px-2 py-1 text-sm focus:border-slate-500 focus:outline-none"
          aria-label="Search schema"
        />
        <button
          type="button"
          onClick={refresh}
          disabled={query.isFetching}
          className="rounded p-1 text-slate-500 hover:bg-slate-200 hover:text-slate-900 disabled:cursor-not-allowed disabled:opacity-50"
          aria-label="Refresh schema"
          title="Refresh schema"
        >
          <RefreshIcon className={query.isFetching ? 'animate-spin' : ''} />
        </button>
      </div>
      <div className="flex-auto overflow-auto">
        <SchemaTreeBody
          query={query}
          databaseId={databaseId}
          searchTerm={searchTerm}
        />
      </div>
    </div>
  );
}

interface SchemaTreeBodyProps {
  query: ReturnType<typeof useQuery<DatabaseSchema>>;
  databaseId: string;
  searchTerm: string;
}

function SchemaTreeBody({
  query,
  databaseId,
  searchTerm,
}: SchemaTreeBodyProps) {
  if (query.isError) {
    return (
      <div className="p-4 text-sm text-red-600">
        {(query.error as Error).message}
      </div>
    );
  }
  if (!query.data) {
    return <div className="p-4 text-sm text-slate-500">Loading…</div>;
  }
  const schemas = query.data.schemas ?? [];
  if (schemas.length === 0) {
    return (
      <div className="p-4 text-sm text-slate-500">No user-visible schemas.</div>
    );
  }
  return (
    <SchemaTree
      databaseId={databaseId}
      schemas={schemas}
      searchTerm={searchTerm}
    />
  );
}

// ---- Tree implementation ---------------------------------------------------

interface TreeContext {
  databaseId: string;
  expanded: Set<string>;
  searchExpanded: Set<string>;
  searchActive: boolean;
  searchMatches: Set<string> | null;
  searchTerm: string;
  toggle: (key: string) => void;
  setContextMenu: (m: ContextMenuState | null) => void;
}

interface SchemaTreeProps {
  databaseId: string;
  schemas: NamespacedSchema[];
  searchTerm: string;
}

function SchemaTree({ databaseId, schemas, searchTerm }: SchemaTreeProps) {
  const expandedList = useServeStore(
    (s) => s.schemaTreeExpanded[databaseId] ?? EMPTY_LIST,
  );
  const toggle = useServeStore((s) => s.toggleSchemaNode);

  const expanded = useMemo(() => new Set(expandedList), [expandedList]);

  // Pre-compute search match info: which node keys contain or are ancestors
  // of a match, so we can both show matches and auto-expand to reveal them.
  const search = useMemo(
    () => computeSearch(schemas, searchTerm),
    [schemas, searchTerm],
  );

  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null);

  const ctx = useMemo<TreeContext>(
    () => ({
      databaseId,
      expanded,
      searchExpanded: search.expanded,
      searchActive: searchTerm.length > 0,
      searchMatches: search.visible,
      searchTerm,
      toggle: (key) => toggle(databaseId, key),
      setContextMenu,
    }),
    [databaseId, expanded, search, searchTerm, toggle],
  );

  return (
    <div className="pb-2 text-sm">
      {schemas.map((ns) =>
        ctx.searchActive && !ctx.searchMatches?.has(schemaKey(ns)) ? null : (
          <SchemaNode key={ns.name} ns={ns} ctx={ctx} />
        ),
      )}
      {contextMenu ? (
        <ContextMenu state={contextMenu} onClose={() => setContextMenu(null)} />
      ) : null}
    </div>
  );
}

const EMPTY_LIST: string[] = [];

// ---- Node renderers --------------------------------------------------------

interface NodeProps {
  ctx: TreeContext;
}

interface SchemaNodeProps extends NodeProps {
  ns: NamespacedSchema;
}

function SchemaNode({ ns, ctx }: SchemaNodeProps) {
  const key = schemaKey(ns);
  const tables = ns.tables ?? [];
  const views = ns.views ?? [];
  const matViews = ns.materialized_views ?? [];
  const funcs = ns.functions ?? [];
  const procs = ns.procedures ?? [];
  const enums = ns.enums ?? [];

  const groups: GroupSpec[] = [
    { kind: 'tables', label: 'Tables', items: tables },
    { kind: 'views', label: 'Views', items: views },
    { kind: 'matViews', label: 'Materialized Views', items: matViews },
    { kind: 'functions', label: 'Functions', items: funcs },
    { kind: 'procedures', label: 'Procedures', items: procs },
    { kind: 'enums', label: 'Enums', items: enums },
  ];

  return (
    <TreeRow
      ctx={ctx}
      nodeKey={key}
      label={ns.name}
      icon="🗂"
      hasChildren={groups.some((g) => (g.items ?? []).length > 0)}
      onContextMenu={(e) =>
        ctx.setContextMenu({
          x: e.clientX,
          y: e.clientY,
          items: schemaMenuItems(ns.name),
        })
      }
    >
      {groups.map((g) =>
        (g.items ?? []).length === 0 ? null : (
          <GroupNode key={g.kind} ns={ns.name} group={g} ctx={ctx} />
        ),
      )}
    </TreeRow>
  );
}

type GroupKind =
  | 'tables'
  | 'views'
  | 'matViews'
  | 'functions'
  | 'procedures'
  | 'enums';

interface GroupSpec {
  kind: GroupKind;
  label: string;
  items:
    | TableSchema[]
    | ViewSchema[]
    | Routine[]
    | NamespacedSchema['enums']
    | undefined;
}

interface GroupNodeProps extends NodeProps {
  ns: string;
  group: GroupSpec;
}

function renderItem(
  ns: string,
  kind: GroupKind,
  item: { name: string },
  ctx: TreeContext,
): React.ReactNode {
  const itemKey = childKey(ns, kind, item.name);
  switch (kind) {
    case 'tables':
      return (
        <TableNode
          key={itemKey}
          ns={ns}
          table={item as TableSchema}
          ctx={ctx}
        />
      );
    case 'views':
      return (
        <ViewNode
          key={itemKey}
          ns={ns}
          view={item as ViewSchema}
          kind="view"
          ctx={ctx}
        />
      );
    case 'matViews':
      return (
        <ViewNode
          key={itemKey}
          ns={ns}
          view={item as ViewSchema}
          kind="matview"
          ctx={ctx}
        />
      );
    case 'functions':
      return (
        <RoutineNode
          key={itemKey}
          ns={ns}
          routine={item as Routine}
          ctx={ctx}
        />
      );
    case 'procedures':
      return (
        <RoutineNode
          key={itemKey}
          ns={ns}
          routine={item as Routine}
          ctx={ctx}
        />
      );
    case 'enums':
      return (
        <EnumNode
          key={itemKey}
          ns={ns}
          enum_={item as NonNullable<NamespacedSchema['enums']>[number]}
          ctx={ctx}
        />
      );
  }
}

function GroupNode({ ns, group, ctx }: GroupNodeProps) {
  const key = `${schemaKey({ name: ns })}/${group.kind}`;
  const items = group.items ?? [];
  const visibleItems = ctx.searchActive
    ? items.filter((item) => {
        const itemKey = childKey(ns, group.kind, item.name);
        return ctx.searchMatches?.has(itemKey);
      })
    : items;
  if (visibleItems.length === 0) return null;

  return (
    <TreeRow
      ctx={ctx}
      nodeKey={key}
      label={`${group.label} (${visibleItems.length})`}
      icon=""
      hasChildren
    >
      {visibleItems.map((item) => renderItem(ns, group.kind, item, ctx))}
    </TreeRow>
  );
}

interface TableNodeProps extends NodeProps {
  ns: string;
  table: TableSchema;
}

function TableNode({ ns, table, ctx }: TableNodeProps) {
  const key = childKey(ns, 'tables', table.name);
  const indexes = table.indexes ?? [];
  const triggers = table.triggers ?? [];

  return (
    <TreeRow
      ctx={ctx}
      nodeKey={key}
      label={table.name}
      icon="📋"
      badge={
        table.hypertable
          ? `hyper · ${table.hypertable.num_chunks}c${
              table.hypertable.compression_enabled ? ' · zip' : ''
            }`
          : undefined
      }
      hasChildren
      onContextMenu={(e) =>
        ctx.setContextMenu({
          x: e.clientX,
          y: e.clientY,
          items: tableMenuItems(ns, table, 'table'),
        })
      }
    >
      <ColumnGroup ns={ns} parent={table} parentKind="tables" ctx={ctx} />
      {indexes.length > 0 ? (
        <SubGroup
          ctx={ctx}
          parentKey={key}
          subKey="indexes"
          label={`Indexes (${indexes.length})`}
        >
          {indexes.map((idx) => (
            <LeafRow
              key={idx.name}
              ctx={ctx}
              nodeKey={`${key}/indexes/${idx.name}`}
              icon="🔑"
              label={idx.name}
              detail={idx.columns}
            />
          ))}
        </SubGroup>
      ) : null}
      {triggers.length > 0 ? (
        <SubGroup
          ctx={ctx}
          parentKey={key}
          subKey="triggers"
          label={`Triggers (${triggers.length})`}
        >
          {triggers.map((trg) => (
            <LeafRow
              key={`${trg.name}/${trg.manipulation}`}
              ctx={ctx}
              nodeKey={`${key}/triggers/${trg.name}/${trg.manipulation}`}
              icon="⚡"
              label={trg.name}
              detail={`${trg.timing} ${trg.manipulation}`}
            />
          ))}
        </SubGroup>
      ) : null}
    </TreeRow>
  );
}

interface ColumnGroupProps extends NodeProps {
  ns: string;
  parent: TableSchema | ViewSchema;
  parentKind: 'tables' | 'views' | 'matViews';
}

// ColumnGroup is rendered for tables only — views and matviews flatten
// their columns directly under the view node (popsql-style). Indexes are
// only present on materialized views via the parent.
function ColumnGroup({ ns, parent, parentKind, ctx }: ColumnGroupProps) {
  const cols = (parent.columns ?? []) as TableColumn[];
  if (cols.length === 0) return null;
  const parentKey = childKey(ns, parentKind, parent.name);
  return (
    <SubGroup
      ctx={ctx}
      parentKey={parentKey}
      subKey="columns"
      label={`Columns (${cols.length})`}
    >
      {cols.map((col) => (
        <LeafRow
          key={col.name}
          ctx={ctx}
          nodeKey={`${parentKey}/columns/${col.name}`}
          icon={columnIcon(parent, col.name)}
          label={col.name}
          detail={formatColumnType(col)}
          onContextMenu={(e) =>
            ctx.setContextMenu({
              x: e.clientX,
              y: e.clientY,
              items: columnMenuItems(ns, parent.name, col.name),
            })
          }
        />
      ))}
    </SubGroup>
  );
}

interface ViewNodeProps extends NodeProps {
  ns: string;
  view: ViewSchema;
  kind: 'view' | 'matview';
}

function ViewNode({ ns, view, kind, ctx }: ViewNodeProps) {
  const groupKind = kind === 'view' ? 'views' : 'matViews';
  const key = childKey(ns, groupKind, view.name);
  const cols = view.columns ?? [];
  const indexes = view.indexes ?? [];

  return (
    <TreeRow
      ctx={ctx}
      nodeKey={key}
      label={view.name}
      icon={kind === 'view' ? '👁' : '💾'}
      hasChildren
      onContextMenu={(e) =>
        ctx.setContextMenu({
          x: e.clientX,
          y: e.clientY,
          items: tableMenuItems(
            ns,
            view as unknown as TableSchema,
            kind === 'view' ? 'view' : 'materialized view',
          ),
        })
      }
    >
      {cols.map((col) => (
        <LeafRow
          key={col.name}
          ctx={ctx}
          nodeKey={`${key}/columns/${col.name}`}
          icon="·"
          label={col.name}
          detail={col.type.toUpperCase()}
          onContextMenu={(e) =>
            ctx.setContextMenu({
              x: e.clientX,
              y: e.clientY,
              items: columnMenuItems(ns, view.name, col.name),
            })
          }
        />
      ))}
      {indexes.length > 0 ? (
        <SubGroup
          ctx={ctx}
          parentKey={key}
          subKey="indexes"
          label={`Indexes (${indexes.length})`}
        >
          {indexes.map((idx) => (
            <LeafRow
              key={idx.name}
              ctx={ctx}
              nodeKey={`${key}/indexes/${idx.name}`}
              icon="🔑"
              label={idx.name}
              detail={idx.columns}
            />
          ))}
        </SubGroup>
      ) : null}
    </TreeRow>
  );
}

interface RoutineNodeProps extends NodeProps {
  ns: string;
  routine: Routine;
}

function RoutineNode({ ns, routine, ctx }: RoutineNodeProps) {
  const key = childKey(
    ns,
    routine.type === 'FUNCTION' ? 'functions' : 'procedures',
    routine.name,
  );
  return (
    <LeafRow
      ctx={ctx}
      nodeKey={key}
      icon={routine.type === 'FUNCTION' ? 'ƒ' : '⚙'}
      label={routine.name}
      onContextMenu={(e) =>
        ctx.setContextMenu({
          x: e.clientX,
          y: e.clientY,
          items: routineMenuItems(ns, routine.name),
        })
      }
    />
  );
}

interface EnumNodeProps extends NodeProps {
  ns: string;
  enum_: NonNullable<NamespacedSchema['enums']>[number];
}

function EnumNode({ ns, enum_, ctx }: EnumNodeProps) {
  const key = childKey(ns, 'enums', enum_.name);
  return (
    <LeafRow
      ctx={ctx}
      nodeKey={key}
      icon="≡"
      label={enum_.name}
      detail={(enum_.values ?? []).join(', ')}
      onContextMenu={(e) =>
        ctx.setContextMenu({
          x: e.clientX,
          y: e.clientY,
          items: routineMenuItems(ns, enum_.name),
        })
      }
    />
  );
}

// ---- Row primitives --------------------------------------------------------

interface SubGroupProps extends NodeProps {
  parentKey: string;
  subKey: string;
  label: string;
  children: React.ReactNode;
}

function SubGroup({ parentKey, subKey, label, ctx, children }: SubGroupProps) {
  const key = `${parentKey}/${subKey}`;
  return (
    <TreeRow ctx={ctx} nodeKey={key} label={label} icon="" hasChildren>
      {children}
    </TreeRow>
  );
}

interface TreeRowProps {
  ctx: TreeContext;
  nodeKey: string;
  label: string;
  icon: string;
  badge?: string;
  hasChildren?: boolean;
  onContextMenu?: (e: MouseEvent<HTMLDivElement>) => void;
  children?: React.ReactNode;
}

function TreeRow({
  ctx,
  nodeKey,
  label,
  icon,
  badge,
  hasChildren,
  onContextMenu,
  children,
}: TreeRowProps) {
  const isExpanded =
    ctx.expanded.has(nodeKey) || ctx.searchExpanded.has(nodeKey);
  const depth = nodeKey.split('/').length - 1;

  return (
    <>
      <div
        role={hasChildren ? 'button' : undefined}
        tabIndex={hasChildren ? 0 : undefined}
        className="group flex cursor-default items-center gap-1 px-1 py-0.5 hover:bg-blue-50"
        style={{ paddingLeft: 4 + depth * 12 }}
        onClick={hasChildren ? () => ctx.toggle(nodeKey) : undefined}
        onKeyDown={
          hasChildren
            ? (e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  ctx.toggle(nodeKey);
                }
              }
            : undefined
        }
        onContextMenu={onContextMenu}
      >
        <span className="w-3 text-xs text-slate-400">
          {hasChildren ? (isExpanded ? '▾' : '▸') : ''}
        </span>
        {icon ? (
          <span className="w-4 text-center text-xs">{icon}</span>
        ) : (
          <span className="w-4" />
        )}
        <span
          className={
            ctx.searchActive && ctx.searchMatches?.has(nodeKey)
              ? 'text-slate-900'
              : 'text-slate-700'
          }
        >
          {highlight(label, ctx.searchTerm)}
        </span>
        {badge ? (
          <span className="ml-1 rounded bg-purple-100 px-1 py-0 text-xs text-purple-700">
            {badge}
          </span>
        ) : null}
      </div>
      {isExpanded ? children : null}
    </>
  );
}

interface LeafRowProps {
  ctx: TreeContext;
  nodeKey: string;
  icon: string;
  label: string;
  detail?: string;
  onContextMenu?: (e: MouseEvent<HTMLDivElement>) => void;
}

function LeafRow({
  ctx,
  nodeKey,
  icon,
  label,
  detail,
  onContextMenu,
}: LeafRowProps) {
  const depth = nodeKey.split('/').length - 1;
  return (
    <div
      className="group flex cursor-default items-center gap-1 px-1 py-0.5 hover:bg-blue-50"
      style={{ paddingLeft: 4 + depth * 12 }}
      onContextMenu={onContextMenu}
    >
      <span className="w-3" />
      <span className="w-4 text-center text-xs text-slate-500">{icon}</span>
      <span className="truncate text-slate-700">
        {highlight(label, ctx.searchTerm)}
      </span>
      {detail ? (
        <span className="ml-2 truncate text-xs text-slate-400">{detail}</span>
      ) : null}
    </div>
  );
}

// ---- Helpers ---------------------------------------------------------------

function schemaKey(ns: { name: string }): string {
  return `schema:${ns.name}`;
}

function childKey(ns: string, group: GroupKind, name: string): string {
  return `${schemaKey({ name: ns })}/${group}/${name}`;
}

function columnIcon(parent: TableSchema | ViewSchema, name: string): string {
  const t = parent as TableSchema;
  const isPk = (t.constraints ?? []).some(
    (c) => c.type === 'PRIMARY KEY' && (c.columns ?? []).includes(name),
  );
  if (isPk) return '🔑';
  return '·';
}

function formatColumnType(col: TableColumn): string {
  let s = col.type.toUpperCase();
  if (col.not_null) s += ' NOT NULL';
  if (col.default) s += ` DEFAULT ${col.default}`;
  return s;
}

function highlight(text: string, term: string): React.ReactNode {
  if (!term) return text;
  const idx = text.toLowerCase().indexOf(term.toLowerCase());
  if (idx < 0) return text;
  const before = text.slice(0, idx);
  const match = text.slice(idx, idx + term.length);
  const after = text.slice(idx + term.length);
  return (
    <>
      {before}
      <mark className="bg-yellow-200">{match}</mark>
      {after}
    </>
  );
}

interface SearchInfo {
  visible: Set<string>;
  expanded: Set<string>;
}

// computeSearch walks the schema tree and collects, for the current search
// term, (a) every node key that should remain visible (matches itself OR has
// a descendant that matches OR is an ancestor of a match), and (b) every
// non-leaf key that should be auto-expanded so matches are revealed.
function computeSearch(schemas: NamespacedSchema[], term: string): SearchInfo {
  const visible = new Set<string>();
  const expanded = new Set<string>();
  if (!term) return { visible, expanded };
  const lower = term.toLowerCase();
  const match = (s: string) => s.toLowerCase().includes(lower);

  for (const ns of schemas) {
    const sKey = schemaKey(ns);
    const nsHit = match(ns.name);
    void nsHit;

    const considerGroup = (
      kind: GroupKind,
      items: { name: string; columns?: { name: string }[] }[] | undefined,
    ): boolean => {
      const list = items ?? [];
      if (list.length === 0) return false;
      const gKey = `${sKey}/${kind}`;
      let groupHit = false;
      for (const item of list) {
        const iKey = childKey(ns.name, kind, item.name);
        const itemHit = match(item.name);
        let childHit = false;
        if ('columns' in item && item.columns) {
          for (const col of item.columns) {
            if (match(col.name)) {
              visible.add(`${iKey}/columns/${col.name}`);
              visible.add(`${iKey}/columns`);
              expanded.add(iKey);
              expanded.add(`${iKey}/columns`);
              childHit = true;
            }
          }
        }
        if (itemHit || childHit) {
          visible.add(iKey);
          groupHit = true;
        }
      }
      if (groupHit) {
        visible.add(gKey);
        expanded.add(sKey);
        expanded.add(gKey);
      }
      return groupHit;
    };

    let anyHit = nsHit;
    for (const [kind, items] of [
      ['tables', ns.tables],
      ['views', ns.views],
      ['matViews', ns.materialized_views],
      ['functions', ns.functions],
      ['procedures', ns.procedures],
      ['enums', ns.enums],
    ] as const) {
      if (considerGroup(kind as GroupKind, items as never)) {
        anyHit = true;
      }
    }
    if (anyHit) {
      visible.add(sKey);
    }
  }
  return { visible, expanded };
}

// ---- Context menu ---------------------------------------------------------

interface ContextMenuState {
  x: number;
  y: number;
  items: MenuItem[];
}

interface MenuItem {
  label: string;
  onClick: () => void;
}

interface ContextMenuProps {
  state: ContextMenuState;
  onClose: () => void;
}

function ContextMenu({ state, onClose }: ContextMenuProps) {
  const ref = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    const onDown = (e: MouseEvent | globalThis.MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    // Defer attach so the right-click event that opened the menu doesn't
    // immediately close it.
    const id = setTimeout(() => {
      window.addEventListener('mousedown', onDown);
      window.addEventListener('keydown', onKey);
    }, 0);
    return () => {
      clearTimeout(id);
      window.removeEventListener('mousedown', onDown);
      window.removeEventListener('keydown', onKey);
    };
  }, [onClose]);

  return (
    <div
      ref={ref}
      role="menu"
      className="fixed z-50 min-w-[180px] rounded border border-slate-200 bg-white py-1 text-sm shadow-lg"
      style={{ top: state.y, left: state.x }}
    >
      {state.items.map((item) => (
        <button
          key={item.label}
          type="button"
          role="menuitem"
          onClick={() => {
            item.onClick();
            onClose();
          }}
          className="block w-full px-3 py-1 text-left hover:bg-blue-50"
        >
          {item.label}
        </button>
      ))}
    </div>
  );
}

// ---- Menu actions ---------------------------------------------------------

function copyToClipboard(text: string) {
  void navigator.clipboard.writeText(text);
}

function schemaMenuItems(name: string): MenuItem[] {
  const append = useServeStore.getState().appendEditorSql;
  return [
    {
      label: `New query: SET search_path TO ${quoteIdent(name)}`,
      onClick: () => append(`SET search_path TO ${quoteIdent(name)};`),
    },
    {
      label: 'Copy schema name',
      onClick: () => copyToClipboard(quoteIdent(name)),
    },
  ];
}

function tableMenuItems(
  ns: string,
  table: TableSchema,
  kind: 'table' | 'view' | 'materialized view',
): MenuItem[] {
  const append = useServeStore.getState().appendEditorSql;
  const cols = table.columns ?? [];
  const selectSql = selectAllSql(ns, table.name, cols);
  return [
    {
      label: `New query from ${kind}`,
      onClick: () => append(selectSql),
    },
    {
      label: 'Copy SELECT statement',
      onClick: () => copyToClipboard(selectSql),
    },
    {
      label: `Copy ${kind} name`,
      onClick: () => copyToClipboard(qualifiedName(ns, table.name)),
    },
  ];
}

function columnMenuItems(ns: string, table: string, col: string): MenuItem[] {
  const append = useServeStore.getState().appendEditorSql;
  const sql = `SELECT ${quoteIdent(col)} FROM ${qualifiedName(ns, table)} LIMIT 100;`;
  return [
    {
      label: 'New query with column',
      onClick: () => append(sql),
    },
    {
      label: 'Copy SELECT statement',
      onClick: () => copyToClipboard(sql),
    },
    {
      label: 'Copy column name',
      onClick: () => copyToClipboard(quoteIdent(col)),
    },
    {
      label: 'Copy qualified column name',
      onClick: () =>
        copyToClipboard(`${qualifiedName(ns, table)}.${quoteIdent(col)}`),
    },
  ];
}

function routineMenuItems(ns: string, name: string): MenuItem[] {
  return [
    {
      label: 'Copy qualified name',
      onClick: () => copyToClipboard(qualifiedName(ns, name)),
    },
  ];
}

// ---- Icons -----------------------------------------------------------------

function RefreshIcon({ className = '' }: { className?: string }) {
  return (
    <svg
      className={`h-4 w-4 ${className}`}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M3 12a9 9 0 0 1 15.5-6.4L21 8" />
      <path d="M21 3v5h-5" />
      <path d="M21 12a9 9 0 0 1-15.5 6.4L3 16" />
      <path d="M3 21v-5h5" />
    </svg>
  );
}
