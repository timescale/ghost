import { Modal } from '../Modal';
import { SqlCodeView } from '../SqlCodeView';
import type { ModalFormat } from './TreeContext';

interface DefinitionModalProps {
  title: string;
  text: string;
  format: ModalFormat;
  onClose: () => void;
}

// DefinitionModal shows a SQL definition (view/routine definition, trigger
// statement, partition bound, …) in a syntax-highlighted modal, or — with
// format 'text' — plain prose (object comments) in the same editor view
// without SQL highlighting.
export function DefinitionModal({
  title,
  text,
  format,
  onClose,
}: DefinitionModalProps) {
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
        <SqlCodeView
          query={text}
          language={format === 'text' ? 'plaintext' : undefined}
        />
      </div>
    </Modal>
  );
}
