import {
  type Routine,
  routineSignature,
  type TableSchema,
  type ViewSchema,
} from '../../schema';

// The kinds of object groups rendered under a schema node.
export type GroupKind =
  | 'tables'
  | 'views'
  | 'matViews'
  | 'functions'
  | 'procedures'
  | 'enums';

// GroupItem is any item that can appear in a schema group's list.
export type GroupItem = TableSchema | ViewSchema | Routine | { name: string };

// Node keys are built by joining segments with '/'. PostgreSQL identifiers
// (schema/table/column names, routine signatures) can themselves contain '/',
// so the variable segments are escaped before joining. Without this, e.g. a
// table named `a/columns` would collide with the "columns" subgroup key of a
// table named `a`. Backslash is the escape character.
function encodeKeySegment(segment: string): string {
  return segment.replace(/\\/g, '\\\\').replace(/\//g, '\\/');
}

export function schemaKey(ns: { name: string }): string {
  return `schema:${encodeKeySegment(ns.name)}`;
}

export function childKey(ns: string, group: GroupKind, name: string): string {
  return `${schemaKey({ name: ns })}/${group}/${encodeKeySegment(name)}`;
}

// subItemKey builds the key for a leaf node nested under a group item (e.g. a
// column or partition under a table). The sub-item name is escaped for the
// same reason as childKey's name segment.
export function subItemKey(
  itemKey: string,
  subKind: string,
  name: string,
): string {
  return `${itemKey}/${subKind}/${encodeKeySegment(name)}`;
}

// partitionNodeName returns the identifier used to key a partition within its
// parent's Partitions list. PostgreSQL allows a partition to live in a
// different schema than its parent, so two partitions of the same parent can
// share a name as long as their schemas differ. Qualifying such partitions
// with their schema keeps their React keys and search-visibility keys unique;
// same-schema partitions (the common case) keep their bare name.
export function partitionNodeName(p: {
  name: string;
  schema?: string;
}): string {
  return p.schema ? `${p.schema}.${p.name}` : p.name;
}

// itemLabel returns the display/key label for a group item. Routines use
// their signature (name + identity arguments) so overloaded routines that
// share a name remain distinct in both the rendered tree and the keys/state
// derived from them.
export function itemLabel(kind: GroupKind, item: GroupItem): string {
  if (kind === 'functions' || kind === 'procedures') {
    return routineSignature(item as Routine);
  }
  return item.name;
}
