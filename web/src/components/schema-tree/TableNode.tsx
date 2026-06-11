import {
  type ForeignTableInfo,
  foreignTableDetails,
  type HypertableInfo,
  hypertableDetails,
  type TableSchema,
} from '../../schema';
import { highlight } from '../../util/highlight';
import { tableConstraintItems } from './constraints';
import { childKey, partitionNodeName } from './keys';
import {
  ColumnRow,
  ConstraintRow,
  IndexRow,
  PartitionRow,
  TriggerRow,
} from './leafRows';
import {
  foreignTableMenuItems,
  hypertableMenuItems,
  tableMenuItems,
} from './menus';
import { CommentBadge, TreeRow } from './rows';
import { SubItemGroup } from './SubItemGroup';
import { contextMenuHandler, type TreeContext } from './TreeContext';

interface TableNodeProps {
  ctx: TreeContext;
  ns: string;
  table: TableSchema;
}

export function TableNode({ ns, table, ctx }: TableNodeProps) {
  const key = childKey(ns, 'tables', table.name);
  return (
    <TreeRow
      ctx={ctx}
      nodeKey={key}
      label={highlight(table.name, ctx.searchTerm)}
      depth={2}
      hasChildren
      rightDetail={
        <>
          {table.hypertable ? (
            <HypertableBadge info={table.hypertable} />
          ) : null}
          {table.foreign ? <ForeignTableBadge info={table.foreign} /> : null}
          <CommentBadge
            title={table.name}
            comment={table.comment}
            showModal={ctx.showModal}
          />
        </>
      }
      onContextMenu={contextMenuHandler(ctx, () => [
        ...hypertableMenuItems(table.name, table.hypertable, ctx.showModal),
        ...foreignTableMenuItems(table.name, table.foreign, ctx.showModal),
        ...tableMenuItems(ns, table, 'table', ctx.showModal),
      ])}
    >
      <SubItemGroup
        ctx={ctx}
        parentKey={key}
        subKind="columns"
        label="Columns"
        items={table.columns ?? []}
        renderItem={(col) => (
          <ColumnRow
            key={col.name}
            parent={table}
            ns={ns}
            parentName={table.name}
            col={col}
            ctx={ctx}
          />
        )}
      />
      <SubItemGroup
        ctx={ctx}
        parentKey={key}
        subKind="partitions"
        label="Partitions"
        items={table.partitions ?? []}
        getName={partitionNodeName}
        renderItem={(part) => (
          <PartitionRow
            key={partitionNodeName(part)}
            ns={ns}
            partition={part}
            ctx={ctx}
          />
        )}
      />
      <SubItemGroup
        ctx={ctx}
        parentKey={key}
        subKind="constraints"
        label="Constraints"
        items={tableConstraintItems(table)}
        renderItem={(c) => (
          <ConstraintRow key={c.name} ns={ns} item={c} ctx={ctx} />
        )}
      />
      <SubItemGroup
        ctx={ctx}
        parentKey={key}
        subKind="indexes"
        label="Indexes"
        items={table.indexes ?? []}
        renderItem={(idx) => (
          <IndexRow key={idx.name} ns={ns} index={idx} ctx={ctx} />
        )}
      />
      <SubItemGroup
        ctx={ctx}
        parentKey={key}
        subKind="triggers"
        label="Triggers"
        items={table.triggers ?? []}
        renderItem={(trg) => (
          <TriggerRow
            key={`${trg.name}/${trg.timing}/${trg.manipulation}`}
            ns={ns}
            trigger={trg}
            ctx={ctx}
          />
        )}
      />
    </TreeRow>
  );
}

// HypertableBadge marks a TimescaleDB hypertable. The full metadata (chunk
// count, compression) is readable on hover; the row's context menu offers
// "View hypertable details" for the modal version.
function HypertableBadge({ info }: { info: HypertableInfo }) {
  return (
    <span
      title={hypertableDetails(info)}
      className="inline-flex items-center whitespace-nowrap rounded bg-purple-100 px-1 py-px text-[11px] leading-tight text-purple-700"
    >
      hypertable · {info.num_chunks}c{info.compression_enabled ? ' · zip' : ''}
    </span>
  );
}

// ForeignTableBadge marks a foreign (FDW-backed) table. The full binding
// (server, wrapper, options) is readable on hover; the row's context menu
// offers "View FDW details" for the modal version.
function ForeignTableBadge({ info }: { info: ForeignTableInfo }) {
  return (
    <span
      title={foreignTableDetails(info)}
      className="inline-flex items-center whitespace-nowrap rounded bg-sky-100 px-1 py-px text-[11px] leading-tight text-sky-700"
    >
      fdw · {info.server}
    </span>
  );
}
