import { Icon, type IconName } from '../Icon';
import type { ResultView } from './types';

const OPTIONS: { value: ResultView; label: string; icon: IconName }[] = [
  { value: 'table', label: 'Table', icon: 'table' },
  { value: 'chart', label: 'Chart', icon: 'chart' },
  { value: 'editor', label: 'Edit', icon: 'code' },
];

interface Props {
  value: ResultView;
  onChange: (next: ResultView) => void;
}

// ResultViewToggle is a small segmented control, rendered in the query widget's
// toolbar, that switches the area below the editor between the results table,
// the rendered chart, and the chart config editor.
export function ResultViewToggle({ value, onChange }: Props) {
  return (
    <div className="flex items-center gap-0.5 rounded border border-slate-300 p-0.5">
      {OPTIONS.map((opt) => {
        const active = opt.value === value;
        return (
          <button
            key={opt.value}
            type="button"
            onClick={() => onChange(opt.value)}
            aria-pressed={active}
            aria-label={opt.label}
            title={opt.label}
            className={`flex items-center rounded p-1 transition-colors ${
              active
                ? 'bg-slate-800 text-white'
                : 'text-slate-600 hover:bg-slate-100'
            }`}
          >
            <Icon name={opt.icon} size="sm" color="current" />
          </button>
        );
      })}
    </div>
  );
}
