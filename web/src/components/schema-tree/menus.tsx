import type { ReactNode } from 'react';

import {
  type ContinuousAggregateInfo,
  continuousAggregateDetails,
  type ForeignTableInfo,
  foreignTableDetails,
  type HypertableInfo,
  hypertableDetails,
  type IndexSchema,
  type PartitionInfo,
  qualifiedName,
  quoteIdent,
  type Routine,
  routineSignature,
  selectAllSql,
  type TriggerSchema,
  type ViewSchema,
} from '../../schema';
import { useServeStore } from '../../store';
import { copyText } from '../../util/clipboard';
import type { MenuItem } from '../ContextMenu';
import { Icon, type IconName } from '../Icon';
import type { ShowModal } from './TreeContext';

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

export function copyQualifiedNameItem(ns: string, name: string): MenuItem {
  return {
    key: 'copy-name',
    label: iconLabel('copy', 'Copy qualified name'),
    onClick: () => copyText(qualifiedName(ns, name)),
  };
}

// commentMenuItems builds the View/Copy comment pair for an object's
// COMMENT ON text. Returns no items when the object has no comment, so
// callers can unconditionally splice the result into their menus.
export function commentMenuItems(
  title: string,
  comment: string | undefined,
  showModal: ShowModal,
): MenuItem[] {
  if (!comment) return [];
  return [
    {
      key: 'view-comment',
      label: iconLabel('eye', 'View comment'),
      // Comments are prose, not SQL — render them without syntax
      // highlighting.
      onClick: () => showModal(title, comment, 'text'),
    },
    {
      key: 'copy-comment',
      label: iconLabel('copy', 'Copy comment'),
      onClick: () => copyText(comment),
    },
  ];
}

export function schemaMenuItems(
  schema: { name: string; comment?: string },
  showModal: ShowModal,
): MenuItem[] {
  const append = useServeStore.getState().appendEditorSql;
  const { name } = schema;
  return [
    ...commentMenuItems(name, schema.comment, showModal),
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

// hypertableMenuItems builds the View/Copy pair for a hypertable's metadata
// (chunk count, compression). Returns no items when the table is not a
// hypertable, so callers can unconditionally splice the result into their
// menus (like commentMenuItems).
export function hypertableMenuItems(
  title: string,
  hypertable: HypertableInfo | undefined,
  showModal: ShowModal,
): MenuItem[] {
  if (!hypertable) return [];
  const details = hypertableDetails(hypertable);
  return [
    {
      key: 'view-hypertable',
      label: iconLabel('eye', 'View hypertable details'),
      // The details are key/value prose, not SQL — render without syntax
      // highlighting.
      onClick: () => showModal(title, details, 'text'),
    },
    {
      key: 'copy-hypertable',
      label: iconLabel('copy', 'Copy hypertable details'),
      onClick: () => copyText(details),
    },
  ];
}

// continuousAggregateMenuItems builds the View/Copy pair for a continuous
// aggregate's metadata (materialized-only, compression). Returns no items
// when the view is not a continuous aggregate, so callers can
// unconditionally splice the result into their menus (like
// commentMenuItems).
export function continuousAggregateMenuItems(
  title: string,
  cagg: ContinuousAggregateInfo | undefined,
  showModal: ShowModal,
): MenuItem[] {
  if (!cagg) return [];
  const details = continuousAggregateDetails(cagg);
  return [
    {
      key: 'view-cagg',
      label: iconLabel('eye', 'View continuous aggregate details'),
      // The details are key/value prose, not SQL — render without syntax
      // highlighting.
      onClick: () => showModal(title, details, 'text'),
    },
    {
      key: 'copy-cagg',
      label: iconLabel('copy', 'Copy continuous aggregate details'),
      onClick: () => copyText(details),
    },
  ];
}

// foreignTableMenuItems builds the View/Copy pair for a foreign table's FDW
// binding (server, wrapper, table-level options). Returns no items when the
// table is not foreign, so callers can unconditionally splice the result
// into their menus (like commentMenuItems).
export function foreignTableMenuItems(
  title: string,
  foreign: ForeignTableInfo | undefined,
  showModal: ShowModal,
): MenuItem[] {
  if (!foreign) return [];
  const details = foreignTableDetails(foreign);
  return [
    {
      key: 'view-fdw',
      label: iconLabel('eye', 'View FDW details'),
      // The details are key/value prose, not SQL — render without syntax
      // highlighting.
      onClick: () => showModal(title, details, 'text'),
    },
    {
      key: 'copy-fdw',
      label: iconLabel('copy', 'Copy FDW details'),
      onClick: () => copyText(details),
    },
  ];
}

// tableMenuItems builds the comment/query/copy actions shared by tables and
// views. It only needs the relation's name, comment, and column names, so its
// parameter is narrowed to that shape (rather than the full TableSchema) —
// this lets viewMenuItems reuse it without an unsafe cast, and surfaces a
// compile error if a future edit reaches for a table-only field.
export function tableMenuItems(
  ns: string,
  table: { name: string; comment?: string; columns?: { name: string }[] },
  kind: 'table' | 'view' | 'materialized view',
  showModal: ShowModal,
): MenuItem[] {
  const append = useServeStore.getState().appendEditorSql;
  const cols = table.columns ?? [];
  const sql = selectAllSql(ns, table.name, cols);
  return [
    ...commentMenuItems(table.name, table.comment, showModal),
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

export function viewMenuItems(
  ns: string,
  view: ViewSchema,
  kind: 'view' | 'materialized view',
  showModal: ShowModal,
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
  // Reuse the table comment/query/copy actions (View comment, SELECT *,
  // copy name, etc.).
  items.push(...tableMenuItems(ns, view, kind, showModal));
  return items;
}

export function columnMenuItems(
  ns: string,
  table: string,
  col: { name: string; comment?: string },
  showModal: ShowModal,
): MenuItem[] {
  const append = useServeStore.getState().appendEditorSql;
  const colName = col.name;
  const sql = `SELECT ${quoteIdent(colName)} FROM ${qualifiedName(ns, table)} LIMIT 100;`;
  return [
    ...commentMenuItems(colName, col.comment, showModal),
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
      onClick: () => copyText(quoteIdent(colName)),
    },
    {
      key: 'copy-qualified',
      label: iconLabel('copy', 'Copy qualified column name'),
      onClick: () =>
        copyText(`${qualifiedName(ns, table)}.${quoteIdent(colName)}`),
    },
  ];
}

export function enumMenuItems(
  ns: string,
  enum_: { name: string; comment?: string },
  showModal: ShowModal,
): MenuItem[] {
  return [
    ...commentMenuItems(enum_.name, enum_.comment, showModal),
    copyQualifiedNameItem(ns, enum_.name),
  ];
}

export function indexMenuItems(
  ns: string,
  index: IndexSchema,
  showModal: ShowModal,
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

export function triggerMenuItems(
  ns: string,
  trigger: TriggerSchema,
  showModal: ShowModal,
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

export function partitionMenuItems(
  ns: string,
  partition: PartitionInfo,
  showModal: ShowModal,
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

export function routineMenuItems(
  ns: string,
  routine: Routine,
  showModal: ShowModal,
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
  items.push(
    ...commentMenuItems(routineSignature(routine), routine.comment, showModal),
    copyQualifiedNameItem(ns, routine.name),
  );
  return items;
}
