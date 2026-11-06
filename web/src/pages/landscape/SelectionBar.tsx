interface SelectionBarProps {
  count: number;
  onAdd: (name: string, color: string) => Promise<void>;
  onClear: () => void;
}

export default function SelectionBar({ count, onAdd, onClear }: SelectionBarProps) {
  if (count === 0) return null;
  return (
    <form
      className="absolute bottom-3 left-3 z-10 flex items-center gap-2 rounded-lg border border-[var(--gantry-border)] bg-[var(--gantry-bg-primary)] px-3 py-2 text-sm"
      onSubmit={(event) => {
        event.preventDefault();
        const data = new FormData(event.currentTarget);
        const name = String(data.get('name') || '').trim();
        const color = String(data.get('color') || 'mark-1');
        if (!name) return;
        const form = event.currentTarget;
        void onAdd(name, color).then(() => form.reset());
      }}
    >
      <span className="text-[var(--gantry-text-secondary)]">{count} selected</span>
      <input name="name" placeholder="Highlight name" className="w-36 rounded border border-[var(--gantry-border)] bg-[var(--gantry-bg-secondary)] px-2 py-1" />
      <select name="color" defaultValue="mark-1" className="rounded border border-[var(--gantry-border)] bg-[var(--gantry-bg-secondary)] px-2 py-1">
        {['mark-1', 'mark-2', 'mark-3', 'mark-4', 'mark-5', 'mark-6'].map((color) => (
          <option key={color} value={color}>{color}</option>
        ))}
      </select>
      <button type="submit" className="rounded bg-[var(--gantry-accent)] px-2 py-1 text-[var(--gantry-bg-primary)]">Add to highlight…</button>
      <button type="button" onClick={onClear} className="text-[var(--gantry-text-secondary)]">Clear</button>
    </form>
  );
}
