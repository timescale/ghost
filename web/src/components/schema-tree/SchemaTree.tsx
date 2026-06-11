import { useMemo, useState } from 'react';

import type { NamespacedSchema } from '../../schema';
import { ContextMenu, type ContextMenuState } from '../ContextMenu';
import { DefinitionModal } from './DefinitionModal';
import { schemaKey } from './keys';
import { SchemaNode } from './SchemaNode';
import { computeSearch } from './search';
import type { ModalFormat, TreeContext } from './TreeContext';
import { useSchemaTreeExpansion } from './useSchemaTreeExpansion';

interface SchemaTreeProps {
  databaseId: string;
  schemas: NamespacedSchema[];
  searchTerm: string;
}

export function SchemaTree({
  databaseId,
  schemas,
  searchTerm,
}: SchemaTreeProps) {
  const { expanded, collapsedDuringSearch, searchActive, toggleNode } =
    useSchemaTreeExpansion(databaseId, searchTerm);

  const search = useMemo(
    () => computeSearch(schemas, searchTerm),
    [schemas, searchTerm],
  );

  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null);
  const [definitionModal, setDefinitionModal] = useState<{
    title: string;
    text: string;
    format: ModalFormat;
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
      showModal: (title, text, format = 'sql') =>
        setDefinitionModal({ title, text, format }),
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
          format={definitionModal.format}
          onClose={() => setDefinitionModal(null)}
        />
      ) : null}
    </div>
  );
}
