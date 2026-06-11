import type { NamespacedSchema, TableSchema } from '../../schema';
import { tableConstraintItems } from './constraints';
import {
  childKey,
  type GroupKind,
  itemLabel,
  partitionNodeName,
  schemaKey,
  subItemKey,
} from './keys';
import type { TreeContext } from './TreeContext';

export interface SearchInfo {
  visible: Set<string>;
}

// computeSearch walks the full schema tree and collects the node keys that
// should remain visible for the given search term: every node whose label
// matches, plus all of its ancestors (so matches are reachable).
export function computeSearch(
  schemas: NamespacedSchema[],
  term: string,
): SearchInfo {
  const visible = new Set<string>();
  if (!term) return { visible };
  const lower = term.toLowerCase();
  const match = (s: string) => s.toLowerCase().includes(lower);

  for (const ns of schemas) {
    const sKey = schemaKey(ns);
    // When the schema's own name matches, show the entire schema (every group,
    // item, and sub-item) rather than an empty header — mirroring the
    // item-name behavior below, where matching a table reveals the whole
    // table. schemaHit forces every descendant visible.
    const schemaHit = match(ns.name);
    let anyHit = schemaHit;

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
        // A schema-name hit makes every item a full match, so the whole
        // schema is revealed; otherwise an item matches on its own label.
        const itemHit = schemaHit || match(label);
        let childHit = false;
        // considerSub is the single place that enumerates a sub-list and
        // derives its node keys, so the two visibility modes stay in
        // lockstep: when the item itself matched by name (or the schema name
        // matched), every sub-item key is added (the user searched for the
        // table/schema; show the whole thing, not an empty shell); otherwise
        // only matching sub-items are added (searching collapses the group
        // down to the matches, like popsql).
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
            const subHit = match(sub.name);
            if (subHit) {
              childHit = true;
            }
            if (itemHit || subHit) {
              visible.add(subItemKey(iKey, subKind, keyName(sub)));
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

// filterForSearch narrows a group item's sub-items (columns, indexes, etc.) to
// those that matched the active search, mirroring popsql: searching collapses
// a group down to just the matching rows. When no search is active it returns
// the list unchanged. The key derivation must stay in lockstep with
// computeSearch's, so getName defaults to the bare `name` but can be
// overridden (e.g. partitionNodeName, which keys cross-schema partitions
// uniquely).
export function filterForSearch<T extends { name: string }>(
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
