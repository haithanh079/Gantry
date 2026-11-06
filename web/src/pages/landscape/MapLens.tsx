import { useEffect, useMemo, useState } from 'react';
import {
  ReactFlow,
  Background,
  Controls,
  MarkerType,
  type Node,
  type Edge,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import type { LandscapeGraph } from '../../lib/types';
import LayersPanel, { markVar } from './LayersPanel';

interface MapLensProps {
  graph: LandscapeGraph;
  activeHighlights: string[];
  only: boolean;
  network: boolean;
  data: boolean;
  service: string;
  selected: string[];
  onToggleHighlight: (name: string) => void;
  onToggleLayer: (layer: 'network' | 'data') => void;
  onOnly: (value: boolean) => void;
  onSelect: (ids: string[], primary: string) => void;
}

interface LayoutPoint {
  id: string;
  x?: number;
  y?: number;
  children?: LayoutPoint[];
}

export default function MapLens(props: MapLensProps) {
  const [points, setPoints] = useState<Record<string, { x: number; y: number }>>({});

  const highlightedNodes = useMemo(() => {
    const ids = new Set<string>();
    for (const item of props.graph.highlights) {
      if (props.activeHighlights.includes(item.name)) item.nodeIds.forEach((id) => ids.add(id));
    }
    return ids;
  }, [props.graph.highlights, props.activeHighlights]);

  const visibleNodes = props.graph.nodes.filter((node) => {
    if (props.service && node.kind === 'Deployment' && !node.name.startsWith(`${props.service}-`) && node.name !== props.service) return false;
    if (props.only && props.activeHighlights.length > 0 && !highlightedNodes.has(node.id)) return false;
    return true;
  });
  const visibleIds = new Set(visibleNodes.map((node) => node.id));
  const visibleEdges = props.graph.edges.filter((edge) => {
    if (edge.layer === 'network' && !props.network) return false;
    if (edge.layer === 'data' && !props.data) return false;
    return visibleIds.has(edge.from) && visibleIds.has(edge.to);
  });

  useEffect(() => {
    const worker = new Worker(new URL('./elk.worker.ts', import.meta.url), { type: 'module' });
    const graph = {
      id: 'root',
      layoutOptions: {
        'elk.algorithm': 'layered',
        'elk.direction': 'RIGHT',
        'elk.spacing.nodeNode': '40',
        'elk.layered.spacing.nodeNodeBetweenLayers': '80',
      },
      children: visibleNodes.map((node) => ({ id: node.id, width: 180, height: 56 })),
      edges: visibleEdges.map((edge) => ({ id: edge.id, sources: [edge.from], targets: [edge.to] })),
    };
    worker.onmessage = (event: MessageEvent<{ ok: boolean; layout?: LayoutPoint }>) => {
      if (!event.data.ok || !event.data.layout?.children) return;
      const next: Record<string, { x: number; y: number }> = {};
      for (const child of event.data.layout.children) {
        next[child.id] = { x: child.x || 0, y: child.y || 0 };
      }
      setPoints(next);
    };
    worker.postMessage(graph);
    return () => worker.terminate();
  }, [props.graph, props.network, props.data, props.only, props.service, props.activeHighlights]);

  const nodes: Node[] = visibleNodes.map((node, index) => {
    const marks = props.graph.highlights.filter((item) => props.activeHighlights.includes(item.name) && item.nodeIds.includes(node.id));
    const dim = props.activeHighlights.length > 0 && !highlightedNodes.has(node.id);
    const pos = points[node.id] || { x: (index % 6) * 200, y: Math.floor(index / 6) * 80 };
    return {
      id: node.id,
      position: pos,
      data: { label: node.title || node.name },
      selected: props.selected.includes(node.id),
      style: {
        width: 180,
        background: 'var(--gantry-bg-primary)',
        color: 'var(--gantry-text-primary)',
        border: `2px solid ${marks[0] ? markVar(marks[0].color) : 'var(--gantry-border)'}`,
        borderRadius: 8,
        fontSize: 12,
        opacity: dim ? 0.28 : 1,
      },
    };
  });

  const edges: Edge[] = visibleEdges.map((edge) => {
    const sensitive = (edge.dataClasses || []).some((item) => item === 'pii' || item === 'payment');
    const actor = edge.actorRef ? edge.actorRef.split('/').pop() : '';
    return {
      id: edge.id,
      source: edge.from,
      target: edge.to,
      animated: edge.layer === 'data',
      label: edge.layer === 'data' ? [actor, edge.mode].filter(Boolean).join(' · ') : edge.count ? String(edge.count) : undefined,
      markerEnd: { type: MarkerType.ArrowClosed, color: sensitive ? 'var(--gantry-warning)' : 'var(--gantry-text-secondary)' },
      style: {
        stroke: sensitive ? 'var(--gantry-warning)' : edge.layer === 'data' ? 'var(--gantry-accent)' : 'var(--gantry-text-secondary)',
        strokeWidth: sensitive ? 2.5 : 1.5,
        strokeDasharray: edge.layer === 'data' ? '6 4' : undefined,
        fill: 'none',
      },
    };
  });

  return (
    <div className="relative h-full min-h-[480px]">
      <ReactFlow
        nodes={nodes}
        edges={edges}
        fitView
        onNodeClick={(event, node) => {
          if (event.shiftKey) {
            const next = props.selected.includes(node.id)
              ? props.selected.filter((id) => id !== node.id)
              : [...props.selected, node.id];
            props.onSelect(next, node.id);
            return;
          }
          props.onSelect([node.id], node.id);
        }}
        onPaneClick={() => props.onSelect([], '')}
      >
        <Background />
        <Controls />
      </ReactFlow>
      <LayersPanel
        highlights={props.graph.highlights}
        active={props.activeHighlights}
        only={props.only}
        network={props.network}
        data={props.data}
        onToggleHighlight={props.onToggleHighlight}
        onToggleLayer={props.onToggleLayer}
        onOnly={props.onOnly}
      />
    </div>
  );
}
