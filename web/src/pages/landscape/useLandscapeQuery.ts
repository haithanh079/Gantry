import { useSearchParams } from 'react-router-dom';

export interface LandscapeQuery {
  lens: 'map' | 'inventory';
  level: 'company' | 'project' | 'deployment';
  groupBy: 'org' | 'cloud' | 'env';
  layers: string;
  hl: string | null;
  only: boolean;
  env: string;
  cloud: string;
  region: string;
  runtime: string;
  team: string;
  project: string;
  department: string;
  lifecycle: string;
  service: string;
  tab: 'matrix' | 'flows';
}

const FILTER_KEYS = ['env', 'cloud', 'region', 'runtime', 'team', 'project', 'department', 'lifecycle', 'service'] as const;

export function useLandscapeQuery() {
  const [params, setParams] = useSearchParams();

  const query: LandscapeQuery = {
    lens: params.get('lens') === 'inventory' ? 'inventory' : 'map',
    level: (['company', 'project', 'deployment'].includes(params.get('level') || '') ? params.get('level') : 'deployment') as LandscapeQuery['level'],
    groupBy: (['org', 'cloud', 'env'].includes(params.get('groupBy') || '') ? params.get('groupBy') : 'org') as LandscapeQuery['groupBy'],
    layers: params.get('layers') || 'network,data',
    hl: params.has('hl') ? params.get('hl') : null,
    only: params.get('only') === '1',
    env: params.get('env') || '',
    cloud: params.get('cloud') || '',
    region: params.get('region') || '',
    runtime: params.get('runtime') || '',
    team: params.get('team') || '',
    project: params.get('project') || '',
    department: params.get('department') || '',
    lifecycle: params.get('lifecycle') || '',
    service: params.get('service') || '',
    tab: params.get('tab') === 'flows' ? 'flows' : 'matrix',
  };

  function patch(next: Partial<Record<string, string | null>>) {
    const updated = new URLSearchParams(params);
    for (const [key, value] of Object.entries(next)) {
      if (value == null || value === '') updated.delete(key);
      else updated.set(key, value);
    }
    setParams(updated, { replace: true });
  }

  function apiQuery(includeGraph: boolean) {
    const search = new URLSearchParams();
    if (includeGraph) {
      search.set('groupBy', query.groupBy);
      search.set('level', query.level);
      search.set('layers', query.layers);
    }
    for (const key of FILTER_KEYS) {
      if (key === 'service') continue;
      if (query[key]) search.set(key, query[key]);
    }
    const text = search.toString();
    return text ? `?${text}` : '';
  }

  return { query, patch, apiQuery };
}
