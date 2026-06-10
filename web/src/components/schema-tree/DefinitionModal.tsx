import { Modal } from '../Modal';
import { SqlCodeView } from '../SqlCodeView';

interface DefinitionModalProps {
  title: string;
  text: string;
  onClose: () => void;
}

// DefinitionModal shows a SQL definition (view/routine definition, trigger
// statement, partition bound, …) in a syntax-highlighted modal.
export function DefinitionModal({
  title,
  text,
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
        <SqlCodeView query={text} />
      </div>
    </Modal>
  );
}
