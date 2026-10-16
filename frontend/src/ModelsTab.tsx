import { useEffect, useState } from 'react';
import { api, type ModelEntry, type ModelManagementState, type DefaultModelSettings } from './wails';
import modelTemplate from '../../internal/modelmanagement/template.json';

// Only newly discovered entries use this conservative template.
export function mergeModels(models: ModelEntry[], ids: string[]) {
  const available = new Set(ids.filter(Boolean));
  const retained = models.filter((model) => model && typeof model === 'object' && typeof model.slug === 'string' && available.has(model.slug));
  const seen = new Set(retained.map((model) => model.slug));
  const additions: ModelEntry[] = [];
  for (const id of ids) {
    if (!id || seen.has(id)) continue;
    seen.add(id);
    additions.push({ ...modelTemplate, slug: id, display_name: id });
  }
  return { models: [...retained, ...additions], added: additions.length, removed: models.length - retained.length };
}

export function parseModelDraft(text: string): { models: ModelEntry[] } | { error: string } {
  let parsed: unknown;
  try { parsed = JSON.parse(text); } catch (error) { return { error: `Invalid JSON: ${String(error)}` }; }
  const list = Array.isArray(parsed) ? parsed : (parsed && typeof parsed === 'object' && Array.isArray((parsed as { models?: unknown }).models) ? (parsed as { models: unknown[] }).models : null);
  if (!list) return { error: 'Expected a JSON array of models, or an object with a "models" array.' };
  for (const item of list) {
    if (!item || typeof item !== 'object' || Array.isArray(item)) return { error: 'Every model entry must be a JSON object.' };
  }
  return { models: list as ModelEntry[] };
}

export function summarizeEnrichment(state: ModelManagementState) {
  if (state.enrichmentError) return `Models saved, but model parameters were not updated: ${state.enrichmentError}`;
  const unmatched = state.unmatchedModels || [];
  if (!unmatched.length) return 'Models saved and parameters updated from the model catalog. Restart ChatGPT / Codex to reload the model list.';
  return `Models saved. Kept local parameters for ${unmatched.length} unmatched model${unmatched.length === 1 ? '' : 's'}: ${unmatched.join(', ')}`;
}

export default function ModelsTab({ onDirtyChange, onBusyChange }: { onDirtyChange: (dirty: boolean) => void; onBusyChange: (busy: boolean) => void }) {
  const [state, setState] = useState<ModelManagementState | null>(null);
  const [draft, setDraft] = useState<{ raw: string; visibility?: string }[]>([]);
  const [busy, setBusy] = useState(true);
  const [message, setMessage] = useState<{ error: boolean; text: string } | null>(null);
  const [deleting, setDeleting] = useState<number | null>(null);
  const [defaults, setDefaults] = useState<DefaultModelSettings | null>(null);
  const [defaultModel, setDefaultModel] = useState('');
  const [effort, setEffort] = useState('');
  const [updateParams, setUpdateParams] = useState(true);
  const [jsonOpen, setJsonOpen] = useState(false);
  const [jsonText, setJsonText] = useState('');
  const baselineJson = JSON.stringify(draft.map((entry) => JSON.parse(entry.raw)), null, 2);
  const modelsDirty = !!state && JSON.stringify(draft) !== JSON.stringify(state.rawModels.map((raw) => ({ raw })));
  const jsonDirty = jsonOpen && jsonText !== baselineJson;
  const defaultsDirty = !!defaults && (defaultModel !== defaults.model || effort !== defaults.effort);
  const dirty = modelsDirty || defaultsDirty || jsonDirty;
  useEffect(() => { onDirtyChange(dirty); }, [dirty, onDirtyChange]);
  useEffect(() => { onBusyChange(busy); }, [busy, onBusyChange]);
  useEffect(() => { void load(); }, []);

  async function load() {
    if (!api) return;
    setBusy(true); setMessage(null);
    try {
      const [next, settings] = await Promise.all([api.GetModelManagementState(), api.GetDefaultModelSettings()]);
      setState(next); setDraft(next.rawModels.map((raw) => ({ raw })));
      setDefaults(settings); setDefaultModel(settings.model); setEffort(settings.effort);
    }
    catch (error) { setMessage({ error: true, text: String(error) }); }
    finally { setBusy(false); }
  }
  async function discover() {
    if (!api) return;
    setBusy(true); setMessage(null);
    try {
      const ids = await api.DiscoverModels();
      const models = draft.map((entry) => JSON.parse(entry.raw) as ModelEntry);
      const existing = new Map(models.map((model, index) => [model, draft[index]]));
      const result = mergeModels(models, ids);
      setDraft(result.models.map((model) => existing.get(model) ?? { raw: JSON.stringify(model) }));
      setMessage({ error: false, text: result.added || result.removed ? `Updated draft: added ${result.added}, removed ${result.removed} model${result.removed === 1 ? '' : 's'}. Save models to apply changes.` : 'Models are already up to date.' });
    } catch (error) { setMessage({ error: true, text: String(error) }); }
    finally { setBusy(false); }
  }
  async function save() {
    if (!api || !state) return;
    if (jsonOpen) {
      const parsed = parseModelDraft(jsonText);
      if ('error' in parsed) { setMessage({ error: true, text: parsed.error }); return; }
    }
    const entries = jsonOpen ? (parseModelDraft(jsonText) as { models: ModelEntry[] }).models : draft.map((entry) => JSON.parse(entry.raw) as ModelEntry);
    setBusy(true); setMessage(null);
    try {
      const next = await api.SaveModels({ fileHash: state.fileHash, rawModels: entries.map((model) => JSON.stringify(model)), visibilityOverrides: jsonOpen ? {} : Object.fromEntries(draft.flatMap((entry, index) => entry.visibility ? [[index, entry.visibility]] : [])), updateParams });
      if (!next.available || !next.fileValid || next.error) throw new Error(next.error || 'Could not reload saved models.');
      setState(next); setDraft(next.rawModels.map((raw) => ({ raw })));
      if (jsonOpen) setJsonText(JSON.stringify(next.rawModels.map((raw) => JSON.parse(raw)), null, 2));
      setDefaults(await api.GetDefaultModelSettings());
      setMessage({ error: false, text: updateParams ? summarizeEnrichment(next) : 'Models saved and catalog configured. Restart ChatGPT / Codex to reload the model list.' });
    } catch (error) { setMessage({ error: true, text: String(error) }); }
    finally { setBusy(false); }
  }
  function toggleJson() {
    if (!jsonOpen) { setJsonText(baselineJson); setJsonOpen(true); return; }
    setJsonOpen(false); setJsonText('');
  }
  async function saveDefaults() {
    if (!api || !defaults) return;
    setBusy(true); setMessage(null);
    try {
      const next = await api.SaveDefaultModelSettings({ configHash: defaults.configHash, model: defaultModel, effort });
      if (!next.available) throw new Error(next.error || 'Could not reload default settings.');
      setDefaults(next); setDefaultModel(next.model); setEffort(next.effort);
      setMessage({ error: false, text: 'Default model settings saved.' });
    } catch (error) { setMessage({ error: true, text: String(error) }); }
    finally { setBusy(false); }
  }
  const editable = !!state?.available && !!state.fileValid && !!state.writable;
  const selectable = draft.filter((entry) => { const model = JSON.parse(entry.raw); return model && typeof model === 'object' && !Array.isArray(model); });
  const allVisible = selectable.length > 0 && selectable.every((entry) => (entry.visibility ?? JSON.parse(entry.raw).visibility) === 'list');
  function toggleAll(visible: boolean) {
    const visibility = visible ? 'list' : 'hide';
    setDraft(draft.map((entry) => {
      const model = JSON.parse(entry.raw);
      if (!model || typeof model !== 'object' || Array.isArray(model)) return entry;
      return model.visibility === visibility ? { raw: entry.raw } : { raw: entry.raw, visibility };
    }));
  }
  return <section className="workspace">
    <div className="section-heading"><div><h1>Models</h1><p>{state?.available ? [state.provider || 'Current Provider', state.baseUrl].filter(Boolean).join(' · ') : 'Manage models for the current API Key profile.'}</p></div>
      <div className="button-row models-toolbar"><button className="secondary" aria-pressed={jsonOpen} title="Toggle raw JSON editor" disabled={busy || !editable} onClick={toggleJson}>Raw</button><button className="secondary" disabled={busy || !editable || !!state?.discoveryError} title={state?.discoveryError || 'Pull models from the current Provider'} onClick={() => void discover()}>Pull</button><label className="model-toggle" title="Refresh reasoning levels, input modalities and context window from the model catalog on save"><input type="checkbox" checked={updateParams} disabled={busy} onChange={(event) => setUpdateParams(event.target.checked)} /><span>params</span></label><button className="primary" disabled={busy || !editable || (!modelsDirty && !jsonDirty && !updateParams)} onClick={() => void save()}>Save</button></div>
    </div>
    {busy && <div role="status" className="empty-inline">Loading models…</div>}
    {message && <div role={message.error ? 'alert' : 'status'} className={`notice ${message.error ? 'error' : 'success'}`}>{message.text}</div>}
    {state?.error && <div role="alert" className={`notice ${state.available ? 'error' : 'warning'}`}>{state.error}</div>}
    {state?.available && state.discoveryError && <div className="notice warning">Pull unavailable: {state.discoveryError}</div>}
    {state?.available && defaults?.error && <div role="alert" className="notice error">Default model settings unavailable: {defaults.error}</div>}
    {defaults?.available && <section className="model-defaults" aria-label="Default model settings">
      <label className="field"><span>Default model</span><input list="default-model-options" value={defaultModel} disabled={busy} placeholder="Codex default" onChange={(event) => setDefaultModel(event.target.value)} /><datalist id="default-model-options">{Array.from(new Set(draft.flatMap((entry) => { const model = JSON.parse(entry.raw); return typeof model?.slug === 'string' ? [model.slug] : []; }))).map((slug) => <option key={slug} value={slug} />)}</datalist></label>
      <label className="field"><span>Reasoning effort</span><select value={effort} disabled={busy} onChange={(event) => setEffort(event.target.value)}><option value="">Codex default</option>{['none', 'minimal', 'low', 'medium', 'high', 'xhigh'].map((value) => <option value={value} key={value}>{value}</option>)}{effort && !['none', 'minimal', 'low', 'medium', 'high', 'xhigh'].includes(effort) && <option value={effort}>{effort} (current)</option>}</select></label>
      <button className="secondary" disabled={busy || !defaultsDirty} onClick={() => void saveDefaults()}>Save defaults</button>
    </section>}
    {!busy && <div className="models-status"><span>{dirty ? 'Unsaved changes' : state?.available && state.fileValid ? 'All changes saved' : ''}</span><button className="text-button" onClick={() => { if (!dirty || window.confirm('Discard unsaved model changes and reload?')) void load(); }}>Reload models</button></div>}
    {state?.available && state.fileValid && <div className="profile-list">
      {jsonOpen && <label className="field"><span>models.json (models array)</span><textarea className="code-editor" spellCheck={false} rows={16} aria-label="models.json editor" value={jsonText} onChange={(event) => setJsonText(event.target.value)} /></label>}
      {draft.length > 0 && <div className="models-select-all"><span>Select all models</span><label className="model-visibility"><input type="checkbox" role="switch" aria-label="Select all models" checked={allVisible} disabled={busy || !editable || !selectable.length} onChange={(event) => toggleAll(event.target.checked)} /><span aria-hidden="true" /></label></div>}
      {!draft.length && <div className="empty-inline">{state.fileExists ? 'No models configured.' : 'No models.json file yet.'} Use Pull to discover models.</div>}
      {draft.map((entry, index) => {
        const parsed = JSON.parse(entry.raw);
        const model: ModelEntry | null = parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : null;
        const title = typeof model?.display_name === 'string' ? model.display_name : typeof model?.slug === 'string' ? model.slug : `Model ${index + 1}`;
        const unmatched = !!state?.unmatchedModels?.includes(typeof model?.slug === 'string' ? model.slug : '');
        return <article className="profile-row model-row" key={index}><div className="profile-main"><strong title={title}>{title}</strong><small title={typeof model?.slug === 'string' ? model.slug : undefined}>{typeof model?.slug === 'string' ? model.slug : 'No slug'}</small>{unmatched && <small className="sync-status sync-needs_sync">Not found in model catalog · kept local parameters</small>}</div><div className="profile-actions inline">
          <label className="model-visibility" title={`Show ${title}`}><input type="checkbox" role="switch" aria-label={`Show ${title}`} checked={(entry.visibility ?? model?.visibility) === 'list'} disabled={busy || !editable || !model} onChange={(event) => setDraft(draft.map((item, i) => { if (i !== index) return item; const visibility = event.target.checked ? 'list' : 'hide'; return visibility === model?.visibility ? { raw: item.raw } : { raw: item.raw, visibility }; }))} /><span aria-hidden="true" /></label>
          <button className="secondary" disabled={busy || !editable} aria-label={`Delete ${title}`} onClick={() => setDeleting(index)}>Delete</button>
        </div></article>;
      })}
    </div>}
    {deleting !== null && <div className="dialog-backdrop"><div className="dialog" role="dialog" aria-modal="true" aria-labelledby="delete-model-title"><h2 id="delete-model-title">Delete model?</h2><p>Remove this model from the draft? Save models to apply the deletion.</p><div className="button-row"><button className="secondary" onClick={() => setDeleting(null)}>Cancel</button><button className="danger" onClick={() => { setDraft(draft.filter((_, i) => i !== deleting)); setDeleting(null); }}>Delete</button></div></div></div>}
  </section>;
}
