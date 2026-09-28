import { useEffect, useState, type ReactNode } from 'react';
import { api, type ApplyChange, type AuthProfile, type BackupInfo, type Bootstrap, type ConfigRefresh, type CurrentAuthStatus, type SettingsRequest } from './wails';

type Tab = 'configuration' | 'authentication' | 'backups';

export default function App() {
  const [bootstrap, setBootstrap] = useState<Bootstrap | null>(null);
  const [error, setError] = useState('');
  useEffect(() => { void api?.GetBootstrap().then(setBootstrap).catch((reason) => setError(String(reason))); }, []);
  if (!api) return <main className="shell"><Notice kind="warning">Backend bridge is unavailable. Run AgentPal inside Wails.</Notice></main>;
  if (error) return <main className="shell"><Notice kind="error">{error}</Notice></main>;
  if (!bootstrap) return <main className="shell loading">Loading AgentPal...</main>;
  return bootstrap.mode === 'server' ? <ServerApp bootstrap={bootstrap} /> : <ClientApp bootstrap={bootstrap} />;
}

function ClientApp({ bootstrap }: { bootstrap: Bootstrap }) {
  const [tab, setTab] = useState<Tab>('configuration');
  const [refresh, setRefresh] = useState<ConfigRefresh | null>(null);
  const [profiles, setProfiles] = useState<AuthProfile[]>([]);
  const [menuOpen, setMenuOpen] = useState(false);
  const [showSettings, setShowSettings] = useState(false);
  const [notice, setNotice] = useState<{ kind: NoticeKind; text: string } | null>(null);
  const [version, setVersion] = useState<string>(`v${bootstrap.version}`);

  async function loadConfiguration() {
    if (!api) return;
    const next = await api.RefreshConfiguration(); setRefresh(next);
    if (!next.connected) setNotice({ kind: 'warning', text: next.error || 'Central node is offline.' });
    else setNotice(null);
  }
  async function loadProfiles() { if (api) setProfiles(await api.ListAuthProfiles()); }
  useEffect(() => { void loadConfiguration(); void loadProfiles(); void api?.CheckForUpdate().then((info) => setVersion(info.hasUpdate ? `a newer version discovered · update` : `v${info.currentVersion} (latest)`)).catch(() => undefined); }, []);

  return <main className="shell">
    <header className="topbar"><div><div className="brand">AgentPal</div><small>Codex configuration client</small></div><div className="top-actions"><span className={`connection-dot ${refresh?.connected ? 'online' : ''}`}>● {refresh?.connected ? 'Central connected' : 'Central offline'}</span><button className="icon-button" aria-label="Open menu" onClick={() => setMenuOpen(!menuOpen)}>•••</button>{menuOpen && <Menu bootstrap={bootstrap} onRefresh={loadConfiguration} onSettings={() => setShowSettings(true)} onClose={() => setMenuOpen(false)} />}</div></header>
    <section className="node-strip"><strong>{bootstrap.centralHost}:{bootstrap.centralPort}</strong><span>Central configuration node</span><button className="text-button" onClick={() => void loadConfiguration()}>Refresh</button></section>
    <nav className="tabs" aria-label="AgentPal sections">{(['configuration', 'authentication', 'backups'] as Tab[]).map((item) => <button key={item} className={tab === item ? 'tab active' : 'tab'} onClick={() => { setTab(item); if (item === 'authentication') void loadProfiles(); }}>{labelForTab(item)}</button>)}</nav>
    {notice && <Notice kind={notice.kind}>{notice.text}</Notice>}
    {tab === 'configuration' && <ConfigurationTab refresh={refresh} onRefresh={loadConfiguration} setNotice={setNotice} />}
    {tab === 'authentication' && <AuthenticationTabCompact profiles={profiles} reload={loadProfiles} setNotice={setNotice} />}
    {tab === 'backups' && <BackupsTab setNotice={setNotice} />}
    {showSettings && <SettingsDialog bootstrap={bootstrap} onClose={() => setShowSettings(false)} setNotice={setNotice} />}
    <div className="version-status">{version}</div>
  </main>;
}

function ConfigurationTab({ refresh, onRefresh, setNotice }: { refresh: ConfigRefresh | null; onRefresh: () => Promise<void>; setNotice: (value: { kind: NoticeKind; text: string } | null) => void }) {
  const [selected, setSelected] = useState<Record<string, boolean>>({});
  const [applying, setApplying] = useState(false);
  const items = refresh?.diff?.items ?? [];
  useEffect(() => { setSelected(Object.fromEntries(items.map((item) => [`${item.kind}:${item.name}`, item.selected]))); }, [refresh]);
  if (!refresh?.connected) return <section className="workspace empty-state"><div className="status-symbol">!</div><h1>Connect to the central node</h1><p>AgentPal could not load the latest Skills and MCP configuration.</p><button className="primary" onClick={() => void onRefresh()}>Retry connection</button></section>;
  const changes = items.filter((item) => item.status !== 'same');
  async function apply() { if (!api || !changes.length) return; setApplying(true); const selectedChanges: ApplyChange[] = changes.filter((item) => selected[`${item.kind}:${item.name}`]).map((item) => ({ kind: item.kind, name: item.name, action: item.status === 'local_only' ? 'delete' : 'apply' })); try { const result = await api.ApplyConfiguration({ changes: selectedChanges }); setNotice({ kind: result.ok ? 'success' : 'error', text: result.ok ? `Configuration applied. Backup: ${result.backupPath}` : `Configuration rolled back. Backup: ${result.backupPath}` }); await onRefresh(); } catch (error) { setNotice({ kind: 'error', text: String(error) }); } finally { setApplying(false); } }
  return <section className="workspace"><div className="section-heading"><div><h1>Available updates</h1><p>Review central changes before applying them to this Mac.</p></div><span className={changes.length ? 'badge attention' : 'badge'}>{changes.length} updates</span></div><div className="resource-list">{items.map((item) => <label className="resource-row" key={`${item.kind}:${item.name}`}><input type="checkbox" checked={!!selected[`${item.kind}:${item.name}`]} onChange={(event) => setSelected({ ...selected, [`${item.kind}:${item.name}`]: event.target.checked })} /><span><strong>{item.name}</strong><small>{item.kind === 'skill' ? 'Skill' : 'MCP server'}</small></span><em className={`diff-${item.status}`}>{statusLabel(item.status)}</em></label>)}</div><button className="primary" disabled={applying || !changes.some((item) => selected[`${item.kind}:${item.name}`])} onClick={() => void apply()}>{applying ? 'Applying...' : 'Review and apply'}</button></section>;
}

function AuthenticationTab({ profiles, reload, setNotice }: { profiles: AuthProfile[]; reload: () => Promise<void>; setNotice: (value: { kind: NoticeKind; text: string } | null) => void }) {
  const [showKey, setShowKey] = useState(false); const [name, setName] = useState(''); const [provider, setProvider] = useState('openai'); const [model, setModel] = useState(''); const [baseUrl, setBaseUrl] = useState('https://api.openai.com/v1'); const [key, setKey] = useState(''); const [current, setCurrent] = useState<{ apiKeyPreview?: string; baseUrl?: string; chatgptUserId?: string; chatgptExpiresAt?: string; apiKeyImported: boolean; chatgptImported: boolean } | null>(null);
  useEffect(() => { void api?.GetCurrentAuthStatus().then(setCurrent).catch(() => setCurrent(null)); }, [profiles]);
  async function create() { if (!api) return; try { await api.CreateAPIKeyProfile({ name, provider, baseUrl, apiKey: key }); setShowKey(false); setKey(''); await reload(); setNotice({ kind: 'success', text: 'API Key profile created.' }); } catch (error) { setNotice({ kind: 'error', text: String(error) }); } }
  async function activate(id: string) { if (!api) return; try { const result = await api.ActivateAuthProfile(id); setNotice({ kind: 'success', text: `Profile activated. Backup: ${result.backupPath}` }); await reload(); } catch (error) { setNotice({ kind: 'error', text: String(error) }); } }
  async function importAPIKey() { if (!api) return; try { await api.ImportCurrentAPIKey('Current API Key'); await reload(); setNotice({ kind: 'success', text: 'Current API Key imported.' }); } catch (error) { setNotice({ kind: 'error', text: String(error) }); } }
  async function importChatGPT() { if (!api) return; try { await api.ImportCurrentAuth('Current ChatGPT login'); await reload(); setNotice({ kind: 'success', text: 'Current ChatGPT login imported.' }); } catch (error) { setNotice({ kind: 'error', text: String(error) }); } }
  const showCurrentPreview = !!current && !((current.apiKeyImported && current.apiKeyPreview) || (current.chatgptImported && current.chatgptUserId));
  return <section className="workspace"><div className="section-heading"><div><h1>Authentication profiles</h1><p>Switch local ChatGPT logins and API Key configurations without replacing MCP or Projects.</p></div><div className="button-row"><button className="primary" onClick={() => setShowKey(true)}>New API Key</button></div></div>{showCurrentPreview && <div className="current-auth"><div className="current-auth-line">{current?.apiKeyPreview ? <><strong>API KEY: {current.apiKeyPreview}</strong><span>{current.baseUrl || 'base_url not detected'}</span><button className="secondary" onClick={() => void importAPIKey()}>Import</button></> : current?.chatgptUserId ? <><strong>ChatGPT User ID: {current.chatgptUserId}</strong><span>expire at: {current.chatgptExpiresAt || 'unknown'}</span><button className="secondary" onClick={() => void importChatGPT()}>Import</button></> : <strong>Authentication: Not detected</strong>}</div></div>}<div className="profile-list">{profiles.length === 0 && <div className="empty-inline">No local profiles yet.</div>}{profiles.map((profile) => <article className={`profile-row ${profile.active ? 'active' : ''}`} key={profile.id}><div><strong>{profile.name}</strong><small>{profile.type} · {profile.provider} · {profile.model}{profile.apiKeyPreview ? ` · ${profile.apiKeyPreview} · ${profile.baseUrl || 'no base_url'}` : profile.chatgptUserId ? ` · user_id ${profile.chatgptUserId}` : ''}</small></div><div className="profile-actions">{profile.active && <span className="badge success-badge">Current</span>}{!profile.active && <button className="secondary" onClick={() => void activate(profile.id)}>Switch</button>}</div></article>)}</div>{showKey && <div className="dialog-backdrop"><div className="dialog"><div className="section-heading"><h2>New API Key profile</h2><button className="icon-button" onClick={() => setShowKey(false)}>×</button></div><label className="field"><span>Name</span><input value={name} onChange={(event) => setName(event.target.value)} /></label><label className="field"><span>Provider</span><input value={provider} onChange={(event) => setProvider(event.target.value)} /></label><label className="field"><span>Model</span><input value={model} onChange={(event) => setModel(event.target.value)} /></label><label className="field"><span>Base URL</span><input value={baseUrl} onChange={(event) => setBaseUrl(event.target.value)} /></label><label className="field"><span>API Key</span><input type="password" value={key} onChange={(event) => setKey(event.target.value)} /></label><button className="primary" onClick={() => void create()}>Create profile</button></div></div>}</section>;
}

function AuthenticationTabCompact({ profiles, reload, setNotice }: { profiles: AuthProfile[]; reload: () => Promise<void>; setNotice: (value: { kind: NoticeKind; text: string } | null) => void }) {
  const [current, setCurrent] = useState<CurrentAuthStatus | null>(null);
  const [showKey, setShowKey] = useState(false);
  const [name, setName] = useState('');
  const [provider, setProvider] = useState('openai');
  const [model, setModel] = useState('');
  const [baseUrl, setBaseUrl] = useState('https://api.openai.com/v1');
  const [key, setKey] = useState('');
  const [confirmAction, setConfirmAction] = useState<{ kind: 'switch' | 'delete'; profile?: AuthProfile; description?: string } | null>(null);
  const [login, setLogin] = useState<{ loggedIn: boolean; subject?: string }>({ loggedIn: false });
  const [syncItems, setSyncItems] = useState<{ name: string; status: string; detail?: string }[]>([]);
  const [loginBusy, setLoginBusy] = useState(false);

  useEffect(() => { void api?.GetCentralLoginState().then((state) => setLogin({ loggedIn: state.loggedIn, subject: state.subject })).catch(() => setLogin({ loggedIn: false })); }, []);

  async function runSync() {
    if (!api) return;
    try {
      const report = await api.SyncCentralAuthProfiles();
      setSyncItems(report.items);
      await reload();
      setNotice({ kind: 'success', text: `Central profiles checked: ${report.items.length}.` });
    } catch (error) {
      setNotice({ kind: 'error', text: String(error) });
    }
  }

  async function startLogin() {
    if (!api || loginBusy) return;
    setLoginBusy(true);
    try {
      const start = await api.StartCentralLogin();
      const deadline = new Date(start.expiresAt).getTime();
      while (Date.now() < deadline) {
        await new Promise((resolve) => setTimeout(resolve, 2000));
        let status;
        try {
          status = await api.PollCentralLogin(start.loginId, start.pollToken);
        } catch {
          break;
        }
        if (status.status === 'approved') {
          setLogin({ loggedIn: true, subject: status.subject });
          setNotice({ kind: 'success', text: 'Logged in with Casdoor.' });
          await runSync();
          return;
        }
        if (status.status === 'failed') {
          setNotice({ kind: 'error', text: status.error || 'Central login failed.' });
          return;
        }
      }
      setNotice({ kind: 'warning', text: 'Central login was not completed.' });
    } catch (error) {
      setNotice({ kind: 'error', text: String(error) });
    } finally {
      setLoginBusy(false);
    }
  }

  async function logout() {
    if (!api) return;
    await api.LogoutCentralLogin();
    setLogin({ loggedIn: false });
    setSyncItems([]);
  }

  async function overwrite(name: string) {
    if (!api) return;
    try {
      await api.ApplyCentralAuthProfile(name);
      await reload();
      await runSync();
      setNotice({ kind: 'success', text: `Overwrote local profile ${name} from central.` });
    } catch (error) {
      setNotice({ kind: 'error', text: String(error) });
    }
  }

  useEffect(() => { void api?.GetCurrentAuthStatus().then(setCurrent).catch(() => setCurrent(null)); }, [profiles]);

  async function importCurrentAPIKey() {
    if (!api) return;
    try { await api.ImportCurrentAPIKey('Current API Key'); await reload(); setNotice({ kind: 'success', text: 'Current API Key imported.' }); } catch (error) { setNotice({ kind: 'error', text: String(error) }); }
  }

  async function importCurrentChatGPT() {
    if (!api) return;
    try { await api.ImportCurrentAuth('Current ChatGPT login'); await reload(); setNotice({ kind: 'success', text: 'Current ChatGPT login imported.' }); } catch (error) { setNotice({ kind: 'error', text: String(error) }); }
  }

  async function createAPIKeyProfile() {
    if (!api) return;
    try { await api.CreateAPIKeyProfile({ name, provider, baseUrl, apiKey: key }); setShowKey(false); setKey(''); await reload(); setNotice({ kind: 'success', text: 'API Key profile created.' }); } catch (error) { setNotice({ kind: 'error', text: String(error) }); }
  }

  async function activate(id: string) {
    if (!api) { setNotice({ kind: 'error', text: 'AgentPal backend is not available. Restart the desktop client.' }); return; }
    setNotice({ kind: 'info', text: 'Switching authentication profile...' });
    try {
      const preview = await api.PreviewAuthProfile(id);
      if (preview.providerConflict) {
        setConfirmAction({ kind: 'switch', profile: profiles.find((profile) => profile.id === id), description: preview.conflictDescription });
        return;
      }
      const result = await api.ActivateAuthProfileWithConflict(id, false);
      if (result.providerConflict) { setNotice({ kind: 'warning', text: result.conflictDescription || 'Provider configuration conflict. Nothing was changed.' }); return; }
      setNotice({ kind: 'success', text: `Profile activated. Backup: ${result.backupPath}` }); await reload();
    } catch (error) { setNotice({ kind: 'error', text: String(error) }); }
  }

  async function removeProfile(profile: AuthProfile) {
    if (profile.active) { setNotice({ kind: 'warning', text: 'Switch to another profile before deleting the current profile.' }); return; }
    if (!api) { setNotice({ kind: 'error', text: 'AgentPal backend is not available. Restart the desktop client.' }); return; }
    setConfirmAction({ kind: 'delete', profile });
  }

  async function confirmActionNow() {
    if (!api || !confirmAction) return;
    const action = confirmAction;
    setConfirmAction(null);
    setNotice({ kind: 'info', text: action.kind === 'delete' ? 'Deleting authentication profile...' : 'Switching authentication profile...' });
    try {
      if (action.kind === 'delete' && action.profile) {
        await api.DeleteAuthProfile(action.profile.id);
        await reload();
        setNotice({ kind: 'success', text: `Deleted local profile ${action.profile.name}.` });
      } else if (action.kind === 'switch' && action.profile) {
        const result = await api.ActivateAuthProfileWithConflict(action.profile.id, true);
        if (result.providerConflict) { setNotice({ kind: 'warning', text: result.conflictDescription || 'Provider configuration conflict. Nothing was changed.' }); return; }
        setNotice({ kind: 'success', text: `Profile activated. Backup: ${result.backupPath}` });
        await reload();
      }
    } catch (error) {
      setNotice({ kind: 'error', text: String(error) });
    }
  }

  const hasCurrentProfile = profiles.some((profile) => profile.active);
  return <section className="workspace">
    <div className="section-heading">
      <div><h1>Authentication</h1><p>Choose one local API Key or ChatGPT profile.</p></div>
      <button className="primary" onClick={() => setShowKey(true)}>New API Key</button>
    </div>
    <div className="current-auth">
      <div className="current-auth-line">
        {login.loggedIn
          ? <><strong>Casdoor: {login.subject}</strong><button className="secondary" onClick={() => void runSync()}>Sync central profiles</button><button className="secondary" onClick={() => void logout()}>Logout</button></>
          : <><strong>Central login</strong><button className="primary" onClick={() => void startLogin()} disabled={loginBusy}>{loginBusy ? 'Waiting for login...' : 'Login with Casdoor'}</button></>}
      </div>
    </div>
    {syncItems.length > 0 && <div className="profile-list">{syncItems.map((item) => <article className="profile-row" key={item.name}>
      <div className="profile-main"><strong>{item.name}</strong><small>{item.status}{item.detail ? ` · ${item.detail}` : ''}</small></div>
      <div className="profile-actions">
        {item.status === 'needs_sync' && <button className="primary" onClick={() => void overwrite(item.name)}>Overwrite local</button>}
        {item.status === 'needs_sync' && <button className="secondary" onClick={() => setSyncItems(syncItems.filter((entry) => entry.name !== item.name))}>Keep local</button>}
      </div>
    </article>)}</div>}
    {!hasCurrentProfile && current?.apiKeyPreview && <div className="current-auth"><div className="current-auth-line"><strong>API KEY: {current.apiKeyPreview}</strong><span>{current.baseUrl || 'base_url not detected'}</span><button className="secondary" onClick={() => void importCurrentAPIKey()}>Import</button></div></div>}
    {!hasCurrentProfile && !current?.apiKeyPreview && current?.chatgptUserId && <div className="current-auth"><div className="current-auth-line"><strong>ChatGPT User ID: {current.chatgptUserId}</strong><span>expire at: {current.chatgptExpiresAt || 'unknown'}</span><button className="secondary" onClick={() => void importCurrentChatGPT()}>Import</button></div></div>}
    <div className="profile-list">
      {profiles.length === 0 && <div className="empty-inline">No local profiles yet.</div>}
      {profiles.map((profile) => <article className={`profile-row ${profile.active ? 'active' : ''}`} key={profile.id}>
        <div className="profile-main"><strong>{profileTitle(profile)}</strong><small>{profileSummary(profile)}</small></div>
        <div className="profile-actions">{profile.active ? <span className="badge success-badge">Current</span> : <button className="secondary" onClick={() => void activate(profile.id)}>Select</button>}<button className="danger" disabled={profile.active} title={profile.active ? 'Switch to another profile before deleting this one.' : 'Delete this local profile.'} onClick={() => void removeProfile(profile)}>Delete</button></div>
      </article>)}
    </div>
    {showKey && <div className="dialog-backdrop"><div className="dialog"><div className="section-heading"><h2>New API Key profile</h2><button className="icon-button" onClick={() => setShowKey(false)}>×</button></div><label className="field"><span>Name</span><input value={name} onChange={(event) => setName(event.target.value)} /></label><label className="field"><span>Provider</span><input value={provider} onChange={(event) => setProvider(event.target.value)} /></label><label className="field"><span>Base URL</span><input value={baseUrl} onChange={(event) => setBaseUrl(event.target.value)} /></label><label className="field"><span>API Key</span><input type="password" value={key} onChange={(event) => setKey(event.target.value)} /></label><button className="primary" onClick={() => void createAPIKeyProfile()}>Create profile</button></div></div>}
    {confirmAction && <div className="dialog-backdrop"><div className="dialog"><div className="section-heading"><h2>{confirmAction.kind === 'delete' ? 'Delete profile?' : 'Provider configuration differs'}</h2><button className="icon-button" onClick={() => setConfirmAction(null)}>×</button></div><p>{confirmAction.kind === 'delete' ? `Delete ${confirmAction.profile?.name || 'this profile'} and its saved credentials? Remote backups and current credentials will not change.` : `${confirmAction.description || 'The local Provider table differs from this profile.'} Only auth.json and model_provider will change; the local Provider table will be kept.`}</p><div className="button-row"><button className="secondary" onClick={() => setConfirmAction(null)}>Cancel</button><button className={confirmAction.kind === 'delete' ? 'danger' : 'primary'} onClick={() => void confirmActionNow()}>{confirmAction.kind === 'delete' ? 'Delete' : 'Switch profile'}</button></div></div></div>}
  </section>;
}

function profileTitle(profile: AuthProfile) {
  if (profile.name === 'Current API Key') return 'API Key';
  if (profile.name === 'Current ChatGPT login') return 'ChatGPT';
  return profile.name;
}

function profileSummary(profile: AuthProfile) {
  const config = profile.provider;
  if (profile.type === 'api_key') {
    return [config, profile.apiKeyPreview, profile.baseUrl || 'no base_url'].filter(Boolean).join(' · ') || 'API Key';
  }
  const identity = profile.chatgptUserId ? `${profile.chatgptUserId}${profile.chatgptExpiresAt ? ` · expire at: ${profile.chatgptExpiresAt}` : ''}` : 'ChatGPT';
  return [config, identity].filter(Boolean).join(' · ');
}

function BackupsTab({ setNotice }: { setNotice: (value: { kind: NoticeKind; text: string } | null) => void }) {
  const [backups, setBackups] = useState<BackupInfo[]>([]);
  const [profiles, setProfiles] = useState<AuthProfile[]>([]);
  async function load() { if (!api) return; try { setBackups(await api.ListRemoteBackups()); setProfiles(await api.ListAuthProfiles()); } catch (error) { setNotice({ kind: 'error', text: String(error) }); } }
  useEffect(() => { void load(); }, []);
  async function upload(id: string) { if (!api) return; try { await api.UploadAuthProfile(id, false); setNotice({ kind: 'success', text: 'Authentication profile uploaded.' }); await load(); } catch (error) { setNotice({ kind: 'error', text: String(error) }); } }
  async function restore(device: string, id: string) { if (!api) return; try { const profile = await api.RestoreAuthProfile(device, id); setNotice({ kind: 'success', text: `Restored ${profile.name} as a local inactive profile.` }); } catch (error) { setNotice({ kind: 'error', text: String(error) }); } }
  async function remove(device: string, id: string) { if (!api) return; try { await api.DeleteRemoteBackup(device, id); setNotice({ kind: 'success', text: 'Remote backup deleted.' }); await load(); } catch (error) { setNotice({ kind: 'error', text: String(error) }); } }
  return <section className="workspace"><div className="section-heading"><div><h1>Authentication backups</h1><p>Restore named profiles from the central node when moving between devices.</p></div><button className="secondary" onClick={() => void load()}>Refresh backups</button></div><Notice kind="warning">Authentication backups contain plaintext credentials and are intended only for a trusted local network.</Notice><div className="backup-actions"><strong>Upload a local profile</strong>{profiles.length === 0 && <span className="empty-inline">No local profiles yet.</span>}{profiles.map((profile) => <button className="secondary" key={profile.id} onClick={() => void upload(profile.id)}>{profile.name}</button>)}</div><div className="profile-list">{backups.length === 0 && <div className="empty-inline">No remote backups found.</div>}{backups.map((backup) => <article className="profile-row" key={`${backup.device}:${backup.profile.id}`}><div><strong>{backup.profile.name}</strong><small>{backup.device} · {backup.profile.type} · {backup.profile.provider}</small></div><div className="button-row"><button className="secondary" onClick={() => void restore(backup.device, backup.profile.id)}>Restore</button><button className="danger" onClick={() => void remove(backup.device, backup.profile.id)}>Delete</button></div></article>)}</div></section>;
}
function ServerApp({ bootstrap }: { bootstrap: Bootstrap }) { return <main className="shell"><header className="topbar"><div><div className="brand">AgentPal</div><small>Central configuration node</small></div><span className="connection-dot online">● Server running</span></header><section className="workspace server-workspace"><h1>Central node is running</h1><p>Publishing the configured Codex directory over trusted LAN HTTP.</p><div className="server-grid"><div><small>Listen address</small><strong>0.0.0.0:{bootstrap.port}</strong></div><div><small>Published directory</small><strong>{bootstrap.codexDir}</strong></div><div><small>Backup directory</small><strong>{bootstrap.backupDir}</strong></div></div><Notice kind="warning">Authentication backups are plaintext and unauthenticated in this trusted-network mode.</Notice></section><div className="version-status">v{bootstrap.version}</div></main>; }
function Menu({ bootstrap, onRefresh, onSettings, onClose }: { bootstrap: Bootstrap; onRefresh: () => Promise<void>; onSettings: () => void; onClose: () => void }) { return <div className="menu-popover"><strong>Settings</strong><small>Central node: {bootstrap.centralHost}:{bootstrap.centralPort}</small><button onClick={() => { onSettings(); onClose(); }}>Central and Codex settings</button><button onClick={() => { void onRefresh(); onClose(); }}>Refresh configuration</button><button onClick={() => { void api?.OpenFolder(bootstrap.codexDir); onClose(); }}>Open Codex folder</button></div>; }
function SettingsDialog({ bootstrap, onClose, setNotice }: { bootstrap: Bootstrap; onClose: () => void; setNotice: (value: { kind: NoticeKind; text: string } | null) => void }) {
  const [form, setForm] = useState<SettingsRequest>({ mode: bootstrap.mode, codexDir: bootstrap.codexDir, centralHost: bootstrap.centralHost, centralPort: bootstrap.centralPort, port: bootstrap.port, deviceName: bootstrap.deviceName, backupDir: bootstrap.backupDir });
  function update<K extends keyof SettingsRequest>(key: K, value: SettingsRequest[K]) { setForm({ ...form, [key]: value }); }
  async function save() { if (!api) return; try { const result = await api.SaveSettings(form); setNotice({ kind: 'success', text: result.restartRequired ? 'Settings saved. Restart AgentPal to apply the role or server path change.' : 'Settings saved. Refresh to connect with the new center address.' }); onClose(); } catch (error) { setNotice({ kind: 'error', text: String(error) }); } }
  return <div className="dialog-backdrop"><div className="dialog"><div className="section-heading"><h2>AgentPal settings</h2><button className="icon-button" onClick={onClose}>×</button></div><label className="field"><span>Central host</span><input value={form.centralHost} onChange={(event) => update('centralHost', event.target.value)} /></label><label className="field"><span>Central port</span><input type="number" value={form.centralPort} onChange={(event) => update('centralPort', Number(event.target.value))} /></label><label className="field"><span>Codex directory</span><input value={form.codexDir} onChange={(event) => update('codexDir', event.target.value)} /></label><label className="field"><span>Device name</span><input value={form.deviceName} onChange={(event) => update('deviceName', event.target.value)} /></label><div className="button-row"><button className="secondary" onClick={onClose}>Cancel</button><button className="primary" onClick={() => void save()}>Save settings</button></div></div></div>;
}
type NoticeKind = 'info' | 'success' | 'warning' | 'error';
function Notice({ kind, children }: { kind: NoticeKind; children: ReactNode }) { return <div className={`notice ${kind}`}>{children}</div>; }
function labelForTab(tab: Tab) { return tab[0].toUpperCase() + tab.slice(1); }
function statusLabel(status: string) { return status === 'local_only' ? 'Local only' : status[0].toUpperCase() + status.slice(1); }
