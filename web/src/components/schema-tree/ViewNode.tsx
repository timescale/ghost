import {
  type ContinuousAggregateInfo,
  continuousAggregateDetails,
  type ViewSchema,
} from '../../schema';
import { highlight } from '../../util/highlight';
import { childKey } from './keys';
import { ColumnRow, IndexRow, TriggerRow } from './leafRows';
import { continuousAggregateMenuItems, viewMenuItems } from './menus';
import { CommentBadge, TreeRow } from './rows';
import { SubItemGroup } from './SubItemGroup';
import { filterForSearch } from './search';
import { contextMenuHandler, type TreeContext } from './TreeContext';

interface ViewNodeProps {
  ctx: TreeContext;
  ns: string;
  view: ViewSchema;
  kind: 'view' | 'matview';
}

export function ViewNode({ ns, view, kind, ctx }: ViewNodeProps) {
  const isMatView = kind !== 'view';
  const groupKind = isMatView ? 'matViews' : 'views';
  const key = childKey(ns, groupKind, view.name);
  const allCols = view.columns ?? [];
  // When a search is active, only render the children that themselves match
  // (same behavior as TableNode).
  const cols = filterForSearch(ctx, key, 'columns', allCols);
  return (
    <TreeRow
      ctx={ctx}
      nodeKey={key}
      label={highlight(view.name, ctx.searchTerm)}
      depth={2}
      hasChildren
      rightDetail={
        <>
          {view.continuous_aggregate ? (
            <ContinuousAggregateBadge info={view.continuous_aggregate} />
          ) : null}
          <CommentBadge
            title={view.name}
            comment={view.comment}
            showModal={ctx.showModal}
          />
        </>
      }
      onContextMenu={contextMenuHandler(ctx, () => [
        ...continuousAggregateMenuItems(
          view.name,
          view.continuous_aggregate,
          ctx.showModal,
        ),
        ...viewMenuItems(
          ns,
          view,
          kind === 'view' ? 'view' : 'materialized view',
          ctx.showModal,
        ),
      ])}
    >
      {/*
        Regular views can only have columns, so we render them directly
        under the view (no "Columns" group). Materialized views behave more
        like tables — they also carry indexes — so we group their columns
        to keep the two sibling lists from floating at the same level.
      */}
      {isMatView ? (
        <SubItemGroup
          ctx={ctx}
          parentKey={key}
          subKind="columns"
          label="Columns"
          items={allCols}
          renderItem={(col) => (
            <ColumnRow
              key={col.name}
              parent={view}
              ns={ns}
              parentName={view.name}
              col={col}
              ctx={ctx}
            />
          )}
        />
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
      <SubItemGroup
        ctx={ctx}
        parentKey={key}
        subKind="indexes"
        label="Indexes"
        items={view.indexes ?? []}
        renderItem={(idx) => (
          <IndexRow key={idx.name} ns={ns} index={idx} ctx={ctx} />
        )}
      />
      <SubItemGroup
        ctx={ctx}
        parentKey={key}
        subKind="triggers"
        label="Triggers"
        items={view.triggers ?? []}
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

// ContinuousAggregateBadge marks a TimescaleDB continuous aggregate (a view
// backed by an internal materialization hypertable). The full metadata
// (materialized-only, compression) is readable on hover; the row's context
// menu offers "View continuous aggregate details" for the modal version.
function ContinuousAggregateBadge({ info }: { info: ContinuousAggregateInfo }) {
  return (
    <span
      title={continuousAggregateDetails(info)}
      className="inline-flex items-center whitespace-nowrap rounded bg-emerald-100 px-1 py-px text-[11px] leading-tight text-emerald-700"
    >
      cagg{info.compression_enabled ? ' · zip' : ''}
    </span>
  );
}
