import type { ReactNode } from 'react';
import type { ResourceStatus } from './wails';

type NoticeProps = {
  kind: 'info' | 'success' | 'warning' | 'error';
  children: ReactNode;
};

export function Notice({ kind, children }: NoticeProps) {
  return <div className={`notice ${kind}`}>{children}</div>;
}

type DirectoryPickerProps = {
  label: string;
  value: string;
  onChange(value: string): void;
  onBrowse(): void;
};

export function DirectoryPicker({ label, value, onChange, onBrowse }: DirectoryPickerProps) {
  return (
    <label className="field">
      <span>{label}</span>
      <div className="row">
        <input value={value} onChange={(event) => onChange(event.target.value)} />
        <button className="secondary" type="button" onClick={onBrowse}>Browse</button>
      </div>
    </label>
  );
}

type ResourceChecklistProps = {
  values: { config: boolean; auth: boolean; skills: boolean };
  onChange(values: { config: boolean; auth: boolean; skills: boolean }): void;
  statuses?: { config: ResourceStatus; auth: ResourceStatus; skills: ResourceStatus };
  available?: { config: boolean; auth: boolean; skills: boolean };
  mode: 'share' | 'sync';
  details?: Partial<Record<'config' | 'auth' | 'skills', ReactNode>>;
};

export function ResourceChecklist({ values, onChange, statuses, available, mode, details }: ResourceChecklistProps) {
  const rows = [
    { key: 'config' as const, label: 'config.toml', detail: statuses?.config.exists ? 'Found' : 'Missing' },
    { key: 'auth' as const, label: 'auth.json', detail: statuses?.auth.exists ? 'Found, sensitive' : 'Sensitive' },
    { key: 'skills' as const, label: 'skills', detail: statuses?.skills.exists ? `${statuses.skills.count ?? 0} files` : 'Sync as a whole' },
  ];
  const selectedCount = rows.filter((row) => values[row.key]).length;

  return (
    <details className="section-card" open>
      <summary className="section-summary">
        <strong>{mode === 'share' ? 'share resources' : 'sync resources'}</strong>
        <span>{selectedCount}/{rows.length} selected</span>
      </summary>
      <div className="section-body">
        <div className="resource-list">
          {rows.map((row) => {
            const enabled = available ? available[row.key] : statuses ? statuses[row.key].exists : true;
            const detail = details?.[row.key];
            return (
              <div className={`resource-item ${!enabled ? 'disabled' : ''}`} key={row.key}>
                <label>
                  <input
                    type="checkbox"
                    disabled={!enabled}
                    checked={values[row.key] && enabled}
                    onChange={(event) => onChange({ ...values, [row.key]: event.target.checked })}
                  />
                  <span>{row.label}</span>
                  <small>{mode === 'sync' && row.key === 'skills' ? 'Replaces whole directory' : row.detail}</small>
                </label>
                {values[row.key] && enabled && detail && <div className="resource-detail">{detail}</div>}
              </div>
            );
          })}
        </div>
      </div>
    </details>
  );
}
