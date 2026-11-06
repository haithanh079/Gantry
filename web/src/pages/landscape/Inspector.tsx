import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../../lib/api';
import { catalogEntityPath, parseCatalogRef } from '../../lib/utils';
import type { LandscapeBlast, LandscapeDataFlow, LandscapeFinding, LandscapeGraph, LandscapeNode } from '../../lib/types';

interface InspectorProps {
  nodeId: string;
  flowRef: string;
  graph: LandscapeGraph | null;
  flows: LandscapeDataFlow[];
  findings: LandscapeFinding[];
  onClose: () => void;
}

export default function Inspector({ nodeId, flowRef, graph, flows, findings, onClose }: InspectorProps) {
  const [blast, setBlast] = useState<LandscapeBlast | null>(null);
  const node = graph?.nodes.find((item) => item.id === nodeId);
  const flow = flows.find((item) => item.ref === flowRef);
  const subject = node?.id || flow?.ref || '';

  useEffect(() => { setBlast(null); }, [subject]);

  if (!node && !flow) return null;

  const related = flows.filter((item) => item.source === subject || item.target === subject || item.actor === subject || item.ref === flowRef);
  const marks = (graph?.highlights || []).filter((item) => item.nodeIds.includes(nodeId) || item.edgeIds.some((id) => id.includes(flowRef)));
  const subjectFindings = findings.filter((item) => item.subjectRef === subject || (node && item.subjectRef.endsWith(`/${node.name}`)));

  async function loadBlast(target: LandscapeNode | { ref: string }) {
    const id = 'id' in target ? target.id : target.ref;
    const ref = parseCatalogRef(id);
    if (!ref?.kind || !ref.name) return;
    setBlast(await api.getLandscapeBlast(ref.kind, ref.namespace || 'default', ref.name));
  }

  const link = node ? catalogEntityPath(node.kind, node.name, node.namespace) : flow ? catalogEntityPath('DataFlow', flow.name, flow.namespace) : '';

  return (
    <aside className="w-80 shrink-0 overflow-y-auto border-l border-[var(--gantry-border)] bg-[var(--gantry-bg-primary)] p-4 text-sm">
      <div className="mb-3 flex items-start justify-between gap-2">
        <div>
          <p className="text-xs uppercase text-[var(--gantry-text-secondary)]">{node?.kind || 'DataFlow'}</p>
          <h2 className="text-base font-semibold">{node?.title || node?.name || flow?.name}</h2>
        </div>
        <button onClick={onClose} className="text-[var(--gantry-text-secondary)]">Close</button>
      </div>
      {link && <Link to={link} className="text-[var(--gantry-accent)]">Open in catalog</Link>}
      {node?.meta && (
        <dl className="mt-3 space-y-1">
          {Object.entries(node.meta).map(([key, value]) => (
            <div key={key} className="flex justify-between gap-3">
              <dt className="text-[var(--gantry-text-secondary)]">{key}</dt>
              <dd className="truncate">{Array.isArray(value) ? value.join(', ') : String(value ?? '')}</dd>
            </div>
          ))}
        </dl>
      )}
      {flow && (
        <dl className="mt-3 space-y-1">
          <div>Source {flow.source}</div>
          <div>Target {flow.target}</div>
          <div>Actor {flow.actor || '—'}</div>
          <div>Transport {flow.transport}</div>
          <div>Classes {(flow.dataClasses || []).join(', ') || '—'}</div>
        </dl>
      )}
      <h3 className="mt-4 font-medium">Related flows</h3>
      <ul className="mt-1 space-y-1">
        {related.map((item) => <li key={item.ref}>{item.name}</li>)}
        {related.length === 0 && <li className="text-[var(--gantry-text-secondary)]">None</li>}
      </ul>
      <h3 className="mt-4 font-medium">Highlights</h3>
      <ul className="mt-1 space-y-1">
        {marks.map((item) => <li key={item.ref}>{item.name}</li>)}
        {marks.length === 0 && <li className="text-[var(--gantry-text-secondary)]">None</li>}
      </ul>
      <h3 className="mt-4 font-medium">Findings</h3>
      <ul className="mt-1 space-y-1">
        {subjectFindings.map((item) => <li key={item.id}>{item.severity}: {item.rule}</li>)}
        {subjectFindings.length === 0 && <li className="text-[var(--gantry-text-secondary)]">None</li>}
      </ul>
      <button
        className="mt-4 rounded bg-[var(--gantry-accent)] px-3 py-1.5 text-[var(--gantry-bg-primary)]"
        onClick={() => void loadBlast(node || { ref: flowRef })}
      >
        Blast radius
      </button>
      {blast && (
        <ul className="mt-3 space-y-2">
          {blast.groups.map((group) => (
            <li key={group.project}>
              <div className="font-medium">{group.project}</div>
              <div className="text-[var(--gantry-text-secondary)]">{group.deployments.length} deployments, {group.dataFlows.length} flows</div>
            </li>
          ))}
          {blast.groups.length === 0 && <li className="text-[var(--gantry-text-secondary)]">No downstream impact in the catalog.</li>}
        </ul>
      )}
    </aside>
  );
}
