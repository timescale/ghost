import {
  type MouseEvent,
  type ReactNode,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from 'react';

import {
  type IndexSchema,
  type NamespacedSchema,
  type PartitionInfo,
  qualifiedName,
  quoteIdent,
  type Routine,
  routineSignature,
  selectAllSql,
  type TableColumn,
  type TableConstraint,
  type TableSchema,
  type TriggerSchema,
  type ViewSchema,
} from '../schema';
import { useServeStore } from '../store';
import {
  ContextMenu,
  type ContextMenuState,
  type MenuItem,
} from './ContextMenu';
import { Icon, type IconName } from './Icon';
import { Modal } from './Modal';
import { SqlCodeView } from './SqlCodeView';

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

export function SchemaTree({
  databaseId,
  schemas,
  searchTerm,
}: SchemaTreeProps) {
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
        searchActive && !search.visible.has(schemaKey(ns)) ? null : (
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
      return <Icon name="table" size={14} />;
    case 'functions':
    case 'procedures':
      return <Icon name="function" size={14} />;
    case 'enums':
      return null;
  }
}

interface GroupNodeProps extends NodeProps {
  ns: string;
  group: GroupSpec;
}

function GroupNode({ ns, group, ctx }: GroupNodeProps) {
  const key = `${schemaKey({ name: ns })}/${group.kind}`;
  const items = group.items;
  const visibleItems = ctx.searchActive
    ? items.filter((item) =>
        ctx.searchMatches?.has(
          childKey(ns, group.kind, itemLabel(group.kind, item)),
        ),
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
  const itemKey = childKey(ns, kind, itemLabel(kind, item));
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
  const allPartitions = table.partitions ?? [];
  const allConstraints = tableConstraintItems(table);
  const allIndexes = table.indexes ?? [];
  const allTriggers = table.triggers ?? [];
  // When a search is active, only render the children that themselves match.
  // popsql does the same: searching for "plan" inside a multi-column table
  // collapses Columns down to just the matching ones.
  const cols = filterForSearch(ctx, key, 'columns', allCols);
  const partitions = filterForSearch(
    ctx,
    key,
    'partitions',
    allPartitions,
    partitionNodeName,
  );
  const constraints = filterForSearch(ctx, key, 'constraints', allConstraints);
  const indexes = filterForSearch(ctx, key, 'indexes', allIndexes);
  const triggers = filterForSearch(ctx, key, 'triggers', allTriggers);
  return (
    <TreeRow
      ctx={ctx}
      nodeKey={key}
      label={highlight(table.name, ctx.searchTerm)}
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
      {partitions.length > 0 ? (
        <TreeRow
          ctx={ctx}
          nodeKey={`${key}/partitions`}
          label="Partitions"
          depth={3}
          hasChildren
          count={ctx.searchActive ? partitions.length : allPartitions.length}
        >
          {partitions.map((part) => (
            <PartitionRow
              key={partitionNodeName(part)}
              ns={ns}
              partition={part}
              ctx={ctx}
            />
          ))}
        </TreeRow>
      ) : null}
      {constraints.length > 0 ? (
        <TreeRow
          ctx={ctx}
          nodeKey={`${key}/constraints`}
          label="Constraints"
          depth={3}
          hasChildren
          count={ctx.searchActive ? constraints.length : allConstraints.length}
        >
          {constraints.map((c) => (
            <ConstraintRow key={c.name} ns={ns} item={c} ctx={ctx} />
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
              ns={ns}
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
  // Columns nested under a "Columns" group sit at depth 4 (the default).
  // Regular-view columns render directly under the view, so they sit one
  // level shallower.
  depth?: number;
}

function ColumnRow({
  parent,
  ns,
  parentName,
  col,
  ctx,
  depth = 4,
}: ColumnRowProps) {
  const constraint = columnConstraintLabel(parent, col);
  const foreignKey = columnForeignKey(parent, col);
  return (
    <LeafRow
      ctx={ctx}
      label={highlight(col.name, ctx.searchTerm)}
      depth={depth}
      rightDetail={
        <>
          {constraint ? <Pill>{constraint}</Pill> : null}
          {foreignKey ? <Pill>{`\u2192 ${foreignKey}`}</Pill> : null}
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

interface ConstraintRowProps extends NodeProps {
  ns: string;
  item: ConstraintItem;
}

function ConstraintRow({ ns, item, ctx }: ConstraintRowProps) {
  return (
    <LeafRow
      ctx={ctx}
      label={highlight(item.name, ctx.searchTerm)}
      depth={4}
      rightDetail={
        <>
          <Pill>{item.kindWord}</Pill>
          <Pill>{item.detail}</Pill>
        </>
      }
      onContextMenu={(e) => {
        e.preventDefault();
        ctx.setContextMenu({
          x: e.clientX,
          y: e.clientY,
          items: [copyQualifiedNameItem(ns, item.name)],
        });
      }}
    />
  );
}

interface TriggerRowProps extends NodeProps {
  ns: string;
  trigger: TriggerSchema;
}

function TriggerRow({ ns, trigger, ctx }: TriggerRowProps) {
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
      onContextMenu={(e) => {
        e.preventDefault();
        ctx.setContextMenu({
          x: e.clientX,
          y: e.clientY,
          items: triggerMenuItems(ns, trigger, ctx.showModal),
        });
      }}
    />
  );
}

interface PartitionRowProps extends NodeProps {
  ns: string;
  partition: PartitionInfo;
}

function PartitionRow({ ns, partition, ctx }: PartitionRowProps) {
  return (
    <LeafRow
      ctx={ctx}
      label={highlight(partition.name, ctx.searchTerm)}
      depth={4}
      rightDetail={partition.bound ? <Pill>{partition.bound}</Pill> : null}
      onContextMenu={(e) => {
        e.preventDefault();
        ctx.setContextMenu({
          x: e.clientX,
          y: e.clientY,
          items: partitionMenuItems(ns, partition, ctx.showModal),
        });
      }}
    />
  );
}

interface ViewNodeProps extends NodeProps {
  ns: string;
  view: ViewSchema;
  kind: 'view' | 'matview';
}

function ViewNode({ ns, view, kind, ctx }: ViewNodeProps) {
  const isMatView = kind !== 'view';
  const groupKind = isMatView ? 'matViews' : 'views';
  const key = childKey(ns, groupKind, view.name);
  const allCols = view.columns ?? [];
  const allIndexes = view.indexes ?? [];
  const allTriggers = view.triggers ?? [];
  // When a search is active, only render the children that themselves match
  // (same behavior as TableNode).
  const cols = filterForSearch(ctx, key, 'columns', allCols);
  const indexes = filterForSearch(ctx, key, 'indexes', allIndexes);
  const triggers = filterForSearch(ctx, key, 'triggers', allTriggers);
  return (
    <TreeRow
      ctx={ctx}
      nodeKey={key}
      label={highlight(view.name, ctx.searchTerm)}
      depth={2}
      hasChildren
      onContextMenu={(e) => {
        e.preventDefault();
        ctx.setContextMenu({
          x: e.clientX,
          y: e.clientY,
          items: viewMenuItems(
            ns,
            view,
            kind === 'view' ? 'view' : 'materialized view',
            ctx.showModal,
          ),
        });
      }}
    >
      {/*
        Regular views can only have columns, so we render them directly
        under the view (no "Columns" group). Materialized views behave more
        like tables — they also carry indexes — so we group their columns
        to keep the two sibling lists from floating at the same level.
      */}
      {isMatView ? (
        cols.length > 0 ? (
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
                parent={view}
                ns={ns}
                parentName={view.name}
                col={col}
                ctx={ctx}
              />
            ))}
          </TreeRow>
        ) : null
      ) : (
        cols.map((col) => (
          <ColumnRow
            key={col.name}
            parent={view}
            ns={ns}
            parentName={view.name}
            col={col}
            ctx={ctx}
            depth={3}
          />
        ))
      )}
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
              ns={ns}
              trigger={trg}
              ctx={ctx}
            />
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
      label={highlight(routineSignature(routine), ctx.searchTerm)}
      depth={2}
      onContextMenu={(e) => {
        e.preventDefault();
        ctx.setContextMenu({
          x: e.clientX,
          y: e.clientY,
          items: routineMenuItems(ns, routine, ctx.showModal),
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
          items: [copyQualifiedNameItem(ns, enum_.name)],
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
          <Icon
            name="chevron-down"
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
        <Icon
          name="chevron-down"
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

// Node keys are built by joining segments with '/'. PostgreSQL identifiers
// (schema/table/column names, routine signatures) can themselves contain '/',
// so the variable segments are escaped before joining. Without this, e.g. a
// table named `a/columns` would collide with the "columns" subgroup key of a
// table named `a`. Backslash is the escape character.
function encodeKeySegment(segment: string): string {
  return segment.replace(/\\/g, '\\\\').replace(/\//g, '\\/');
}

function schemaKey(ns: { name: string }): string {
  return `schema:${encodeKeySegment(ns.name)}`;
}

function childKey(ns: string, group: GroupKind, name: string): string {
  return `${schemaKey({ name: ns })}/${group}/${encodeKeySegment(name)}`;
}

// subItemKey builds the key for a leaf node nested under a group item (e.g. a
// column or partition under a table). The sub-item name is escaped for the
// same reason as childKey's name segment.
function subItemKey(itemKey: string, subKind: string, name: string): string {
  return `${itemKey}/${subKind}/${encodeKeySegment(name)}`;
}

// filterForSearch narrows a group item's sub-items (columns, indexes, etc.) to
// those that matched the active search, mirroring popsql: searching collapses
// a group down to just the matching rows. When no search is active it returns
// the list unchanged. The key derivation must stay in lockstep with
// computeSearch's, so getName defaults to the bare `name` but can be
// overridden (e.g. partitionNodeName, which keys cross-schema partitions
// uniquely).
function filterForSearch<T extends { name: string }>(
  ctx: TreeContext,
  itemKey: string,
  subKind: string,
  items: T[],
  getName: (item: T) => string = (item) => item.name,
): T[] {
  if (!ctx.searchActive) return items;
  return items.filter((item) =>
    ctx.searchMatches?.has(subItemKey(itemKey, subKind, getName(item))),
  );
}

// partitionNodeName returns the identifier used to key a partition within its
// parent's Partitions list. PostgreSQL allows a partition to live in a
// different schema than its parent, so two partitions of the same parent can
// share a name as long as their schemas differ. Qualifying such partitions
// with their schema keeps their React keys and search-visibility keys unique;
// same-schema partitions (the common case) keep their bare name.
function partitionNodeName(p: { name: string; schema?: string }): string {
  return p.schema ? `${p.schema}.${p.name}` : p.name;
}

// itemLabel returns the display/key label for a group item. Routines use
// their signature (name + identity arguments) so overloaded routines that
// share a name remain distinct in both the rendered tree and the keys/state
// derived from them.
function itemLabel(
  kind: GroupKind,
  item: TableSchema | ViewSchema | Routine | { name: string },
): string {
  if (kind === 'functions' || kind === 'procedures') {
    return routineSignature(item as Routine);
  }
  return item.name;
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
  // Only single-column PK/UNIQUE constraints are conveyed inline on the
  // column. Composite (multi-column) constraints would be misleading as a
  // per-column pill (e.g. UNIQUE (a, b) does not make `a` unique on its
  // own), so those are surfaced under the table's Constraints group
  // instead (see tableConstraintItems).
  const hasSingleColumn = (type: TableConstraint['type']) =>
    constraints.some(
      (c) =>
        c.type === type &&
        (c.columns ?? []).length === 1 &&
        c.columns?.[0] === col.name,
    );
  if (hasSingleColumn('PRIMARY KEY')) {
    return 'primary key';
  }
  if (hasSingleColumn('UNIQUE')) {
    return 'unique';
  }
  if ((col as TableColumn).not_null) {
    return 'not null';
  }
  return null;
}

// columnForeignKey returns the referenced table for a single-column foreign
// key on the given column, or null. Surfaced inline as a hint pill on the
// column row; the full constraint (with referenced columns) is also listed
// under the table's Constraints group.
function columnForeignKey(
  parent: TableSchema | ViewSchema,
  col: { name: string },
): string | null {
  const constraints = (parent as TableSchema).constraints ?? [];
  for (const c of constraints) {
    const cols = c.columns ?? [];
    if (
      c.type === 'FOREIGN KEY' &&
      cols.length === 1 &&
      cols[0] === col.name &&
      c.ref_table
    ) {
      return c.ref_table;
    }
  }
  return null;
}

// A single row under the table's Constraints group.
interface ConstraintItem {
  name: string;
  kindWord: string;
  detail: string;
}

// tableConstraintItems flattens the constraints a table carries that aren't
// already conveyed by the per-column pills: composite primary-key/unique
// constraints, foreign keys (with their full referenced table/columns),
// check constraints, and exclusion constraints. Single-column PK/UNIQUE
// membership is omitted here because it's shown inline on each member
// column (see columnConstraintLabel).
function tableConstraintItems(table: TableSchema): ConstraintItem[] {
  const items: ConstraintItem[] = [];
  for (const c of table.constraints ?? []) {
    const cols = c.columns ?? [];
    if (c.type === 'PRIMARY KEY' || c.type === 'UNIQUE') {
      // Single-column PK/UNIQUE are already shown inline on the column.
      if (cols.length <= 1) continue;
      items.push({
        name: c.name,
        kindWord: c.type === 'PRIMARY KEY' ? 'primary key' : 'unique',
        detail: `(${cols.join(', ')})`,
      });
      continue;
    }
    if (c.type !== 'FOREIGN KEY') continue;
    const colsList = cols.join(', ');
    const refCols = (c.ref_columns ?? []).join(', ');
    items.push({
      name: c.name,
      kindWord: 'foreign key',
      detail: `(${colsList}) \u2192 ${c.ref_table ?? '?'}(${refCols})`,
    });
  }
  for (const chk of table.checks ?? []) {
    items.push({ name: chk.name, kindWord: 'check', detail: chk.expression });
  }
  for (const exc of table.exclusions ?? []) {
    items.push({ name: exc.name, kindWord: 'exclude', detail: exc.definition });
  }
  return items;
}

// iconLabel wraps a text label with a leading icon, matching popsql's
// context-menu icon+label layout.
function iconLabel(name: IconName, text: string): ReactNode {
  return (
    <>
      <Icon name={name} className="flex-none text-slate-500" size={14} />
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
            constraints?: { name: string }[];
            checks?: { name: string }[];
            exclusions?: { name: string }[];
            indexes?: { name: string }[];
            triggers?: { name: string }[];
            partitions?: { name: string; schema?: string }[];
          }[]
        | undefined,
    ): boolean => {
      const list = items ?? [];
      if (list.length === 0) return false;
      let groupHit = false;
      for (const item of list) {
        const label = itemLabel(kind, item as never);
        const iKey = childKey(ns.name, kind, label);
        const itemHit = match(label);
        let childHit = false;
        const considerSub = (
          subKind: string,
          subs?: { name: string; schema?: string }[],
          // keyName derives the node-key segment from a sub-item; it can
          // differ from the searched name (e.g. partitions are keyed by their
          // schema-qualified name to stay unique, but matched on bare name).
          keyName: (sub: { name: string; schema?: string }) => string = (s) =>
            s.name,
        ) => {
          if (!subs) return;
          for (const sub of subs) {
            if (match(sub.name)) {
              visible.add(subItemKey(iKey, subKind, keyName(sub)));
              childHit = true;
            }
          }
        };
        considerSub('columns', item.columns);
        considerSub(
          'constraints',
          tableConstraintItems(item as unknown as TableSchema),
        );
        considerSub('indexes', item.indexes);
        considerSub('triggers', item.triggers);
        considerSub('partitions', item.partitions, partitionNodeName);
        if (itemHit || childHit) {
          visible.add(iKey);
          groupHit = true;
        }
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
  return (
    <Modal onClose={onClose} className="w-[min(960px,92vw)]">
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
      <div className="min-h-0 flex-1 overflow-auto p-2">
        <SqlCodeView query={text} />
      </div>
    </Modal>
  );
}

function schemaMenuItems(name: string): MenuItem[] {
  const append = useServeStore.getState().appendEditorSql;
  return [
    {
      key: 'new-query',
      label: iconLabel(
        'new-query',
        `New query: SET search_path TO ${quoteIdent(name)}`,
      ),
      onClick: () => append(`SET search_path TO ${quoteIdent(name)};`),
    },
    {
      key: 'copy-name',
      label: iconLabel('copy', 'Copy schema name'),
      onClick: () => copyText(quoteIdent(name)),
    },
  ];
}

// tableMenuItems builds the query/copy actions shared by tables and views. It
// only needs the relation's name and column names, so its parameter is
// narrowed to that shape (rather than the full TableSchema) — this lets
// viewMenuItems reuse it without an unsafe cast, and surfaces a compile error
// if a future edit reaches for a table-only field.
function tableMenuItems(
  ns: string,
  table: { name: string; columns?: { name: string }[] },
  kind: 'table' | 'view' | 'materialized view',
): MenuItem[] {
  const append = useServeStore.getState().appendEditorSql;
  const cols = table.columns ?? [];
  const sql = selectAllSql(ns, table.name, cols);
  return [
    {
      key: 'new-query',
      label: iconLabel('new-query', `New query from ${kind}`),
      onClick: () => append(sql),
    },
    {
      key: 'copy-select',
      label: iconLabel('copy', 'Copy SELECT statement'),
      onClick: () => copyText(sql),
    },
    {
      key: 'copy-name',
      label: iconLabel('copy', `Copy ${kind} name`),
      onClick: () => copyText(qualifiedName(ns, table.name)),
    },
  ];
}

function viewMenuItems(
  ns: string,
  view: ViewSchema,
  kind: 'view' | 'materialized view',
  showModal: (title: string, text: string) => void,
): MenuItem[] {
  const items: MenuItem[] = [];
  const { definition } = view;
  if (definition) {
    items.push(
      {
        key: 'view-def',
        label: iconLabel('eye', 'View definition'),
        onClick: () => showModal(view.name, definition),
      },
      {
        key: 'copy-def',
        label: iconLabel('copy', 'Copy definition'),
        onClick: () => copyText(definition),
      },
    );
  }
  // Reuse the table query/copy actions (SELECT *, copy name, etc.).
  items.push(...tableMenuItems(ns, view, kind));
  return items;
}

function columnMenuItems(ns: string, table: string, col: string): MenuItem[] {
  const append = useServeStore.getState().appendEditorSql;
  const sql = `SELECT ${quoteIdent(col)} FROM ${qualifiedName(ns, table)} LIMIT 100;`;
  return [
    {
      key: 'new-query',
      label: iconLabel('new-query', 'New query with column'),
      onClick: () => append(sql),
    },
    {
      key: 'copy-select',
      label: iconLabel('copy', 'Copy SELECT statement'),
      onClick: () => copyText(sql),
    },
    {
      key: 'copy-name',
      label: iconLabel('copy', 'Copy column name'),
      onClick: () => copyText(quoteIdent(col)),
    },
    {
      key: 'copy-qualified',
      label: iconLabel('copy', 'Copy qualified column name'),
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
  const { definition } = index;
  if (definition) {
    items.push(
      {
        key: 'view-def',
        label: iconLabel('eye', 'View definition'),
        onClick: () => showModal(index.name, definition),
      },
      {
        key: 'copy-def',
        label: iconLabel('copy', 'Copy definition'),
        onClick: () => copyText(definition),
      },
    );
  }
  items.push(copyQualifiedNameItem(ns, index.name));
  return items;
}

function triggerMenuItems(
  ns: string,
  trigger: TriggerSchema,
  showModal: (title: string, text: string) => void,
): MenuItem[] {
  const items: MenuItem[] = [];
  const { statement } = trigger;
  if (statement) {
    items.push(
      {
        key: 'view-statement',
        label: iconLabel('eye', 'View action statement'),
        onClick: () => showModal(trigger.name, statement),
      },
      {
        key: 'copy-statement',
        label: iconLabel('copy', 'Copy action statement'),
        onClick: () => copyText(statement),
      },
    );
  }
  items.push(copyQualifiedNameItem(ns, trigger.name));
  return items;
}

function partitionMenuItems(
  ns: string,
  partition: PartitionInfo,
  showModal: (title: string, text: string) => void,
): MenuItem[] {
  const items: MenuItem[] = [];
  const { bound } = partition;
  if (bound) {
    items.push(
      {
        key: 'view-bound',
        label: iconLabel('eye', 'View partition bound'),
        onClick: () => showModal(partition.name, bound),
      },
      {
        key: 'copy-bound',
        label: iconLabel('copy', 'Copy partition bound'),
        onClick: () => copyText(bound),
      },
    );
  }
  // PostgreSQL allows a partition to live in a different schema than its
  // parent table; when that happens the partition carries its own schema, so
  // qualify with that rather than the parent's schema.
  items.push(copyQualifiedNameItem(partition.schema ?? ns, partition.name));
  return items;
}

function copyQualifiedNameItem(ns: string, name: string): MenuItem {
  return {
    key: 'copy-name',
    label: iconLabel('copy', 'Copy qualified name'),
    onClick: () => copyText(qualifiedName(ns, name)),
  };
}

function routineMenuItems(
  ns: string,
  routine: Routine,
  showModal: (title: string, text: string) => void,
): MenuItem[] {
  const items: MenuItem[] = [];
  const { definition } = routine;
  if (definition) {
    items.push(
      {
        key: 'view-def',
        label: iconLabel('eye', 'View definition'),
        onClick: () => showModal(routineSignature(routine), definition),
      },
      {
        key: 'copy-def',
        label: iconLabel('copy', 'Copy definition'),
        onClick: () => copyText(definition),
      },
    );
  }
  items.push(copyQualifiedNameItem(ns, routine.name));
  return items;
}
