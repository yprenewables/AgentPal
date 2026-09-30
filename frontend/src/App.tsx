import { useEffect, useState, type ReactNode } from 'react';
import { api, type ApplyChange, type AuthProfile, type AuthSyncItem, type BackupInfo, type Bootstrap, type ConfigRefresh, type CurrentAuthStatus, type RawProfileContent, type RawProfileRequest, type SettingsRequest } from './wails';

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
  const [editDialog, setEditDialog] = useState<{ open: boolean; profile?: AuthProfile; name: string; authJson: string; configToml: string }>({ open: false, name: '', authJson: '', configToml: '' });
  const [confirmAction, setConfirmAction] = useState<{ kind: 'switch' | 'delete'; profile?: AuthProfile; description?: string } | null>(null);
  const [login, setLogin] = useState<{ loggedIn: boolean; subject?: string; name?: string; displayName?: string; email?: string }>({ loggedIn: false });
  const [syncItems, setSyncItems] = useState<AuthSyncItem[]>([]);
  const [dismissed, setDismissed] = useState<string[]>([]);
  const [loginBusy, setLoginBusy] = useState(false);
  const [syncing, setSyncing] = useState(false);

  // 已登录时自动拉取服务端为当前用户维护的 profile，无需手动点击同步。
  useEffect(() => {
    void api?.GetCentralLoginState().then((state) => {
      setLogin({ loggedIn: state.loggedIn, subject: state.subject, name: state.name, displayName: state.displayName, email: state.email });
      if (state.loggedIn) void runSync(true);
    }).catch(() => setLogin({ loggedIn: false }));
  }, []);

  // silent=true 用于自动同步：成功时不弹提示，只在新增 profile 或失败时提示。
  async function runSync(silent = false) {
    if (!api) return;
    setSyncing(true);
    try {
      const report = await api.SyncCentralAuthProfiles();
      setSyncItems(report.items);
      await reload();
      const added = report.items.filter((item) => item.status === 'added').length;
      if (added > 0 && silent) setNotice({ kind: 'success', text: `Added ${added} central profile${added > 1 ? 's' : ''}.` });
    } catch (error) {
      setNotice({ kind: 'error', text: `Central profile sync failed: ${String(error)}` });
    } finally {
      setSyncing(false);
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
          setLogin({ loggedIn: true, subject: status.subject, name: status.name, displayName: status.displayName, email: status.email });
          setNotice({ kind: 'success', text: 'Logged in with Casdoor.' });
          await runSync(true);
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
    setDismissed([]);
  }

  async function overwrite(name: string) {
    if (!api) return;
    try {
      await api.ApplyCentralAuthProfile(name);
      await reload();
      await runSync(true);
      setNotice({ kind: 'success', text: `Overwrote local profile ${name} from central.` });
    } catch (error) {
      setNotice({ kind: 'error', text: String(error) });
    }
  }

  function dismissItem(name: string) {
    setDismissed([...dismissed, name]);
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

  async function openCreateDialog() {
    if (!api) return;
    const template = await api.GetDefaultProfileTemplate();
    setEditDialog({ open: true, name: 'New Profile', authJson: template.authJson, configToml: template.configToml });
  }

  async function openEditDialog(profile: AuthProfile) {
    if (!api) return;
    try {
      const content = await api.GetRawProfileContent(profile.id);
      setEditDialog({ open: true, profile, name: profile.name, authJson: content.authJson, configToml: content.configToml });
    } catch (error) {
      setNotice({ kind: 'error', text: String(error) });
    }
  }

  async function saveRawProfile() {
    if (!api) return;
    try {
      if (editDialog.profile) {
        await api.UpdateRawProfile(editDialog.profile.id, { name: editDialog.name, authJson: editDialog.authJson, configToml: editDialog.configToml });
        setNotice({ kind: 'success', text: 'Profile updated.' });
      } else {
        await api.CreateRawProfile({ name: editDialog.name, authJson: editDialog.authJson, configToml: editDialog.configToml });
        setNotice({ kind: 'success', text: 'Profile created.' });
      }
      setEditDialog({ open: false, name: '', authJson: '', configToml: '' });
      await reload();
    } catch (error) {
      setNotice({ kind: 'error', text: String(error) });
    }
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
  // 中心 profile（登录后由服务端下发并保存到本地）与纯本地 profile 分组展示。
  const centralProfiles = profiles.filter(isCentralProfile);
  const localProfiles = profiles.filter((profile) => !isCentralProfile(profile));
  const syncByName = new Map(syncItems.map((item) => [item.name, item]));
  const centralNames = new Set(centralProfiles.map((profile) => profile.reservedName || profile.name));
  const centralOnlyItems = syncItems.filter((item) => !centralNames.has(item.name));
  const displayName = login.displayName || login.name || (login.subject && !login.subject.match(/^[0-9a-f-]{36}$/i) ? login.subject : '');
  return <section className="workspace">
    <div className="section-heading">
      <div><h1>Authentication</h1><p>Choose one local API Key or ChatGPT profile.</p></div>
    </div>
    {!hasCurrentProfile && current?.apiKeyPreview && <div className="current-auth"><div className="current-auth-line"><strong>API KEY: {current.apiKeyPreview}</strong><span>{current.baseUrl || 'base_url not detected'}</span><button className="secondary" onClick={() => void importCurrentAPIKey()}>Import</button></div></div>}
    {!hasCurrentProfile && !current?.apiKeyPreview && current?.chatgptUserId && <div className="current-auth"><div className="current-auth-line"><strong>ChatGPT User ID: {current.chatgptUserId}</strong><span>expire at: {current.chatgptExpiresAt || 'unknown'}</span><button className="secondary" onClick={() => void importCurrentChatGPT()}>Import</button></div></div>}
    <section className="profile-group" aria-labelledby="central-profiles-heading">
      <div className="profile-group-heading">
        <h2 id="central-profiles-heading">Central profiles{displayName && ` · ${displayName}`}</h2>
        <div className="profile-group-actions">
          {login.loggedIn && <><IconButton label={syncing ? 'Refreshing' : 'Refresh central profiles'} disabled={syncing} onClick={() => void runSync()}><RefreshIcon spinning={syncing} /></IconButton><IconButton label="Log out" onClick={() => void logout()}><LogoutIcon /></IconButton></>}
          {!login.loggedIn && centralProfiles.length > 0 && <button className="primary compact" onClick={() => void startLogin()} disabled={loginBusy}>{loginBusy ? 'Waiting...' : 'Log in'}</button>}
        </div>
      </div>
      {!login.loggedIn && centralProfiles.length === 0 && <div className="empty-state compact">
        <div className="status-symbol">🔒</div>
        <h3>Sign in to access central profiles</h3>
        <p>Server-managed authentication profiles will appear here after login.</p>
        <button className="primary" onClick={() => void startLogin()} disabled={loginBusy}>{loginBusy ? 'Waiting for login...' : 'Login with Casdoor'}</button>
      </div>}
      {(login.loggedIn || centralProfiles.length > 0) && <div className="profile-list">
        {login.loggedIn && centralProfiles.length === 0 && centralOnlyItems.length === 0 && <div className="empty-inline">{syncing ? 'Loading central profiles...' : 'No profiles are assigned to this account on the server.'}</div>}
        {centralProfiles.map((profile) => {
          const item = syncByName.get(profile.reservedName || profile.name);
          const needsSync = item?.status === 'needs_sync' && !dismissed.includes(item.name);
          return <article className={`profile-row central ${profile.active ? 'active' : ''}`} key={profile.id}>
            <div className="profile-main"><strong>{profileTitle(profile)}</strong><small>{profileSummary(profile)}</small><small className={`sync-status sync-${item ? (needsSync ? 'needs_sync' : item.status === 'needs_sync' ? 'kept' : item.status) : 'missing'}`}>{centralStatusText(item, login.loggedIn, syncing, !!item && dismissed.includes(item.name))}</small></div>
            <div className="profile-actions">
              {needsSync && <IconButton label="Update from server" onClick={() => void overwrite(item.name)}><DownloadIcon /></IconButton>}
              {needsSync && <IconButton label="Keep local copy" onClick={() => dismissItem(item.name)}><CloseIcon /></IconButton>}
              {profile.active ? <span className="badge success-badge">Current</span> : <button className="secondary compact" onClick={() => void activate(profile.id)}>Use</button>}
            </div>
          </article>;
        })}
        {centralOnlyItems.map((item) => <article className="profile-row central" key={`remote:${item.name}`}>
          <div className="profile-main"><strong>{item.name}</strong><small className="sync-status sync-error">Could not load from server{item.detail ? ` · ${item.detail}` : ''}</small></div>
        </article>)}
      </div>}
    </section>
    <section className="profile-group" aria-labelledby="local-profiles-heading">
      <div className="profile-group-heading">
        <h2 id="local-profiles-heading">Local profiles</h2>
        <div className="profile-group-actions">
          <IconButton label="New local profile" onClick={() => void openCreateDialog()}><PlusIcon /></IconButton>
        </div>
      </div>
      <div className="profile-list">
        {localProfiles.length === 0 && <div className="empty-inline">No local profiles yet.</div>}
        {localProfiles.map((profile) => <article className={`profile-row ${profile.active ? 'active' : ''}`} key={profile.id}>
          <div className="profile-main"><strong>{profileTitle(profile)}</strong><small>{profileSummary(profile)}</small></div>
          <div className="profile-actions">
            {profile.type === 'api_key' && <IconButton label={`Edit ${profile.name}`} onClick={() => void openEditDialog(profile)}><EditIcon /></IconButton>}
            <IconButton label={profile.active ? 'Switch to another profile before deleting this one' : `Delete ${profile.name}`} danger disabled={profile.active} onClick={() => void removeProfile(profile)}><TrashIcon /></IconButton>
            {profile.active ? <span className="badge success-badge">Current</span> : <button className="secondary compact" onClick={() => void activate(profile.id)}>Use</button>}
          </div>
        </article>)}
      </div>
    </section>
    {editDialog.open && <div className="dialog-backdrop"><div className="dialog wide" role="dialog" aria-modal="true" aria-labelledby="profile-editor-title">
      <div className="section-heading"><h2 id="profile-editor-title">{editDialog.profile ? 'Edit local profile' : 'New local profile'}</h2><IconButton label="Close" onClick={() => setEditDialog({ open: false, name: '', authJson: '', configToml: '' })}><CloseIcon /></IconButton></div>
      <label className="field"><span>Name</span><input value={editDialog.name} onChange={(event) => setEditDialog({ ...editDialog, name: event.target.value })} /></label>
      <label className="field"><span>auth.json</span><textarea className="code-editor" spellCheck={false} rows={5} value={editDialog.authJson} onChange={(event) => setEditDialog({ ...editDialog, authJson: event.target.value })} /></label>
      <label className="field"><span>config-auth.toml</span><textarea className="code-editor" spellCheck={false} rows={10} value={editDialog.configToml} onChange={(event) => setEditDialog({ ...editDialog, configToml: event.target.value })} /></label>
      <div className="button-row dialog-actions"><button className="secondary" onClick={() => setEditDialog({ open: false, name: '', authJson: '', configToml: '' })}>Cancel</button><button className="primary" onClick={() => void saveRawProfile()}>{editDialog.profile ? 'Save' : 'Create'}</button></div>
    </div></div>}
    {confirmAction && <div className="dialog-backdrop"><div className="dialog"><div className="section-heading"><h2>{confirmAction.kind === 'delete' ? 'Delete profile?' : 'Provider configuration differs'}</h2><button className="icon-button" onClick={() => setConfirmAction(null)}>×</button></div><p>{confirmAction.kind === 'delete' ? `Delete ${confirmAction.profile?.name || 'this profile'} and its saved credentials? Remote backups and current credentials will not change.` : `${confirmAction.description || 'The local Provider table differs from this profile.'} Only auth.json and model_provider will change; the local Provider table will be kept.`}</p><div className="button-row"><button className="secondary" onClick={() => setConfirmAction(null)}>Cancel</button><button className={confirmAction.kind === 'delete' ? 'danger' : 'primary'} onClick={() => void confirmActionNow()}>{confirmAction.kind === 'delete' ? 'Delete' : 'Switch profile'}</button></div></div></div>}
  </section>;
}

function isCentralProfile(profile: AuthProfile) {
  return profile.source === 'central_login' || !!profile.reservedName;
}

function centralStatusText(item: AuthSyncItem | undefined, loggedIn: boolean, syncing: boolean, kept: boolean) {
  if (!loggedIn) return 'Saved copy · log in to check for server updates';
  if (!item) return syncing ? 'Checking server...' : 'No longer assigned to this account on the server';
  if (item.status === 'needs_sync') return kept ? 'Keeping local copy · server version differs' : 'Server has a different version';
  if (item.status === 'error') return `Server check failed${item.detail ? ` · ${item.detail}` : ''}`;
  return 'Up to date with server';
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

// 图标按钮
function IconButton({ label, onClick, disabled, danger, children }: { label: string; onClick: () => void; disabled?: boolean; danger?: boolean; children: ReactNode }) {
  return <button className={`icon-btn${danger ? ' danger' : ''}`} aria-label={label} title={label} disabled={disabled} onClick={onClick}>{children}</button>;
}
function RefreshIcon({ spinning }: { spinning?: boolean }) { return <svg className={spinning ? 'spin' : ''} width="16" height="16" fill="currentColor" viewBox="0 0 16 16"><path fillRule="evenodd" d="M8 3a5 5 0 1 0 4.546 2.914.5.5 0 0 1 .908-.417A6 6 0 1 1 8 2v1z"/><path d="M8 4.466V.534a.25.25 0 0 1 .41-.192l2.36 1.966c.12.1.12.284 0 .384L8.41 4.658A.25.25 0 0 1 8 4.466z"/></svg>; }
function LogoutIcon() { return <svg width="16" height="16" fill="currentColor" viewBox="0 0 16 16"><path fillRule="evenodd" d="M6 12.5a.5.5 0 0 0 .5.5h8a.5.5 0 0 0 .5-.5v-9a.5.5 0 0 0-.5-.5h-8a.5.5 0 0 0-.5.5v2a.5.5 0 0 1-1 0v-2A1.5 1.5 0 0 1 6.5 2h8A1.5 1.5 0 0 1 16 3.5v9a1.5 1.5 0 0 1-1.5 1.5h-8A1.5 1.5 0 0 1 5 12.5v-2a.5.5 0 0 1 1 0v2z"/><path fillRule="evenodd" d="M.146 8.354a.5.5 0 0 1 0-.708l3-3a.5.5 0 1 1 .708.708L1.707 7.5H10.5a.5.5 0 0 1 0 1H1.707l2.147 2.146a.5.5 0 0 1-.708.708l-3-3z"/></svg>; }
function EditIcon() { return <svg width="16" height="16" fill="currentColor" viewBox="0 0 16 16"><path d="M12.146.146a.5.5 0 0 1 .708 0l3 3a.5.5 0 0 1 0 .708l-10 10a.5.5 0 0 1-.168.11l-5 2a.5.5 0 0 1-.65-.65l2-5a.5.5 0 0 1 .11-.168l10-10zM11.207 2.5 13.5 4.793 14.793 3.5 12.5 1.207 11.207 2.5zm1.586 3L10.5 3.207 4 9.707V10h.5a.5.5 0 0 1 .5.5v.5h.5a.5.5 0 0 1 .5.5v.5h.293l6.5-6.5zm-9.761 5.175-.106.106-1.528 3.821 3.821-1.528.106-.106A.5.5 0 0 1 5 12.5V12h-.5a.5.5 0 0 1-.5-.5V11h-.5a.5.5 0 0 1-.468-.325z"/></svg>; }
function TrashIcon() { return <svg width="16" height="16" fill="currentColor" viewBox="0 0 16 16"><path d="M5.5 5.5A.5.5 0 0 1 6 6v6a.5.5 0 0 1-1 0V6a.5.5 0 0 1 .5-.5zm2.5 0a.5.5 0 0 1 .5.5v6a.5.5 0 0 1-1 0V6a.5.5 0 0 1 .5-.5zm3 .5a.5.5 0 0 0-1 0v6a.5.5 0 0 0 1 0V6z"/><path fillRule="evenodd" d="M14.5 3a1 1 0 0 1-1 1H13v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V4h-.5a1 1 0 0 1-1-1V2a1 1 0 0 1 1-1H6a1 1 0 0 1 1-1h2a1 1 0 0 1 1 1h3.5a1 1 0 0 1 1 1v1zM4.118 4 4 4.059V13a1 1 0 0 0 1 1h6a1 1 0 0 0 1-1V4.059L11.882 4H4.118zM2.5 3V2h11v1h-11z"/></svg>; }
function PlusIcon() { return <svg width="16" height="16" fill="currentColor" viewBox="0 0 16 16"><path fillRule="evenodd" d="M8 2a.5.5 0 0 1 .5.5v5h5a.5.5 0 0 1 0 1h-5v5a.5.5 0 0 1-1 0v-5h-5a.5.5 0 0 1 0-1h5v-5A.5.5 0 0 1 8 2Z"/></svg>; }
function CloseIcon() { return <svg width="16" height="16" fill="currentColor" viewBox="0 0 16 16"><path d="M2.146 2.854a.5.5 0 1 1 .708-.708L8 7.293l5.146-5.147a.5.5 0 0 1 .708.708L8.707 8l5.147 5.146a.5.5 0 0 1-.708.708L8 8.707l-5.146 5.147a.5.5 0 0 1-.708-.708L7.293 8 2.146 2.854Z"/></svg>; }
function DownloadIcon() { return <svg width="16" height="16" fill="currentColor" viewBox="0 0 16 16"><path d="M.5 9.9a.5.5 0 0 1 .5.5v2.5a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-2.5a.5.5 0 0 1 1 0v2.5a2 2 0 0 1-2 2H2a2 2 0 0 1-2-2v-2.5a.5.5 0 0 1 .5-.5z"/><path d="M7.646 11.854a.5.5 0 0 0 .708 0l3-3a.5.5 0 0 0-.708-.708L8.5 10.293V1.5a.5.5 0 0 0-1 0v8.793L5.354 8.146a.5.5 0 1 0-.708.708l3 3z"/></svg>; }
