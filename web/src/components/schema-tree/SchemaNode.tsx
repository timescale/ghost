import type { ReactNode } from 'react';

import type {
  EnumSchema,
  NamespacedSchema,
  Routine,
  TableSchema,
  ViewSchema,
} from '../../schema';
import { highlight } from '../../util/highlight';
import { Icon } from '../Icon';
import {
  childKey,
  type GroupItem,
  type GroupKind,
  itemLabel,
  schemaKey,
} from './keys';
import { EnumRow, RoutineRow } from './leafRows';
import { schemaMenuItems } from './menus';
import { CommentBadge, SchemaRootRow, TreeRow } from './rows';
import { TableNode } from './TableNode';
import { contextMenuHandler, type TreeContext } from './TreeContext';
import { ViewNode } from './ViewNode';

interface SchemaNodeProps {
  ctx: TreeContext;
  ns: NamespacedSchema;
}

export function SchemaNode({ ns, ctx }: SchemaNodeProps) {
  const key = schemaKey(ns);
  const groups = schemaGroups(ns);

  return (
    <SchemaRootRow
      ctx={ctx}
      nodeKey={key}
      label={highlight(ns.name, ctx.searchTerm)}
      hasChildren
      rightDetail={
        <CommentBadge
          title={ns.name}
          comment={ns.comment}
          showModal={ctx.showModal}
        />
      }
      onContextMenu={contextMenuHandler(ctx, () =>
        schemaMenuItems(ns, ctx.showModal),
      )}
    >
      {groups.map((g) => (
        <GroupNode key={g.kind} ns={ns.name} group={g} ctx={ctx} />
      ))}
    </SchemaRootRow>
  );
}

interface GroupSpec {
  kind: GroupKind;
  label: string;
  items: GroupItem[];
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

interface GroupNodeProps {
  ctx: TreeContext;
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
  item: GroupItem,
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
        <RoutineRow key={itemKey} ns={ns} routine={item as Routine} ctx={ctx} />
      );
    case 'enums':
      return (
        <EnumRow key={itemKey} ns={ns} enum_={item as EnumSchema} ctx={ctx} />
      );
  }
}
