import type { LandscapeDataFlow, LandscapeInventory } from '../../lib/types';

interface InventoryLensProps {
  inventory: LandscapeInventory | null;
  flows: LandscapeDataFlow[];
  tab: 'matrix' | 'flows';
  onTab: (tab: 'matrix' | 'flows') => void;
  onOpen: (ref: string) => void;
}

export default function InventoryLens({ inventory, flows, tab, onTab, onOpen }: InventoryLensProps) {
  const kpis = inventory?.kpis || {};
  return (
    <div className="flex h-full flex-col overflow-hidden">
      <div className="flex gap-4 border-b border-[var(--gantry-border)] px-4 py-3 text-sm">
        {Object.entries(kpis).map(([key, value]) => (
          <div key={key}>
            <div className="text-lg font-semibold text-[var(--gantry-text-primary)]">{value}</div>
            <div className="text-xs uppercase text-[var(--gantry-text-secondary)]">{key}</div>
          </div>
        ))}
        <div className="ml-auto flex gap-2">
          <button className={tab === 'matrix' ? 'text-[var(--gantry-accent)]' : 'text-[var(--gantry-text-secondary)]'} onClick={() => onTab('matrix')}>Matrix</button>
          <button className={tab === 'flows' ? 'text-[var(--gantry-accent)]' : 'text-[var(--gantry-text-secondary)]'} onClick={() => onTab('flows')}>Data flows</button>
        </div>
      </div>
      <div className="flex-1 overflow-auto p-4">
        {tab === 'matrix' && inventory?.groups.map((group) => (
          <section key={group.project} className="mb-6">
            <h3 className="mb-2 text-sm font-semibold">{group.project}</h3>
            <table className="w-full border-collapse text-left text-sm">
              <thead>
                <tr className="text-[var(--gantry-text-secondary)]">
                  <th className="border-b border-[var(--gantry-border)] py-2 pr-3">Service</th>
                  {inventory.envs.map((env) => <th key={env} className="border-b border-[var(--gantry-border)] py-2 pr-3">{env}</th>)}
                </tr>
              </thead>
              <tbody>
                {group.rows.map((row) => (
                  <tr key={row.service}>
                    <td className="border-b border-[var(--gantry-border)] py-2 pr-3">{row.service}</td>
                    {inventory.envs.map((env) => {
                      const cell = row.cells[env];
                      return (
                        <td key={env} className="border-b border-[var(--gantry-border)] py-2 pr-3">
                          {cell ? (
                            <button className="text-left" onClick={() => onOpen(cell.deployment)}>
                              <div>{cell.runtime || cell.deployment.split('/').pop()}</div>
                              <div className="text-xs text-[var(--gantry-text-secondary)]">{[cell.cloud, cell.region, cell.version].filter(Boolean).join(' · ')}</div>
                            </button>
                          ) : '—'}
                        </td>
                      );
                    })}
                  </tr>
                ))}
              </tbody>
            </table>
          </section>
        ))}
        {tab === 'flows' && (
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="text-[var(--gantry-text-secondary)]">
                {['Name', 'Source', 'Target', 'Actor', 'Mode', 'Transport', 'Data'].map((label) => (
                  <th key={label} className="border-b border-[var(--gantry-border)] py-2 pr-3">{label}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {flows.map((flow) => (
                <tr key={flow.ref} className="cursor-pointer" onClick={() => onOpen(flow.ref)}>
                  <td className="border-b border-[var(--gantry-border)] py-2 pr-3">{flow.name}</td>
                  <td className="border-b border-[var(--gantry-border)] py-2 pr-3">{flow.source}</td>
                  <td className="border-b border-[var(--gantry-border)] py-2 pr-3">{flow.target}</td>
                  <td className="border-b border-[var(--gantry-border)] py-2 pr-3">{flow.actor || '—'}</td>
                  <td className="border-b border-[var(--gantry-border)] py-2 pr-3">{flow.mode}</td>
                  <td className="border-b border-[var(--gantry-border)] py-2 pr-3">{flow.transport}</td>
                  <td className="border-b border-[var(--gantry-border)] py-2 pr-3">{(flow.dataClasses || []).join(', ')}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
