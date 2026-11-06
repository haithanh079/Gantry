import { useEffect, useMemo, useState } from 'react';
import { api } from '../lib/api';
import type { Entity, LandscapeDataFlow, LandscapeFinding, LandscapeGraph, LandscapeInventory } from '../lib/types';
import { parseCatalogRef } from '../lib/utils';
import FindingsBar from './landscape/FindingsBar';
import Inspector from './landscape/Inspector';
import InventoryLens from './landscape/InventoryLens';
import MapLens from './landscape/MapLens';
import ScopeTree from './landscape/ScopeTree';
import SelectionBar from './landscape/SelectionBar';
import { useLandscapeQuery } from './landscape/useLandscapeQuery';

const FILTERS = ['env', 'cloud', 'region', 'runtime', 'team', 'lifecycle'] as const;

export default function Landscape() {
  const { query, patch, apiQuery } = useLandscapeQuery();
  const [graph, setGraph] = useState<LandscapeGraph | null>(null);
  const [inventory, setInventory] = useState<LandscapeInventory | null>(null);
  const [flows, setFlows] = useState<LandscapeDataFlow[]>([]);
  const [findings, setFindings] = useState<LandscapeFinding[]>([]);
  const [error, setError] = useState('');
  const [selected, setSelected] = useState<string[]>([]);
  const [primary, setPrimary] = useState('');
  const [flowRef, setFlowRef] = useState('');

  useEffect(() => {
    let active = true;
    setError('');
    const graphQuery = apiQuery(true);
    const listQuery = apiQuery(false);
    Promise.all([
      api.getLandscapeGraph(graphQuery),
      api.getLandscapeInventory(listQuery),
      api.getLandscapeDataFlows(listQuery),
      api.getLandscapeFindings('?status=open'),
    ]).then(([nextGraph, nextInventory, nextFlows, nextFindings]) => {
      if (!active) return;
      setGraph(nextGraph);
      setInventory(nextInventory);
      setFlows(nextFlows.dataflows || []);
      setFindings(nextFindings.findings || []);
    }).catch((err: Error) => {
      if (active) setError(err.message);
    });
    return () => { active = false; };
  }, [query.level, query.groupBy, query.layers, query.env, query.cloud, query.region, query.runtime, query.team, query.project, query.department, query.lifecycle]);

  const activeHighlights = useMemo(() => {
    if (!graph) return [];
    if (query.hl === null) return graph.highlights.filter((item) => item.defaultOn).map((item) => item.name);
    if (query.hl === '-' || query.hl === '') return [];
    return query.hl.split(',').filter(Boolean);
  }, [graph, query.hl]);

  function toggleHighlight(name: string) {
    const next = activeHighlights.includes(name)
      ? activeHighlights.filter((item) => item !== name)
      : [...activeHighlights, name];
    patch({ hl: next.length ? next.join(',') : '-' });
  }

  function toggleLayer(layer: 'network' | 'data') {
    const parts = new Set(query.layers.split(',').filter(Boolean));
    if (parts.has(layer)) parts.delete(layer);
    else parts.add(layer);
    patch({ layers: [...parts].join(',') || 'network' });
  }

  async function addToHighlight(name: string, color: string) {
    const members = selected.flatMap((id) => {
      const ref = parseCatalogRef(id);
      if (!ref?.kind || !ref.name) return [];
      return [{ kind: ref.kind, namespace: ref.namespace || 'default', name: ref.name }];
    });
    let existing: Entity | null = null;
    try {
      existing = await api.getEntity('Highlight', name, 'default');
    } catch {
      existing = null;
    }
    const current = Array.isArray(existing?.spec?.members) ? existing.spec.members : [];
    const spec = {
      ...(existing?.spec || {}),
      color: color || existing?.spec?.color || 'mark-1',
      members: [...current, ...members],
      includeNeighbors: existing?.spec?.includeNeighbors ?? false,
    };
    if (existing) {
      await api.updateEntity('Highlight', name, { ...existing, spec }, 'default');
    } else {
      await api.createEntity({
        kind: 'Highlight',
        apiVersion: 'gantry.io/v1',
        metadata: { name, namespace: 'default', title: name },
        spec,
      });
    }
    patch({ hl: [...new Set([...activeHighlights, name])].join(',') });
    setSelected([]);
  }

  return (
    <div className="flex h-[calc(100dvh-3.5rem)] flex-col lg:h-dvh">
      <header className="flex flex-wrap items-center gap-2 border-b border-[var(--gantry-border)] bg-[var(--gantry-bg-primary)] px-4 py-2 text-sm">
        <h1 className="mr-2 font-semibold">Landscape</h1>
        <button className={query.lens === 'map' ? 'text-[var(--gantry-accent)]' : 'text-[var(--gantry-text-secondary)]'} onClick={() => patch({ lens: 'map' })}>Map</button>
        <button className={query.lens === 'inventory' ? 'text-[var(--gantry-accent)]' : 'text-[var(--gantry-text-secondary)]'} onClick={() => patch({ lens: 'inventory' })}>Inventory</button>
        <select value={query.level} onChange={(e) => patch({ level: e.target.value })} className="rounded border border-[var(--gantry-border)] bg-[var(--gantry-bg-secondary)] px-2 py-1">
          <option value="company">Company</option>
          <option value="project">Project</option>
          <option value="deployment">Deployment</option>
        </select>
        <select value={query.groupBy} onChange={(e) => patch({ groupBy: e.target.value })} className="rounded border border-[var(--gantry-border)] bg-[var(--gantry-bg-secondary)] px-2 py-1">
          <option value="org">Group by org</option>
          <option value="cloud">Group by cloud</option>
          <option value="env">Group by env</option>
        </select>
        {FILTERS.map((key) => (
          <input
            key={key}
            value={query[key]}
            placeholder={key}
            onChange={(e) => patch({ [key]: e.target.value })}
            className="w-24 rounded border border-[var(--gantry-border)] bg-[var(--gantry-bg-secondary)] px-2 py-1"
          />
        ))}
      </header>
      {error && <p className="bg-[var(--gantry-danger)]/10 px-4 py-2 text-sm" style={{ color: 'var(--gantry-danger)' }}>{error}</p>}
      <div className="flex min-h-0 flex-1">
        <ScopeTree
          department={query.department}
          project={query.project}
          service={query.service}
          onSelect={(next) => patch({
            department: next.department ?? null,
            project: next.project ?? null,
            service: next.service ?? null,
          })}
        />
        <div className="relative min-w-0 flex-1">
          {query.lens === 'map' && graph && (
            <>
              <MapLens
                graph={graph}
                activeHighlights={activeHighlights}
                only={query.only}
                network={query.layers.split(',').includes('network')}
                data={query.layers.split(',').includes('data')}
                service={query.service}
                selected={selected}
                onToggleHighlight={toggleHighlight}
                onToggleLayer={toggleLayer}
                onOnly={(value) => patch({ only: value ? '1' : null })}
                onSelect={(ids, id) => { setSelected(ids); setPrimary(id); setFlowRef(''); }}
              />
              <SelectionBar count={selected.length} onAdd={addToHighlight} onClear={() => setSelected([])} />
            </>
          )}
          {query.lens === 'inventory' && (
            <InventoryLens
              inventory={inventory}
              flows={flows}
              tab={query.tab}
              onTab={(tab) => patch({ tab })}
              onOpen={(ref) => { setFlowRef(ref.startsWith('DataFlow/') ? ref : ''); setPrimary(ref.startsWith('DataFlow/') ? '' : ref); }}
            />
          )}
        </div>
        <Inspector
          nodeId={primary}
          flowRef={flowRef}
          graph={graph}
          flows={flows}
          findings={findings}
          onClose={() => { setPrimary(''); setFlowRef(''); }}
        />
      </div>
      <FindingsBar findings={findings} />
    </div>
  );
}
