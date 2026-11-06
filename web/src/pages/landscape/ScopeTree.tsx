import { useEffect, useState } from 'react';
import { api } from '../../lib/api';
import type { Entity } from '../../lib/types';

interface ScopeTreeProps {
  department: string;
  project: string;
  service: string;
  onSelect: (next: { department?: string; project?: string; service?: string }) => void;
}

function refName(value: unknown): string {
  if (!value || typeof value !== 'object') return '';
  const name = (value as { name?: unknown }).name;
  return typeof name === 'string' ? name : '';
}

export default function ScopeTree({ department, project, service, onSelect }: ScopeTreeProps) {
  const [departments, setDepartments] = useState<Entity[]>([]);
  const [projects, setProjects] = useState<Entity[]>([]);
  const [services, setServices] = useState<Entity[]>([]);

  useEffect(() => {
    let active = true;
    Promise.all([
      api.listEntities('Department').catch(() => []),
      api.listEntities('Project').catch(() => []),
      api.listEntities('Service').catch(() => []),
    ]).then(([deps, projs, svcs]) => {
      if (!active) return;
      setDepartments(deps);
      setProjects(projs);
      setServices(svcs);
    });
    return () => { active = false; };
  }, []);

  const deptNames = departments.map((item) => item.metadata.name);
  const byDept = new Map<string, Entity[]>();
  for (const item of projects) {
    const key = refName(item.spec?.department) || 'Unassigned';
    byDept.set(key, [...(byDept.get(key) || []), item]);
  }
  const keys = [...new Set([...deptNames, ...byDept.keys()])];

  return (
    <aside className="w-56 shrink-0 overflow-y-auto border-r border-[var(--gantry-border)] bg-[var(--gantry-bg-primary)] p-3 text-sm">
      <button
        className="mb-3 text-xs font-medium text-[var(--gantry-text-secondary)] hover:text-[var(--gantry-text-primary)]"
        onClick={() => onSelect({ department: '', project: '', service: '' })}
      >
        All departments
      </button>
      {keys.map((dept) => (
        <div key={dept} className="mb-3">
          <button
            className={`font-medium ${department === dept ? 'text-[var(--gantry-accent)]' : 'text-[var(--gantry-text-primary)]'}`}
            onClick={() => onSelect({ department: dept === 'Unassigned' ? '' : dept, project: '', service: '' })}
          >
            {dept}
          </button>
          <ul className="mt-1 space-y-1 pl-3">
            {(byDept.get(dept) || []).map((item) => (
              <li key={item.metadata.name}>
                <button
                  className={project === item.metadata.name ? 'text-[var(--gantry-accent)]' : 'text-[var(--gantry-text-secondary)]'}
                  onClick={() => onSelect({ department: dept === 'Unassigned' ? '' : dept, project: item.metadata.name, service: '' })}
                >
                  {item.metadata.title || item.metadata.name}
                </button>
                <ul className="pl-3">
                  {services.filter((svc) => {
                    const refs = svc.spec?.projects;
                    return Array.isArray(refs) && refs.some((ref) => refName(ref) === item.metadata.name);
                  }).map((svc) => (
                    <li key={svc.metadata.name}>
                      <button
                        className={service === svc.metadata.name ? 'text-[var(--gantry-accent)]' : 'text-[var(--gantry-text-secondary)]'}
                        onClick={() => onSelect({
                          department: dept === 'Unassigned' ? '' : dept,
                          project: item.metadata.name,
                          service: svc.metadata.name,
                        })}
                      >
                        {svc.metadata.name}
                      </button>
                    </li>
                  ))}
                </ul>
              </li>
            ))}
          </ul>
        </div>
      ))}
    </aside>
  );
}
