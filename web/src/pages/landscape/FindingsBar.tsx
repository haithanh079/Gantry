import type { LandscapeFinding } from '../../lib/types';

export default function FindingsBar({ findings }: { findings: LandscapeFinding[] }) {
  const open = findings.filter((item) => item.status === 'open' || !item.status);
  const counts = { error: 0, warning: 0, info: 0 };
  for (const item of open) {
    if (item.severity === 'error' || item.severity === 'warning' || item.severity === 'info') counts[item.severity] += 1;
  }
  return (
    <div className="flex items-center gap-4 border-t border-[var(--gantry-border)] bg-[var(--gantry-bg-primary)] px-4 py-2 text-xs">
      <span className="font-medium text-[var(--gantry-text-primary)]">Findings</span>
      <span style={{ color: 'var(--gantry-danger)' }}>{counts.error} errors</span>
      <span style={{ color: 'var(--gantry-warning)' }}>{counts.warning} warnings</span>
      <span className="text-[var(--gantry-text-secondary)]">{counts.info} info</span>
      <span className="truncate text-[var(--gantry-text-secondary)]">
        {open.slice(0, 3).map((item) => `${item.rule} ${item.subjectRef}`).join(' · ')}
      </span>
    </div>
  );
}
