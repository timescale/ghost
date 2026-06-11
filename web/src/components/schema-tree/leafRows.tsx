import type {
  EnumSchema,
  IndexSchema,
  PartitionInfo,
  Routine,
  TableColumn,
  TableSchema,
  TriggerSchema,
  ViewColumn,
  ViewSchema,
} from '../../schema';
import { routineSignature } from '../../schema';
import { highlight } from '../../util/highlight';
import {
  type ConstraintItem,
  columnConstraintLabel,
  columnForeignKey,
} from './constraints';
import {
  columnMenuItems,
  copyQualifiedNameItem,
  enumMenuItems,
  indexMenuItems,
  partitionMenuItems,
  routineMenuItems,
  triggerMenuItems,
} from './menus';
import { CommentBadge, LeafRow, Pill } from './rows';
import { contextMenuHandler, type TreeContext } from './TreeContext';

interface ColumnRowProps {
  ctx: TreeContext;
  parent: TableSchema | ViewSchema;
  ns: string;
  parentName: string;
  col: TableColumn | ViewColumn;
  // Columns nested under a "Columns" group sit at depth 4 (the default).
  // Regular-view columns render directly under the view, so they sit one
  // level shallower.
  depth?: number;
}

export function ColumnRow({
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
      label={highlight(col.name, ctx.searchTerm)}
      depth={depth}
      rightDetail={
        <>
          {constraint ? <Pill>{constraint}</Pill> : null}
          {foreignKey ? <Pill>{`\u2192 ${foreignKey}`}</Pill> : null}
          <Pill>{col.type}</Pill>
          <CommentBadge
            title={col.name}
            comment={col.comment}
            showModal={ctx.showModal}
          />
        </>
      }
      onContextMenu={contextMenuHandler(ctx, () =>
        columnMenuItems(ns, parentName, col, ctx.showModal),
      )}
    />
  );
}

interface IndexRowProps {
  ctx: TreeContext;
  ns: string;
  index: IndexSchema;
}

export function IndexRow({ ns, index, ctx }: IndexRowProps) {
  return (
    <LeafRow
      label={highlight(index.name, ctx.searchTerm)}
      depth={4}
      rightDetail={
        <>
          {index.is_unique ? <Pill>unique</Pill> : null}
          <Pill>{index.columns}</Pill>
        </>
      }
      onContextMenu={contextMenuHandler(ctx, () =>
        indexMenuItems(ns, index, ctx.showModal),
      )}
    />
  );
}

interface ConstraintRowProps {
  ctx: TreeContext;
  ns: string;
  item: ConstraintItem;
}

export function ConstraintRow({ ns, item, ctx }: ConstraintRowProps) {
  return (
    <LeafRow
      label={highlight(item.name, ctx.searchTerm)}
      depth={4}
      rightDetail={
        <>
          <Pill>{item.kindWord}</Pill>
          <Pill>{item.detail}</Pill>
        </>
      }
      onContextMenu={contextMenuHandler(ctx, () => [
        copyQualifiedNameItem(ns, item.name),
      ])}
    />
  );
}

interface TriggerRowProps {
  ctx: TreeContext;
  ns: string;
  trigger: TriggerSchema;
}

export function TriggerRow({ ns, trigger, ctx }: TriggerRowProps) {
  return (
    <LeafRow
      label={highlight(trigger.name, ctx.searchTerm)}
      depth={4}
      rightDetail={
        <>
          <Pill>{trigger.timing.toLowerCase()}</Pill>
          <Pill>{trigger.manipulation.toLowerCase()}</Pill>
        </>
      }
      onContextMenu={contextMenuHandler(ctx, () =>
        triggerMenuItems(ns, trigger, ctx.showModal),
      )}
    />
  );
}

interface PartitionRowProps {
  ctx: TreeContext;
  ns: string;
  partition: PartitionInfo;
}

export function PartitionRow({ ns, partition, ctx }: PartitionRowProps) {
  return (
    <LeafRow
      label={highlight(partition.name, ctx.searchTerm)}
      depth={4}
      rightDetail={partition.bound ? <Pill>{partition.bound}</Pill> : null}
      onContextMenu={contextMenuHandler(ctx, () =>
        partitionMenuItems(ns, partition, ctx.showModal),
      )}
    />
  );
}

interface RoutineRowProps {
  ctx: TreeContext;
  ns: string;
  routine: Routine;
}

export function RoutineRow({ ns, routine, ctx }: RoutineRowProps) {
  return (
    <LeafRow
      label={highlight(routineSignature(routine), ctx.searchTerm)}
      depth={2}
      rightDetail={
        <CommentBadge
          title={routineSignature(routine)}
          comment={routine.comment}
          showModal={ctx.showModal}
        />
      }
      onContextMenu={contextMenuHandler(ctx, () =>
        routineMenuItems(ns, routine, ctx.showModal),
      )}
    />
  );
}

interface EnumRowProps {
  ctx: TreeContext;
  ns: string;
  enum_: EnumSchema;
}

export function EnumRow({ ns, enum_, ctx }: EnumRowProps) {
  return (
    <LeafRow
      label={highlight(enum_.name, ctx.searchTerm)}
      depth={2}
      rightDetail={
        <>
          <Pill>{(enum_.values ?? []).join(', ')}</Pill>
          <CommentBadge
            title={enum_.name}
            comment={enum_.comment}
            showModal={ctx.showModal}
          />
        </>
      }
      onContextMenu={contextMenuHandler(ctx, () =>
        enumMenuItems(ns, enum_, ctx.showModal),
      )}
    />
  );
}
