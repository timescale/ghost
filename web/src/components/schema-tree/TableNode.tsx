import type { HypertableInfo, TableSchema } from '../../schema';
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
import { tableMenuItems } from './menus';
import { TreeRow } from './rows';
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
        table.hypertable ? <HypertableBadge info={table.hypertable} /> : null
      }
      onContextMenu={contextMenuHandler(ctx, () =>
        tableMenuItems(ns, table, 'table'),
      )}
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

function HypertableBadge({ info }: { info: HypertableInfo }) {
  return (
    <span className="inline-flex items-center whitespace-nowrap rounded bg-purple-100 px-1 py-px text-[11px] leading-tight text-purple-700">
      hypertable · {info.num_chunks}c{info.compression_enabled ? ' · zip' : ''}
    </span>
  );
}
