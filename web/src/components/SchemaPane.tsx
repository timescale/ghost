import { useQuery, useQueryClient } from '@tanstack/react-query';
import {
  type MouseEvent,
  type ReactNode,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';

import {
  type DatabaseSchema,
  type IndexSchema,
  type NamespacedSchema,
  qualifiedName,
  quoteIdent,
  type Routine,
  selectAllSql,
  type TableColumn,
  type TableSchema,
  type TriggerSchema,
  type ViewSchema,
} from '../schema';
import { useServeStore } from '../store';

import {
  ChevronDown,
  CopyIcon,
  EyeIcon,
  type IconProps,
  NavQueriesPlus,
  NavSuperscript,
  NavTable,
  RefreshIcon,
} from './SchemaIcons';

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

  useEffect(() => {
    const id = setTimeout(() => setSearchTerm(searchInput.trim()), 150);
    return () => clearTimeout(id);
  }, [searchInput]);

  return (
    <div className="flex h-full min-w-0 flex-col">
      <div className="flex items-center gap-1 border-b border-slate-200 bg-slate-50 px-2 py-1.5">
        <input
          type="search"
          value={searchInput}
          onChange={(e) => setSearchInput(e.target.value)}
          placeholder="Search schema…"
          className="min-w-0 flex-auto rounded border border-slate-300 bg-white px-2 py-1 text-sm focus:border-slate-500 focus:outline-none"
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

// Indent layout matches popsql: each indent column is a fixed-width span
// carrying the vertical guide line on its right edge. The guide line sits at
// half the caret slot width, which centers it under the ancestor chevron.
const CARET_PX = 12;
const INDENT_STEP_PX = 20;
const INDENT_PAD = CARET_PX / 2;
const INDENT_GAP = INDENT_STEP_PX - INDENT_PAD;

interface TreeContext {
  expanded: Set<string>;
  // Nodes the user has explicitly collapsed while a search is active. During
  // search every rendered node is expanded by default (matching popsql), so we
  // only need to track the exceptions. This set is transient: it lives in
  // component state and is reset whenever the search term changes, so toggling
  // a node while searching never mutates the persisted `expanded` state of the
  // unfiltered tree.
  collapsedDuringSearch: Set<string>;
  searchActive: boolean;
  searchMatches: Set<string> | null;
  searchTerm: string;
  toggle: (key: string) => void;
  setContextMenu: (m: ContextMenuState | null) => void;
  showModal: (title: string, text: string) => void;
}

interface SchemaTreeProps {
  databaseId: string;
  schemas: NamespacedSchema[];
  searchTerm: string;
}

// Whether an expandable node should render its children. While a search is
// active every node is expanded by default (so all matches are visible) unless
// the user has explicitly collapsed it; otherwise we consult the persisted
// expanded state.
function nodeExpanded(ctx: TreeContext, nodeKey: string): boolean {
  return ctx.searchActive
    ? !ctx.collapsedDuringSearch.has(nodeKey)
    : ctx.expanded.has(nodeKey);
}

function SchemaTree({ databaseId, schemas, searchTerm }: SchemaTreeProps) {
  const expandedList = useServeStore(
    (s) => s.schemaTreeExpanded[databaseId] ?? EMPTY_LIST,
  );
  const toggle = useServeStore((s) => s.toggleSchemaNode);
  const toggleForDb = useCallback(
    (key: string) => toggle(databaseId, key),
    [toggle, databaseId],
  );

  const expanded = useMemo(() => new Set(expandedList), [expandedList]);

  const search = useMemo(
    () => computeSearch(schemas, searchTerm),
    [schemas, searchTerm],
  );
  const searchActive = searchTerm.length > 0;

  // Transient per-search collapse overrides. Reset whenever the search term
  // changes so a fresh search always starts fully expanded, and so collapses
  // made during one search never carry over to the next (or to the unfiltered
  // view).
  const [collapsedDuringSearch, setCollapsedDuringSearch] = useState<
    Set<string>
  >(() => new Set());
  // biome-ignore lint/correctness/useExhaustiveDependencies: searchTerm is the reset trigger, not read in the body
  useEffect(() => {
    setCollapsedDuringSearch(new Set());
  }, [searchTerm]);

  const toggleCollapsedDuringSearch = useCallback((key: string) => {
    setCollapsedDuringSearch((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }, []);

  // While searching, toggling adjusts the transient collapse set; otherwise it
  // mutates the persisted expanded state.
  const toggleNode = useCallback(
    (key: string) => {
      if (searchActive) toggleCollapsedDuringSearch(key);
      else toggleForDb(key);
    },
    [searchActive, toggleCollapsedDuringSearch, toggleForDb],
  );

  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null);
  const [definitionModal, setDefinitionModal] = useState<{
    title: string;
    text: string;
  } | null>(null);

  const ctx = useMemo<TreeContext>(
    () => ({
      expanded,
      collapsedDuringSearch,
      searchActive,
      searchMatches: search.visible,
      searchTerm,
      toggle: toggleNode,
      setContextMenu,
      showModal: (title, text) => setDefinitionModal({ title, text }),
    }),
    [
      expanded,
      collapsedDuringSearch,
      searchActive,
      search,
      searchTerm,
      toggleNode,
    ],
  );

  return (
    <div className="select-none py-1 text-[13px] leading-[1.4]">
      {schemas.map((ns) =>
        ctx.searchActive && !ctx.searchMatches?.has(schemaKey(ns)) ? null : (
          <SchemaNode key={ns.name} ns={ns} ctx={ctx} />
        ),
      )}
      {contextMenu ? (
        <ContextMenu state={contextMenu} onClose={() => setContextMenu(null)} />
      ) : null}
      {definitionModal ? (
        <DefinitionModal
          title={definitionModal.title}
          text={definitionModal.text}
          onClose={() => setDefinitionModal(null)}
        />
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
  const groups = schemaGroups(ns);

  return (
    <SchemaRootRow
      ctx={ctx}
      nodeKey={key}
      label={ns.name}
      hasChildren
      onContextMenu={(e) => {
        e.preventDefault();
        ctx.setContextMenu({
          x: e.clientX,
          y: e.clientY,
          items: schemaMenuItems(ns.name),
        });
      }}
    >
      {groups.map((g) => (
        <GroupNode key={g.kind} ns={ns.name} group={g} ctx={ctx} />
      ))}
    </SchemaRootRow>
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
  items: Array<TableSchema | ViewSchema | Routine | { name: string }>;
}

function schemaGroups(ns: NamespacedSchema): GroupSpec[] {
  return [
    { kind: 'tables', label: 'Tables', items: ns.tables ?? [] },
    { kind: 'views', label: 'Views', items: ns.views ?? [] },
    {
      kind: 'matViews',
      label: 'Materialized Views',
      items: ns.materialized_views ?? [],
    },
    { kind: 'functions', label: 'Functions', items: ns.functions ?? [] },
    { kind: 'procedures', label: 'Procedures', items: ns.procedures ?? [] },
    { kind: 'enums', label: 'Enums', items: ns.enums ?? [] },
  ];
}

function groupIcon(kind: GroupKind): ReactNode {
  switch (kind) {
    case 'tables':
    case 'views':
    case 'matViews':
      return <NavTable />;
    case 'functions':
    case 'procedures':
      return <NavSuperscript />;
    case 'enums':
      return null;
  }
}

interface GroupNodeProps extends NodeProps {
  ns: string;
  group: GroupSpec;
}

function GroupNode({ ns, group, ctx }: GroupNodeProps) {
  const key = `schema:${ns}/${group.kind}`;
  const items = group.items;
  const visibleItems = ctx.searchActive
    ? items.filter((item) =>
        ctx.searchMatches?.has(childKey(ns, group.kind, item.name)),
      )
    : items;
  if (visibleItems.length === 0) return null;

  return (
    <TreeRow
      ctx={ctx}
      nodeKey={key}
      label={group.label}
      depth={1}
      icon={groupIcon(group.kind)}
      hasChildren={visibleItems.length > 0}
      count={ctx.searchActive ? visibleItems.length : items.length}
    >
      {visibleItems.map((item) => renderGroupItem(ns, group.kind, item, ctx))}
    </TreeRow>
  );
}

function renderGroupItem(
  ns: string,
  kind: GroupKind,
  item: TableSchema | ViewSchema | Routine | { name: string },
  ctx: TreeContext,
): ReactNode {
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
          enum_={item as { name: string; values?: string[] }}
          ctx={ctx}
        />
      );
  }
}

interface TableNodeProps extends NodeProps {
  ns: string;
  table: TableSchema;
}

function TableNode({ ns, table, ctx }: TableNodeProps) {
  const key = childKey(ns, 'tables', table.name);
  const allCols = table.columns ?? [];
  const allIndexes = table.indexes ?? [];
  const allTriggers = table.triggers ?? [];
  // When a search is active, only render the children that themselves match.
  // popsql does the same: searching for "plan" inside a multi-column table
  // collapses Columns down to just the matching ones.
  const cols = ctx.searchActive
    ? allCols.filter((c) => ctx.searchMatches?.has(`${key}/columns/${c.name}`))
    : allCols;
  const indexes = ctx.searchActive
    ? allIndexes.filter((i) =>
        ctx.searchMatches?.has(`${key}/indexes/${i.name}`),
      )
    : allIndexes;
  const triggers = ctx.searchActive
    ? allTriggers.filter((t) =>
        ctx.searchMatches?.has(`${key}/triggers/${t.name}`),
      )
    : allTriggers;
  return (
    <TreeRow
      ctx={ctx}
      nodeKey={key}
      label={table.name}
      depth={2}
      hasChildren
      rightDetail={
        table.hypertable ? <HypertableBadge info={table.hypertable} /> : null
      }
      onContextMenu={(e) => {
        e.preventDefault();
        ctx.setContextMenu({
          x: e.clientX,
          y: e.clientY,
          items: tableMenuItems(ns, table, 'table'),
        });
      }}
    >
      {cols.length > 0 ? (
        <TreeRow
          ctx={ctx}
          nodeKey={`${key}/columns`}
          label="Columns"
          depth={3}
          hasChildren
          count={ctx.searchActive ? cols.length : allCols.length}
        >
          {cols.map((col) => (
            <ColumnRow
              key={col.name}
              parent={table}
              ns={ns}
              parentName={table.name}
              col={col}
              ctx={ctx}
            />
          ))}
        </TreeRow>
      ) : null}
      {indexes.length > 0 ? (
        <TreeRow
          ctx={ctx}
          nodeKey={`${key}/indexes`}
          label="Indexes"
          depth={3}
          hasChildren
          count={ctx.searchActive ? indexes.length : allIndexes.length}
        >
          {indexes.map((idx) => (
            <IndexRow key={idx.name} ns={ns} index={idx} ctx={ctx} />
          ))}
        </TreeRow>
      ) : null}
      {triggers.length > 0 ? (
        <TreeRow
          ctx={ctx}
          nodeKey={`${key}/triggers`}
          label="Triggers"
          depth={3}
          hasChildren
          count={ctx.searchActive ? triggers.length : allTriggers.length}
        >
          {triggers.map((trg) => (
            <TriggerRow
              key={`${trg.name}/${trg.timing}/${trg.manipulation}`}
              trigger={trg}
              ctx={ctx}
            />
          ))}
        </TreeRow>
      ) : null}
    </TreeRow>
  );
}

interface ColumnRowProps extends NodeProps {
  parent: TableSchema | ViewSchema;
  ns: string;
  parentName: string;
  col: TableColumn | { name: string; type: string };
}

function ColumnRow({ parent, ns, parentName, col, ctx }: ColumnRowProps) {
  const constraint = columnConstraintLabel(parent, col);
  return (
    <LeafRow
      ctx={ctx}
      label={highlight(col.name, ctx.searchTerm)}
      depth={4}
      rightDetail={
        <>
          {constraint ? <Pill>{constraint}</Pill> : null}
          <Pill>{col.type}</Pill>
        </>
      }
      onContextMenu={(e) => {
        e.preventDefault();
        ctx.setContextMenu({
          x: e.clientX,
          y: e.clientY,
          items: columnMenuItems(ns, parentName, col.name),
        });
      }}
    />
  );
}

interface IndexRowProps extends NodeProps {
  ns: string;
  index: IndexSchema;
}

function IndexRow({ ns, index, ctx }: IndexRowProps) {
  return (
    <LeafRow
      ctx={ctx}
      label={highlight(index.name, ctx.searchTerm)}
      depth={4}
      rightDetail={
        <>
          {index.is_unique ? <Pill>unique</Pill> : null}
          <Pill>{index.columns}</Pill>
        </>
      }
      onContextMenu={(e) => {
        e.preventDefault();
        ctx.setContextMenu({
          x: e.clientX,
          y: e.clientY,
          items: indexMenuItems(ns, index, ctx.showModal),
        });
      }}
    />
  );
}

interface TriggerRowProps extends NodeProps {
  trigger: TriggerSchema;
}

function TriggerRow({ trigger, ctx }: TriggerRowProps) {
  return (
    <LeafRow
      ctx={ctx}
      label={highlight(trigger.name, ctx.searchTerm)}
      depth={4}
      rightDetail={
        <>
          <Pill>{trigger.timing.toLowerCase()}</Pill>
          <Pill>{trigger.manipulation.toLowerCase()}</Pill>
        </>
      }
    />
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
      depth={2}
      hasChildren
      onContextMenu={(e) => {
        e.preventDefault();
        ctx.setContextMenu({
          x: e.clientX,
          y: e.clientY,
          items: tableMenuItems(
            ns,
            view as unknown as TableSchema,
            kind === 'view' ? 'view' : 'materialized view',
          ),
        });
      }}
    >
      {cols.map((col) => (
        <ColumnRow
          key={col.name}
          parent={view}
          ns={ns}
          parentName={view.name}
          col={col}
          ctx={ctx}
        />
      ))}
      {indexes.length > 0 ? (
        <TreeRow
          ctx={ctx}
          nodeKey={`${key}/indexes`}
          label="Indexes"
          depth={3}
          hasChildren
          count={indexes.length}
        >
          {indexes.map((idx) => (
            <IndexRow key={idx.name} ns={ns} index={idx} ctx={ctx} />
          ))}
        </TreeRow>
      ) : null}
    </TreeRow>
  );
}

interface RoutineNodeProps extends NodeProps {
  ns: string;
  routine: Routine;
}

function RoutineNode({ ns, routine, ctx }: RoutineNodeProps) {
  return (
    <LeafRow
      ctx={ctx}
      label={highlight(routine.name, ctx.searchTerm)}
      depth={2}
      onContextMenu={(e) => {
        e.preventDefault();
        ctx.setContextMenu({
          x: e.clientX,
          y: e.clientY,
          items: routineMenuItems(ns, routine.name),
        });
      }}
    />
  );
}

interface EnumNodeProps extends NodeProps {
  ns: string;
  enum_: { name: string; values?: string[] };
}

function EnumNode({ ns, enum_, ctx }: EnumNodeProps) {
  return (
    <LeafRow
      ctx={ctx}
      label={highlight(enum_.name, ctx.searchTerm)}
      depth={2}
      rightDetail={<Pill>{(enum_.values ?? []).join(', ')}</Pill>}
      onContextMenu={(e) => {
        e.preventDefault();
        ctx.setContextMenu({
          x: e.clientX,
          y: e.clientY,
          items: routineMenuItems(ns, enum_.name),
        });
      }}
    />
  );
}

// ---- Row primitives --------------------------------------------------------

interface SchemaRootRowProps extends NodeProps {
  nodeKey: string;
  label: string;
  hasChildren: boolean;
  onContextMenu?: (e: MouseEvent<HTMLDivElement>) => void;
  children?: ReactNode;
}

// Schema row: bold, no left caret, with a hover-revealed right caret. Matches
// popsql's root-level treatment.
function SchemaRootRow({
  nodeKey,
  label,
  hasChildren,
  onContextMenu,
  children,
  ctx,
}: SchemaRootRowProps) {
  const isExpanded = nodeExpanded(ctx, nodeKey);
  return (
    <>
      <div
        role={hasChildren ? 'button' : undefined}
        tabIndex={hasChildren ? 0 : undefined}
        className="group flex h-[24px] min-w-0 cursor-default items-center gap-1 px-2 font-semibold text-slate-900 hover:bg-slate-100"
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
        <span className="min-w-0 truncate">{label}</span>
        {hasChildren ? (
          <ChevronDown
            className={`flex-none text-slate-400 opacity-0 transition-transform group-hover:opacity-100 ${
              isExpanded ? '' : '-rotate-90'
            }`}
            size={CARET_PX}
          />
        ) : null}
      </div>
      {isExpanded ? children : null}
    </>
  );
}

interface TreeRowProps {
  ctx: TreeContext;
  nodeKey: string;
  label: ReactNode;
  depth: number;
  icon?: ReactNode;
  count?: number;
  rightDetail?: ReactNode;
  hasChildren?: boolean;
  onContextMenu?: (e: MouseEvent<HTMLDivElement>) => void;
  children?: ReactNode;
}

// TreeRow renders any non-leaf, non-root node: a group header (Tables,
// Columns, Indexes…) or an expandable item (a table). It always reserves a
// caret slot on the left so siblings stay aligned regardless of icon width.
function TreeRow({
  ctx,
  nodeKey,
  label,
  depth,
  icon,
  count,
  rightDetail,
  hasChildren,
  onContextMenu,
  children,
}: TreeRowProps) {
  const isExpanded = nodeExpanded(ctx, nodeKey);
  const onClick = hasChildren ? () => ctx.toggle(nodeKey) : undefined;
  return (
    <>
      <RowShell
        depth={depth}
        onClick={onClick}
        onContextMenu={onContextMenu}
        clickable={!!hasChildren}
      >
        <CaretSlot expanded={isExpanded} hasChildren={!!hasChildren} />
        {icon ? <span className="flex-none text-slate-500">{icon}</span> : null}
        <span className="min-w-0 truncate text-slate-700">{label}</span>
        {typeof count === 'number' ? <Pill>{count}</Pill> : null}
        {rightDetail ? <RightDetail>{rightDetail}</RightDetail> : null}
      </RowShell>
      {isExpanded ? children : null}
    </>
  );
}

interface LeafRowProps {
  ctx: TreeContext;
  label: ReactNode;
  depth: number;
  rightDetail?: ReactNode;
  onContextMenu?: (e: MouseEvent<HTMLDivElement>) => void;
}

// LeafRow is for terminal nodes (column, index, trigger, routine, enum).
// popsql does not reserve an empty caret slot for leaves; the leaf label
// starts where a sibling expandable row's caret would start.
function LeafRow({
  ctx: _ctx,
  label,
  depth,
  rightDetail,
  onContextMenu,
}: LeafRowProps) {
  return (
    <RowShell depth={depth} onContextMenu={onContextMenu} clickable={false}>
      <span className="min-w-0 truncate text-slate-700">{label}</span>
      {rightDetail ? <RightDetail>{rightDetail}</RightDetail> : null}
    </RowShell>
  );
}

interface RowShellProps {
  depth: number;
  clickable: boolean;
  onClick?: () => void;
  onContextMenu?: (e: MouseEvent<HTMLDivElement>) => void;
  children: ReactNode;
}

// RowShell lays out a single tree row: N IndentGuide spans, then the row
// content (caret + icon + label + pills). The indent guides carry the
// vertical connector lines on their right edge, popsql-style.
function RowShell({
  depth,
  clickable,
  onClick,
  onContextMenu,
  children,
}: RowShellProps) {
  return (
    <div
      role={clickable ? 'button' : undefined}
      tabIndex={clickable ? 0 : undefined}
      className="group/row flex h-[24px] min-w-0 cursor-default items-center gap-1.5 pl-2 pr-2 hover:bg-slate-100"
      onClick={onClick}
      onKeyDown={
        clickable && onClick
          ? (e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                onClick();
              }
            }
          : undefined
      }
      onContextMenu={onContextMenu}
    >
      <IndentGuides depth={depth} />
      {children}
    </div>
  );
}

function IndentGuides({ depth }: { depth: number }) {
  const guideCount = Math.max(0, depth - 1);
  if (guideCount === 0) return null;
  const guides = [];
  for (let i = 0; i < guideCount; i++) {
    guides.push(
      <span
        key={i}
        aria-hidden
        className="h-[24px] flex-none self-stretch border-r border-slate-200"
        style={{ width: INDENT_PAD, marginRight: INDENT_GAP }}
      />,
    );
  }
  return <>{guides}</>;
}

// CaretSlot reserves a fixed-width column for the chevron so that the row's
// icon/name column starts at the same x-position regardless of whether the
// row has children. The chevron itself rotates -90deg when collapsed.
function CaretSlot({
  expanded,
  hasChildren,
}: {
  expanded: boolean;
  hasChildren: boolean;
}) {
  return (
    <span
      className="flex flex-none items-center justify-center text-slate-400"
      style={{ width: CARET_PX }}
    >
      {hasChildren ? (
        <ChevronDown
          className={`transition-transform ${expanded ? '' : '-rotate-90'}`}
          size={CARET_PX}
        />
      ) : null}
    </span>
  );
}

// RightDetail wraps trailing pills/badges. It uses `ml-auto` to push itself
// to the right edge of the row when there is free space, and a very high
// `flex-shrink` so that, when horizontal space is tight, the detail column
// shrinks (its trailing content clipping off-screen) far before the row's
// name truncates.
function RightDetail({ children }: { children: ReactNode }) {
  return (
    <span
      className="ml-auto flex min-w-0 items-center justify-end gap-1 overflow-hidden pl-2"
      style={{ flexShrink: 9999 }}
    >
      {children}
    </span>
  );
}

function Pill({ children }: { children: ReactNode }) {
  return (
    <span className="inline-flex flex-none items-center whitespace-nowrap rounded bg-[rgba(0,0,0,0.1)] px-1 py-px text-[11px] leading-tight text-slate-600">
      {children}
    </span>
  );
}

function HypertableBadge({
  info,
}: {
  info: { num_chunks: number; compression_enabled: boolean };
}) {
  return (
    <span className="inline-flex items-center whitespace-nowrap rounded bg-purple-100 px-1 py-px text-[11px] leading-tight text-purple-700">
      hypertable · {info.num_chunks}c{info.compression_enabled ? ' · zip' : ''}
    </span>
  );
}

// ---- Helpers ---------------------------------------------------------------

function schemaKey(ns: { name: string }): string {
  return `schema:${ns.name}`;
}

function childKey(ns: string, group: GroupKind, name: string): string {
  return `${schemaKey({ name: ns })}/${group}/${name}`;
}

// columnConstraintLabel picks the single most informative constraint label
// for a column, in priority order: primary key > unique > not null.
// Mirrors popsql's `constraintForColumn`.
function columnConstraintLabel(
  parent: TableSchema | ViewSchema,
  col: TableColumn | { name: string },
): string | null {
  const t = parent as TableSchema;
  const constraints = t.constraints ?? [];
  if (
    constraints.some(
      (c) => c.type === 'PRIMARY KEY' && (c.columns ?? []).includes(col.name),
    )
  ) {
    return 'primary key';
  }
  if (
    constraints.some(
      (c) => c.type === 'UNIQUE' && (c.columns ?? []).includes(col.name),
    )
  ) {
    return 'unique';
  }
  if ((col as TableColumn).not_null) {
    return 'not null';
  }
  return null;
}

// iconLabel wraps a text label with a leading icon, matching popsql's
// context-menu icon+label layout.
function iconLabel(
  Icon: (props: IconProps) => JSX.Element,
  text: string,
): ReactNode {
  return (
    <>
      <Icon className="flex-none text-slate-500" size={14} />
      <span>{text}</span>
    </>
  );
}

function highlight(text: string, term: string): ReactNode {
  if (!term) return text;
  const idx = text.toLowerCase().indexOf(term.toLowerCase());
  if (idx < 0) return text;
  const before = text.slice(0, idx);
  const match = text.slice(idx, idx + term.length);
  const after = text.slice(idx + term.length);
  return (
    <>
      {before}
      <mark className="rounded bg-yellow-200 px-0.5">{match}</mark>
      {after}
    </>
  );
}

interface SearchInfo {
  visible: Set<string>;
}

function computeSearch(schemas: NamespacedSchema[], term: string): SearchInfo {
  const visible = new Set<string>();
  if (!term) return { visible };
  const lower = term.toLowerCase();
  const match = (s: string) => s.toLowerCase().includes(lower);

  for (const ns of schemas) {
    const sKey = schemaKey(ns);
    let anyHit = match(ns.name);

    const considerGroup = (
      kind: GroupKind,
      items:
        | {
            name: string;
            columns?: { name: string }[];
            indexes?: { name: string }[];
            triggers?: { name: string }[];
          }[]
        | undefined,
    ): boolean => {
      const list = items ?? [];
      if (list.length === 0) return false;
      const gKey = `${sKey}/${kind}`;
      let groupHit = false;
      for (const item of list) {
        const iKey = childKey(ns.name, kind, item.name);
        const itemHit = match(item.name);
        let childHit = false;
        const considerSub = (subKind: string, subs?: { name: string }[]) => {
          if (!subs) return;
          for (const sub of subs) {
            if (match(sub.name)) {
              visible.add(`${iKey}/${subKind}/${sub.name}`);
              visible.add(`${iKey}/${subKind}`);
              childHit = true;
            }
          }
        };
        considerSub('columns', item.columns);
        considerSub('indexes', item.indexes);
        considerSub('triggers', item.triggers);
        if (itemHit || childHit) {
          visible.add(iKey);
          groupHit = true;
        }
      }
      if (groupHit) {
        visible.add(gKey);
      }
      return groupHit;
    };

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
  return { visible };
}

// ---- Context menu ---------------------------------------------------------

interface ContextMenuState {
  x: number;
  y: number;
  items: MenuItem[];
}

interface MenuItem {
  key: string;
  label: ReactNode;
  onClick: () => void;
}

interface ContextMenuProps {
  state: ContextMenuState;
  onClose: () => void;
}

function ContextMenu({ state, onClose }: ContextMenuProps) {
  const ref = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    const onDown = (e: globalThis.MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    // Defer attaching the outside-click listener by one tick so the same
    // mousedown that opened this menu doesn't immediately close it.
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
      className="fixed z-50 min-w-[200px] rounded border border-slate-200 bg-white py-1 text-sm shadow-lg"
      style={{ top: state.y, left: state.x }}
    >
      {state.items.map((item) => (
        <button
          key={item.key}
          type="button"
          role="menuitem"
          onClick={() => {
            item.onClick();
            onClose();
          }}
          className="flex w-full items-center gap-2 px-3 py-1 text-left hover:bg-blue-50"
        >
          {item.label}
        </button>
      ))}
    </div>
  );
}

// ---- Menu actions ---------------------------------------------------------

function copyText(text: string) {
  void navigator.clipboard.writeText(text);
}

// ---- Definition modal -----------------------------------------------------

interface DefinitionModalProps {
  title: string;
  text: string;
  onClose: () => void;
}

function DefinitionModal({ title, text, onClose }: DefinitionModalProps) {
  const ref = useRef<HTMLDivElement | null>(null);
  const downTarget = useRef<EventTarget | null>(null);

  // Close on Escape.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  // Only close on click-outside when the mousedown also originated on the
  // backdrop, so dragging to select text inside the modal doesn't dismiss it.
  const onClickOutside = (e: MouseEvent<HTMLDivElement>) => {
    if (e.target === ref.current && downTarget.current === ref.current) {
      onClose();
    }
  };

  return (
    <div
      ref={ref}
      onClick={onClickOutside}
      onMouseDown={(e) => {
        downTarget.current = e.target;
      }}
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/30"
      onKeyDown={(e) => {
        if (e.key === 'Escape') onClose();
      }}
    >
      <div className="flex max-h-[80vh] min-w-[360px] max-w-[min(600px,90vw)] flex-col rounded-lg border border-slate-200 bg-white shadow-xl">
        <div className="flex items-center justify-between border-b border-slate-200 px-4 py-2">
          <span className="text-sm font-semibold text-slate-900">{title}</span>
          <button
            type="button"
            onClick={onClose}
            className="rounded p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-700"
            aria-label="Close"
          >
            ✕
          </button>
        </div>
        <pre className="min-h-0 flex-1 overflow-auto whitespace-pre-wrap break-all px-4 py-3 text-[13px] leading-relaxed text-slate-800">
          {text}
        </pre>
        <div className="flex justify-end border-t border-slate-200 px-4 py-2">
          <button
            type="button"
            onClick={() => copyText(text)}
            className="rounded border border-slate-300 bg-white px-3 py-1 text-sm text-slate-700 hover:bg-slate-50"
          >
            Copy to clipboard
          </button>
        </div>
      </div>
    </div>
  );
}

function schemaMenuItems(name: string): MenuItem[] {
  const append = useServeStore.getState().appendEditorSql;
  return [
    {
      key: 'new-query',
      label: iconLabel(
        NavQueriesPlus,
        `New query: SET search_path TO ${quoteIdent(name)}`,
      ),
      onClick: () => append(`SET search_path TO ${quoteIdent(name)};`),
    },
    {
      key: 'copy-name',
      label: iconLabel(CopyIcon, 'Copy schema name'),
      onClick: () => copyText(quoteIdent(name)),
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
  const sql = selectAllSql(ns, table.name, cols);
  return [
    {
      key: 'new-query',
      label: iconLabel(NavQueriesPlus, `New query from ${kind}`),
      onClick: () => append(sql),
    },
    {
      key: 'copy-select',
      label: iconLabel(CopyIcon, 'Copy SELECT statement'),
      onClick: () => copyText(sql),
    },
    {
      key: 'copy-name',
      label: iconLabel(CopyIcon, `Copy ${kind} name`),
      onClick: () => copyText(qualifiedName(ns, table.name)),
    },
  ];
}

function columnMenuItems(ns: string, table: string, col: string): MenuItem[] {
  const append = useServeStore.getState().appendEditorSql;
  const sql = `SELECT ${quoteIdent(col)} FROM ${qualifiedName(ns, table)} LIMIT 100;`;
  return [
    {
      key: 'new-query',
      label: iconLabel(NavQueriesPlus, 'New query with column'),
      onClick: () => append(sql),
    },
    {
      key: 'copy-select',
      label: iconLabel(CopyIcon, 'Copy SELECT statement'),
      onClick: () => copyText(sql),
    },
    {
      key: 'copy-name',
      label: iconLabel(CopyIcon, 'Copy column name'),
      onClick: () => copyText(quoteIdent(col)),
    },
    {
      key: 'copy-qualified',
      label: iconLabel(CopyIcon, 'Copy qualified column name'),
      onClick: () => copyText(`${qualifiedName(ns, table)}.${quoteIdent(col)}`),
    },
  ];
}

function indexMenuItems(
  ns: string,
  index: IndexSchema,
  showModal: (title: string, text: string) => void,
): MenuItem[] {
  const items: MenuItem[] = [];
  if (index.definition) {
    items.push(
      {
        key: 'view-def',
        label: iconLabel(EyeIcon, 'View definition'),
        onClick: () => showModal(index.name, index.definition),
      },
      {
        key: 'copy-def',
        label: iconLabel(CopyIcon, 'Copy definition'),
        onClick: () => copyText(index.definition),
      },
    );
  }
  items.push({
    key: 'copy-name',
    label: iconLabel(CopyIcon, 'Copy qualified name'),
    onClick: () => copyText(qualifiedName(ns, index.name)),
  });
  return items;
}

function routineMenuItems(ns: string, name: string): MenuItem[] {
  return [
    {
      key: 'copy-name',
      label: iconLabel(CopyIcon, 'Copy qualified name'),
      onClick: () => copyText(qualifiedName(ns, name)),
    },
  ];
}
