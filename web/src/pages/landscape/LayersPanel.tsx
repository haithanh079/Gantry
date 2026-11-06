import type { LandscapeHighlight } from '../../lib/types';

interface LayersPanelProps {
  highlights: LandscapeHighlight[];
  active: string[];
  only: boolean;
  network: boolean;
  data: boolean;
  onToggleHighlight: (name: string) => void;
  onToggleLayer: (layer: 'network' | 'data') => void;
  onOnly: (value: boolean) => void;
}

export function markVar(color: string) {
  const index = color.replace('mark-', '');
  if (['1', '2', '3', '4', '5', '6'].includes(index)) return `var(--gantry-mark-${index})`;
  return 'var(--gantry-accent)';
}

export default function LayersPanel(props: LayersPanelProps) {
  return (
    <div className="absolute right-3 top-3 z-10 w-56 rounded-lg border border-[var(--gantry-border)] bg-[var(--gantry-bg-primary)] p-3 text-sm shadow-sm">
      <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-[var(--gantry-text-secondary)]">Layers</p>
      <label className="mb-1 flex items-center gap-2">
        <input type="checkbox" checked={props.network} onChange={() => props.onToggleLayer('network')} />
        Network
      </label>
      <label className="mb-3 flex items-center gap-2">
        <input type="checkbox" checked={props.data} onChange={() => props.onToggleLayer('data')} />
        Data flows
      </label>
      {props.highlights.map((item) => (
        <label key={item.ref} className="mb-1 flex items-center gap-2">
          <input type="checkbox" checked={props.active.includes(item.name)} onChange={() => props.onToggleHighlight(item.name)} />
          <span className="h-2.5 w-2.5 rounded-full" style={{ background: markVar(item.color) }} />
          <span className="truncate">{item.name}</span>
        </label>
      ))}
      <label className="mt-2 flex items-center gap-2 border-t border-[var(--gantry-border)] pt-2">
        <input type="checkbox" checked={props.only} onChange={(e) => props.onOnly(e.target.checked)} />
        Only show highlighted
      </label>
    </div>
  );
}
