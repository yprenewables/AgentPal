import { useEffect, useState } from 'react';
import { DirectoryPicker, Notice, ResourceChecklist } from './components';
import { api, type CodexInspection, type RemoteManifest, type ShareStatus, type SyncResult } from './wails';

type Selection = { config: boolean; auth: boolean; skills: boolean };
type ConfigSelection = { rootKeys: string[]; tables: string[] };

const defaultSelection: Selection = { config: false, auth: false, skills: false };
const emptyConfigSelection: ConfigSelection = { rootKeys: [], tables: [] };

export default function App() {
  const [page, setPage] = useState<'share' | 'sync'>('share');

  return (
    <main>
      <header>
        <div>
          <p className="eyebrow">AgentPal</p>
          <h1>Codex local sync</h1>
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
      setConfigSelection({ rootKeys: next.config.rootKeys ?? [], tables: next.config.tables ?? [] });
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

  return (
    <section className="panel">
      <h2>Share Codex Configuration</h2>
      <DirectoryPicker label="Codex Directory" value={dir} onChange={setDir} onBrowse={browse} />
      <button className="secondary" type="button" onClick={() => inspect(dir)}>Inspect Directory</button>
      {inspection && <ResourceChecklist mode="share" values={selected} onChange={setSelected} statuses={inspection} />}
      {selected.config && inspection?.config.exists && (
        <ConfigSectionPicker
          rootKeys={inspection.config.rootKeys ?? []}
          tables={inspection.config.tables ?? []}
          selection={configSelection}
          onChange={setConfigSelection}
        />
      )}
      {selected.auth && <Notice kind="warning">auth.json may contain credentials. Share it only with trusted machines.</Notice>}
      {nothingSelected && <Notice kind="warning">Select at least one existing resource before opening sharing.</Notice>}
      <div className="actions">
        <button className="primary" type="button" disabled={nothingSelected} onClick={start}>Open Sharing</button>
        <button className="danger" type="button" onClick={stop}>Stop Sharing</button>
      </div>
      {status?.running && <Notice kind="success">Tell the other person to enter one of these IPs: {status.localIPs.join(', ') || status.url}. Port 28888.</Notice>}
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
      setConfigSelection({ rootKeys: next.shared.config.rootKeys ?? [], tables: next.shared.config.tables ?? [] });
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
      });
      setResult(next);
      setNotice({ kind: next.ok ? 'success' : 'error', text: next.ok ? 'Sync completed.' : 'Sync partially failed.' });
    } catch (error) {
      setNotice({ kind: 'error', text: String(error) });
    }
  }

  const available = manifest ? { config: manifest.shared.config.enabled, auth: manifest.shared.auth.enabled, skills: manifest.shared.skills.enabled } : undefined;
  const nothingSelected = !selected.config && !selected.auth && !selected.skills;

  return (
    <section className="panel">
      <h2>Connect To AgentPal</h2>
      <div className="peer-row">
        <label className="field compact">
          <span>Peer IP</span>
          <input value={peerIP} placeholder="192.168.1.23" onChange={(event) => setPeerIP(event.target.value)} />
        </label>
        <button className="secondary nowrap" type="button" onClick={testConnection}>Connect</button>
      </div>
      {notice && <Notice kind={notice.kind}>{notice.text}</Notice>}
      {manifest && <ResourceChecklist mode="sync" values={selected} onChange={setSelected} available={available} />}
      {selected.config && manifest?.shared.config.enabled && (
        <ConfigSectionPicker
          rootKeys={manifest.shared.config.rootKeys ?? []}
          tables={manifest.shared.config.tables ?? []}
          selection={configSelection}
          onChange={setConfigSelection}
        />
      )}
      <div className="sync-bottom">
        <DirectoryPicker label="Target Codex Directory" value={targetDir} onChange={setTargetDir} onBrowse={browse} />
        <Notice kind="warning">
          Backed up, then replaced. skills replaces the whole directory.
          <br />
          Backup: <code>{backupExamplePath}</code>
        </Notice>
      </div>
      {selected.auth && <Notice kind="warning">auth.json is sensitive and may replace credentials.</Notice>}
      <div className="actions">
        <button className="primary" type="button" disabled={!manifest || nothingSelected} onClick={syncNow}>Sync</button>
      </div>
      {result && (
        <div className="result-card">
          <strong>{result.ok ? 'Sync completed' : 'Sync partially failed'}</strong>
          <span>Items: {result.items.join(', ')}</span>
          <span>Backup path</span>
          <code>{result.backupPath || 'No existing files were backed up'}</code>
        </div>
      )}
    </section>
  );
}

type ConfigSectionPickerProps = {
  rootKeys: string[];
  tables: string[];
  selection: ConfigSelection;
  onChange(selection: ConfigSelection): void;
};

function ConfigSectionPicker({ rootKeys, tables, selection, onChange }: ConfigSectionPickerProps) {
  function toggle(kind: keyof ConfigSelection, value: string) {
    const current = selection[kind];
    const next = current.includes(value) ? current.filter((item) => item !== value) : [...current, value];
    onChange({ ...selection, [kind]: next });
  }

  function setAll(kind: keyof ConfigSelection, values: string[]) {
    onChange({ ...selection, [kind]: values });
  }

  return (
    <div className="config-sections">
      <div className="section-head">
        <strong>config.toml sections</strong>
        <span>Select root key-values and tables to sync.</span>
      </div>
      <ConfigSectionGroup title="Root K-V" items={rootKeys} selected={selection.rootKeys} onToggle={(value) => toggle('rootKeys', value)} onAll={() => setAll('rootKeys', rootKeys)} />
      <ConfigSectionGroup title="Tables" items={tables} selected={selection.tables} onToggle={(value) => toggle('tables', value)} onAll={() => setAll('tables', tables)} />
    </div>
  );
}

type ConfigSectionGroupProps = {
  title: string;
  items: string[];
  selected: string[];
  onToggle(value: string): void;
  onAll(): void;
};

function ConfigSectionGroup({ title, items, selected, onToggle, onAll }: ConfigSectionGroupProps) {
  return (
    <div className="section-group">
      <div className="section-title">
        <span>{title}</span>
        <button className="link-button" type="button" onClick={onAll}>Select all</button>
      </div>
      {items.length === 0 ? (
        <small>No items found.</small>
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
