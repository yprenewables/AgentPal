import { useEffect, useState, type ReactNode } from 'react';
import { DirectoryPicker, Notice } from './components';
import { api, type CodexInspection, type RemoteManifest, type ShareStatus, type SyncResult } from './wails';

type Selection = { config: boolean; auth: boolean; skills: boolean };
type ConfigSelection = { rootKeys: string[]; tables: string[] };
type ResourceKey = keyof Selection;

const defaultSelection: Selection = { config: false, auth: false, skills: false };
const emptyConfigSelection: ConfigSelection = { rootKeys: [], tables: [] };
const resourceKeys: ResourceKey[] = ['config', 'auth', 'skills'];

function defaultConfigSelection(rootKeys: string[] = [], tables: string[] = []): ConfigSelection {
  return { rootKeys, tables: tables.filter((table) => !isMachineSpecificTable(table)) };
}

function isMachineSpecificTable(table: string) {
  return table === 'projects' || table.startsWith('projects.');
}

type ResourcePanelProps = {
  values: Selection;
  onChange(values: Selection): void;
  details: Partial<Record<ResourceKey, ReactNode>>;
  counts?: Partial<Record<ResourceKey, string>>;
  available?: Record<ResourceKey, boolean>;
  hideUnavailable?: boolean;
  expandedResource: ResourceKey | null;
  onExpandedResourceChange(resource: ResourceKey | null): void;
};

function firstSelectedResource(values: Selection): ResourceKey | null {
  return resourceKeys.find((key) => key !== 'config' && values[key]) ?? null;
}

function ResourcePanel({ values, onChange, details, counts, available, hideUnavailable = false, expandedResource, onExpandedResourceChange }: ResourcePanelProps) {
  function toggle(key: ResourceKey, checked: boolean) {
    const next = { ...values, [key]: checked };
    onChange(next);
    if (!checked && expandedResource === key) {
      onExpandedResourceChange(firstSelectedResource(next));
    }
  }

  function toggleExpanded(key: ResourceKey) {
    onExpandedResourceChange(expandedResource === key ? null : key);
  }

  return (
    <section className="resource-panel">
      <div className="resource-list" role="list" aria-label="Resources">
        {resourceKeys.map((key) => {
          const enabled = available ? available[key] : true;
          if (hideUnavailable && !enabled) return null;
          const selected = values[key];
          const expanded = expandedResource === key;
          const count = selected ? counts?.[key] ?? '1/1' : '0/1';
          return (
            <article className={`resource-card ${!enabled ? 'disabled' : ''}`} key={key} role="listitem">
              <div className="resource-summary-row" role="button" tabIndex={enabled ? 0 : -1} onClick={() => enabled && toggleExpanded(key)} onKeyDown={(event) => { if (!enabled) return; if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); toggleExpanded(key); } }}>
                <label className="resource-toggle" onClick={(event) => event.stopPropagation()}>
                  <input type="checkbox" disabled={!enabled} checked={selected && enabled} onChange={(event) => toggle(key, event.target.checked)} />
                  <div className="resource-copy">
                    <strong>{resourceLabel(key)}</strong>
                  </div>
                </label>
                <span className="selection-count">{count}</span>
                <span className={`resource-chevron ${expanded ? 'open' : ''}`}>⌄</span>
              </div>
              {expanded && selected && enabled ? <div className="resource-detail">{details[key] ?? <div className="detail-empty">No details available.</div>}</div> : null}
            </article>
          );
        })}
      </div>
    </section>
  );
}

function resourceLabel(key: ResourceKey) {
  switch (key) {
    case 'config':
      return 'config.toml';
    case 'auth':
      return 'auth.json';
    case 'skills':
      return 'skills';
  }
}

export default function App() {
  const [page, setPage] = useState<'share' | 'sync'>('share');

  return (
    <main>
      <header>
        <div>
          <p className="eyebrow">AgentPal</p>
          <h1>Codex sync</h1>
        </div>
        <nav aria-label="Primary sections">
          <button className={`nav-button ${page === 'share' ? 'active' : ''}`} onClick={() => setPage('share')}>Share</button>
          <button className={`nav-button ${page === 'sync' ? 'active' : ''}`} onClick={() => setPage('sync')}>Sync</button>
        </nav>
      </header>
      {!api && <Notice kind="warning">Backend bridge is unavailable. Run this inside Wails for full functionality.</Notice>}
      {page === 'share' ? <SharePage /> : <SyncPage />}
    </main>
  );
}

function SharePage() {
  const [dir, setDir] = useState('~/.codex');
  const [inspection, setInspection] = useState<CodexInspection | null>(null);
  const [selected, setSelected] = useState<Selection>(defaultSelection);
  const [configSelection, setConfigSelection] = useState<ConfigSelection>(emptyConfigSelection);
  const [skillSelection, setSkillSelection] = useState<string[]>([]);
  const [expandedResource, setExpandedResource] = useState<ResourceKey | null>(null);
  const [status, setStatus] = useState<ShareStatus | null>(null);
  const [notice, setNotice] = useState<{ kind: 'success' | 'warning' | 'error' | 'info'; text: string } | null>(null);

  useEffect(() => {
    if (!api) return;
    void api.GetDefaultCodexDir()
      .then((path) => {
        setDir(path);
        return inspect(path);
      })
      .catch(() => inspect(dir));
  }, []);

  async function inspect(path: string) {
    if (!api) return;
    try {
      const next = await api.InspectCodexDir(path);
      setInspection(next);
      setSelected({ config: next.config.exists, auth: false, skills: next.skills.exists });
      setConfigSelection(defaultConfigSelection(next.config.rootKeys, next.config.tables));
      setSkillSelection(next.skills.skills ?? []);
      setExpandedResource(null);
      setNotice(null);
    } catch (error) {
      setNotice({ kind: 'error', text: String(error) });
    }
  }

  async function browse() {
    if (!api) return;
    const path = await api.BrowseFolder();
    if (path) {
      setDir(path);
      await inspect(path);
    }
  }

  async function start() {
    if (!api) return;
    try {
      const next = await api.StartSharing({
        codexDir: dir,
        port: 28888,
        shareConfig: selected.config,
        shareAuth: selected.auth,
        shareSkills: selected.skills,
        configRootKeys: configSelection.rootKeys,
        configTables: configSelection.tables,
        skills: skillSelection,
      });
      setStatus(next);
      setNotice({ kind: 'success', text: 'Sharing is running.' });
    } catch (error) {
      setNotice({ kind: 'error', text: String(error) });
    }
  }

  async function stop() {
    if (!api) return;
    await api.StopSharing();
    setStatus(null);
    setNotice({ kind: 'info', text: 'Sharing stopped.' });
  }

  const nothingSelected = !selected.config && !selected.auth && !selected.skills;
  const invalidSkillsSelection = selected.skills && skillSelection.length === 0;
  const details = inspection
    ? {
        config: selected.config && inspection.config.exists ? (
          <ConfigSectionPicker
            rootKeys={inspection.config.rootKeys ?? []}
            tables={inspection.config.tables ?? []}
            selection={configSelection}
            onChange={setConfigSelection}
          />
        ) : undefined,
        auth: selected.auth && inspection.auth.exists ? <AuthNotice kind="warning" /> : undefined,
        skills: selected.skills && inspection.skills.exists ? (
          <SkillPicker skills={inspection.skills.skills ?? []} selected={skillSelection} onChange={setSkillSelection} />
        ) : undefined,
      }
    : {};
  const counts = inspection
    ? {
        config: `${configSelection.rootKeys.length + configSelection.tables.length}/${(inspection.config.rootKeys ?? []).length + (inspection.config.tables ?? []).length}`,
        skills: `${skillSelection.length}/${(inspection.skills.skills ?? []).length}`,
      }
    : {};

  return (
    <section className="panel">
      <DirectoryPicker label="Codex Directory" value={dir} onChange={setDir} onBrowse={browse} />
      {inspection && (
        <ResourcePanel
          values={selected}
          onChange={setSelected}
          details={details}
          counts={counts}
          available={{ config: inspection.config.exists, auth: inspection.auth.exists, skills: inspection.skills.exists }}
          expandedResource={expandedResource}
          onExpandedResourceChange={setExpandedResource}
        />
      )}
      {!inspection && <Notice kind="info">Pick a directory to load the available resources.</Notice>}
      {nothingSelected && <Notice kind="warning">Select at least one existing resource before opening sharing.</Notice>}
      {invalidSkillsSelection && <Notice kind="warning">Select at least one skill or turn off skills sharing.</Notice>}
      <div className="actions">
        <button className="primary" type="button" disabled={nothingSelected || invalidSkillsSelection} onClick={start}>Start</button>
        <button className="danger" type="button" onClick={stop}>Stop</button>
      </div>
      {status?.running && <Notice kind="success">Share this address: {status.localIPs.join(', ') || status.url}:28888</Notice>}
      {notice && <Notice kind={notice.kind}>{notice.text}</Notice>}
    </section>
  );
}

function SyncPage() {
  const [peerIP, setPeerIP] = useState('');
  const [targetDir, setTargetDir] = useState('~/.codex');
  const [manifest, setManifest] = useState<RemoteManifest | null>(null);
  const [selected, setSelected] = useState<Selection>(defaultSelection);
  const [configSelection, setConfigSelection] = useState<ConfigSelection>(emptyConfigSelection);
  const [skillSelection, setSkillSelection] = useState<string[]>([]);
  const [expandedResource, setExpandedResource] = useState<ResourceKey | null>(null);
  const [result, setResult] = useState<SyncResult | null>(null);
  const [backupExamplePath, setBackupExamplePath] = useState('~/.agentpal/backups/YYYYMMDD-HHMMSS');
  const [notice, setNotice] = useState<{ kind: 'success' | 'warning' | 'error' | 'info'; text: string } | null>(null);

  useEffect(() => {
    if (!api) return;
    void api.GetDefaultCodexDir().then(setTargetDir).catch(() => setTargetDir('~/.codex'));
    void api.GetBackupExamplePath().then(setBackupExamplePath).catch(() => setBackupExamplePath('~/.agentpal/backups/YYYYMMDD-HHMMSS'));
  }, []);

  async function testConnection() {
    if (!api) return;
    try {
      const status = await api.TestConnection(peerIP);
      const next = await api.FetchRemoteManifest(peerIP);
      setManifest(next);
      setSelected({ config: next.shared.config.enabled, auth: next.shared.auth.enabled, skills: next.shared.skills.enabled });
      setConfigSelection(defaultConfigSelection(next.shared.config.rootKeys, next.shared.config.tables));
      setSkillSelection(next.shared.skills.skills ?? []);
      setExpandedResource(null);
      setNotice({ kind: 'success', text: `Connected to ${status.url} running ${status.version}.` });
    } catch (error) {
      setNotice({ kind: 'error', text: String(error) });
    }
  }

  async function browse() {
    if (!api) return;
    const path = await api.BrowseFolder();
    if (path) setTargetDir(path);
  }

  async function syncNow() {
    if (!api) return;
    try {
      const next = await api.SyncFromPeer({
        peerIP,
        port: 28888,
        targetDir,
        syncConfig: selected.config,
        syncAuth: selected.auth,
        syncSkills: selected.skills,
        configRootKeys: configSelection.rootKeys,
        configTables: configSelection.tables,
        skills: skillSelection,
      });
      setResult(next);
      setNotice({ kind: next.ok ? 'success' : 'error', text: next.ok ? 'Sync completed.' : 'Sync failed.' });
    } catch (error) {
      setNotice({ kind: 'error', text: String(error) });
    }
  }

  const available = manifest ? { config: manifest.shared.config.enabled, auth: manifest.shared.auth.enabled, skills: manifest.shared.skills.enabled } : undefined;
  const nothingSelected = !selected.config && !selected.auth && !selected.skills;
  const invalidSkillsSelection = selected.skills && skillSelection.length === 0;
  const details = manifest
    ? {
        config: selected.config && manifest.shared.config.enabled ? (
          <ConfigSectionPicker
            rootKeys={manifest.shared.config.rootKeys ?? []}
            tables={manifest.shared.config.tables ?? []}
            selection={configSelection}
            onChange={setConfigSelection}
          />
        ) : undefined,
        auth: selected.auth && manifest.shared.auth.enabled ? <AuthNotice kind="warning" /> : undefined,
        skills: selected.skills && manifest.shared.skills.enabled ? (
          <SkillPicker skills={manifest.shared.skills.skills ?? []} selected={skillSelection} onChange={setSkillSelection} />
        ) : undefined,
      }
    : {};
  const counts = manifest
    ? {
        config: `${configSelection.rootKeys.length + configSelection.tables.length}/${(manifest.shared.config.rootKeys ?? []).length + (manifest.shared.config.tables ?? []).length}`,
        skills: `${skillSelection.length}/${(manifest.shared.skills.skills ?? []).length}`,
      }
    : {};

  return (
    <section className="panel">
      <div className="peer-row">
        <label className="field compact">
          <span>Peer IP</span>
          <input value={peerIP} placeholder="192.168.1.23" onChange={(event) => setPeerIP(event.target.value)} />
        </label>
        <button className="secondary nowrap" type="button" onClick={testConnection}>Connect</button>
      </div>
      {notice && <Notice kind={notice.kind}>{notice.text}</Notice>}
      {manifest && (
        <ResourcePanel
          values={selected}
          onChange={setSelected}
          details={details}
          counts={counts}
          available={available}
          hideUnavailable
          expandedResource={expandedResource}
          onExpandedResourceChange={setExpandedResource}
        />
      )}
      <div className="sync-bottom">
        <DirectoryPicker label="Target Codex Directory" value={targetDir} onChange={setTargetDir} onBrowse={browse} />
        <Notice kind="warning">Backup first. `skills` replaces only selected skill folders. Backup: <code>{backupExamplePath}</code></Notice>
      </div>
      {invalidSkillsSelection && <Notice kind="warning">Select at least one skill or turn off skills sync.</Notice>}
      <div className="actions">
        <button className="primary" type="button" disabled={!manifest || nothingSelected || invalidSkillsSelection} onClick={syncNow}>Sync</button>
      </div>
      {result && (
        <div className="result-card">
          <strong>{result.ok ? 'Sync completed' : 'Sync partial'}</strong>
          <span>{result.items.join(', ')}</span>
          <code>{result.backupPath || 'No backup needed'}</code>
        </div>
      )}
    </section>
  );
}

function AuthNotice({ kind }: { kind: 'warning' }) {
  return <Notice kind={kind}>auth.json may contain credentials. Share it only with trusted machines.</Notice>;
}

type ConfigSectionPickerProps = {
  rootKeys: string[];
  tables: string[];
  selection: ConfigSelection;
  onChange(selection: ConfigSelection): void;
};

function ConfigSectionPicker({ rootKeys, tables, selection, onChange }: ConfigSectionPickerProps) {
  function toggleRootKey(value: string) {
    const current = selection.rootKeys;
    const next = current.includes(value) ? current.filter((item) => item !== value) : [...current, value];
    onChange({ ...selection, rootKeys: next });
  }

  return (
    <div className="inline-section-body">
      <ConfigRootKeyGroup items={rootKeys} selected={selection.rootKeys} onToggle={toggleRootKey} onAll={() => onChange({ ...selection, rootKeys })} />
      <ConfigTableTree tables={tables} selected={selection.tables} onChange={(nextTables) => onChange({ ...selection, tables: nextTables })} />
    </div>
  );
}

type ConfigRootKeyGroupProps = {
  items: string[];
  selected: string[];
  onToggle(value: string): void;
  onAll(): void;
};

function ConfigRootKeyGroup({ items, selected, onToggle, onAll }: ConfigRootKeyGroupProps) {
  return (
    <div className="section-group">
      {items.length === 0 ? (
        <small>No root keys.</small>
      ) : (
        <div className="section-options">
          {items.map((item) => (
            <label key={item}>
              <input type="checkbox" checked={selected.includes(item)} onChange={() => onToggle(item)} />
              <span>{item}</span>
            </label>
          ))}
        </div>
      )}
    </div>
  );
}

type TableNode = {
  name: string;
  label: string;
  fullName: string;
  selectable: boolean;
  children: TableNode[];
};

type ConfigTableTreeProps = {
  tables: string[];
  selected: string[];
  onChange(tables: string[]): void;
};

function ConfigTableTree({ tables, selected, onChange }: ConfigTableTreeProps) {
  const tree = buildTableTree(tables);

  function toggleNode(node: TableNode) {
    const descendants = selectableDescendants(node);
    const selectedSet = new Set(selected);
    const allSelected = descendants.length > 0 && descendants.every((table) => selectedSet.has(table));
    for (const table of descendants) {
      if (allSelected) {
        selectedSet.delete(table);
      } else {
        selectedSet.add(table);
      }
    }
    onChange(tables.filter((table) => selectedSet.has(table)));
  }

  return (
    <div className="section-group">
      {tree.length === 0 ? <small>No tables.</small> : <div className="table-tree">{tree.map((node) => <TableTreeNode key={node.fullName} node={node} selected={selected} onToggle={toggleNode} />)}</div>}
    </div>
  );
}

type TableTreeNodeProps = {
  node: TableNode;
  selected: string[];
  onToggle(node: TableNode): void;
};

function TableTreeNode({ node, selected, onToggle }: TableTreeNodeProps) {
  const descendants = selectableDescendants(node);
  const checked = descendants.length > 0 && descendants.every((table) => selected.includes(table));
  const partial = !checked && descendants.some((table) => selected.includes(table));

  return (
    <div className="table-node">
      <label className={partial ? 'partial' : ''}>
        <input type="checkbox" checked={checked} onChange={() => onToggle(node)} />
        <span>{node.label}</span>
      </label>
      {node.children.length > 0 && <div className="table-children">{node.children.map((child) => <TableTreeNode key={child.fullName} node={child} selected={selected} onToggle={onToggle} />)}</div>}
    </div>
  );
}

function buildTableTree(tables: string[]) {
  const roots: TableNode[] = [];
  const byFullName = new Map<string, TableNode>();

  for (const table of tables) {
    const parts = splitTableName(table);
    let parentChildren = roots;
    let fullName = '';
    for (const part of parts) {
      fullName = fullName ? `${fullName}.${part}` : part;
      let node = byFullName.get(fullName);
      if (!node) {
        node = { name: part, label: trimTableLabel(part), fullName, selectable: tables.includes(fullName), children: [] };
        byFullName.set(fullName, node);
        parentChildren.push(node);
      }
      parentChildren = node.children;
    }
    const leaf = byFullName.get(table);
    if (leaf) leaf.selectable = true;
  }
  return roots;
}

function selectableDescendants(node: TableNode): string[] {
  const own = node.selectable ? [node.fullName] : [];
  return [...own, ...node.children.flatMap(selectableDescendants)];
}

function splitTableName(table: string) {
  const parts: string[] = [];
  let current = '';
  let quote = '';
  for (const char of table) {
    if ((char === '"' || char === "'") && quote === '') {
      quote = char;
    } else if (char === quote) {
      quote = '';
    }
    if (char === '.' && quote === '') {
      parts.push(current);
      current = '';
    } else {
      current += char;
    }
  }
  if (current) parts.push(current);
  return parts;
}

function trimTableLabel(label: string) {
  return label.replace(/^['"]|['"]$/g, '');
}

type SkillPickerProps = {
  skills: string[];
  selected: string[];
  onChange(skills: string[]): void;
};

function SkillPicker({ skills, selected, onChange }: SkillPickerProps) {
  function toggle(skill: string) {
    onChange(selected.includes(skill) ? selected.filter((item) => item !== skill) : [...selected, skill]);
  }

  return (
    <div className="inline-section-body">
      {skills.length === 0 ? (
        <small>No skills.</small>
      ) : (
        <div className="section-options">
          {skills.map((skill) => (
            <label key={skill}>
              <input type="checkbox" checked={selected.includes(skill)} onChange={() => toggle(skill)} />
              <span>{skill}</span>
            </label>
          ))}
        </div>
      )}
    </div>
  );
}
