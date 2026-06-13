# AgentPal Implementation Plan

This plan turns the README MVP specification into an implementation sequence for a small Wails desktop app.

## Goals

- Build a desktop GUI for sharing and syncing selected Codex resources on a trusted local network.
- Support macOS and Windows with a Go backend and TypeScript frontend.
- Keep v1 intentionally narrow: Codex only, HTTP only, fixed default port `28888`, no discovery, no merge UI.

## MVP Scope

AgentPal v1 supports exactly these resources under a selected Codex directory:

- `config.toml`
- `auth.json`
- `skills/` as a whole directory

The sync direction is one-way:

- Sharing machine exposes selected files over HTTP.
- Receiving machine downloads selected files, verifies hashes, backs up existing files, then replaces them.

## Architecture

Use the structure proposed in `README.md`:

```text
AgentPal/
  main.go
  app.go
  wails.json
  internal/
    codex/
    config/
    peer/
    platform/
    security/
    share/
    sync/
  frontend/
```

### Backend Layers

- `app.go`: Wails-facing API surface and orchestration.
- `internal/codex`: inspect selected Codex directories and resource availability.
- `internal/share`: build manifests, serve health, manifest, and file endpoints.
- `internal/peer`: normalize peer input and fetch remote health/manifest/files.
- `internal/sync`: download, verify, backup, apply, and write history.
- `internal/security`: shared path validation helpers.
- `internal/config`: read/write local AgentPal state under `~/.agentpal`.
- `internal/platform`: local IP enumeration and OS-specific helpers.

Keep business rules out of the frontend. The frontend should call backend methods and render results, while backend code owns validation, filesystem access, HTTP behavior, and sync safety.

## Backend Design

### Wails API

Implement the methods listed in the README:

```go
func (a *App) GetLocalIPs() ([]string, error)
func (a *App) InspectCodexDir(path string) (CodexInspection, error)
func (a *App) BrowseFolder() (string, error)

func (a *App) StartSharing(req ShareRequest) (ShareStatus, error)
func (a *App) StopSharing() error

func (a *App) TestConnection(ip string) (PeerStatus, error)
func (a *App) FetchRemoteManifest(ip string) (RemoteManifest, error)
func (a *App) SyncFromPeer(req SyncRequest) (SyncResult, error)

func (a *App) OpenFolder(path string) error
```

Recommended backend state:

```go
type App struct {
    ctx context.Context
    server *share.Server
    serverMu sync.Mutex
}
```

Only one sharing server should run at a time. `StartSharing` should stop or reject an existing server before starting a new one.

### Data Types

Use stable JSON-shaped structs for the Wails boundary:

```go
type CodexInspection struct {
    Path string `json:"path"`
    Config ResourceStatus `json:"config"`
    Auth ResourceStatus `json:"auth"`
    Skills ResourceStatus `json:"skills"`
}

type ResourceStatus struct {
    Exists bool `json:"exists"`
    Path string `json:"path"`
    Size int64 `json:"size,omitempty"`
    Count int `json:"count,omitempty"`
    Sensitive bool `json:"sensitive,omitempty"`
}

type ShareStatus struct {
    Running bool `json:"running"`
    Port int `json:"port"`
    LocalIPs []string `json:"localIPs"`
    URL string `json:"url"`
}

type PeerStatus struct {
    OK bool `json:"ok"`
    URL string `json:"url"`
    Version string `json:"version"`
}

type SyncResult struct {
    OK bool `json:"ok"`
    Items []string `json:"items"`
    BackupPath string `json:"backupPath"`
}
```

Keep manifest structs aligned with the HTTP protocol in the README so they can be reused by both `share` and `peer` packages.

### Codex Inspection

`InspectCodexDir(path)` should:

- Expand `~` and clean the path.
- Check whether `<dir>/config.toml` exists and is a regular file.
- Check whether `<dir>/auth.json` exists and is a regular file.
- Check whether `<dir>/skills` exists and is a directory.
- Count shareable files under `skills/`, excluding `.git`, `.DS_Store`, and `__pycache__`.
- Return status without reading sensitive contents unless needed for hashing during manifest creation.

Default UI selection should be derived from inspection:

- `config.toml`: selected if found.
- `auth.json`: never selected by default.
- `skills/`: selected if found.

### Share Server

`StartSharing(req)` should:

1. Validate the selected Codex directory.
2. Validate at least one selected resource exists.
3. Build a manifest from selected resources.
4. Start an HTTP server on `0.0.0.0:<port>`.
5. Return local IPs and running status.

Endpoints:

- `GET /health`: static app/version/port response.
- `GET /manifest`: current manifest.
- `GET /files/config.toml`: selected config file only.
- `GET /files/auth.json`: selected auth file only.
- `GET /files/skills/<relative-path>`: selected skills files only.

Important server rules:

- Return `404` for resources that were not selected.
- Reject all skill paths that fail security validation.
- Never serve files outside the selected Codex directory.
- Compute hashes from actual served content.
- Ignore `.git`, `.DS_Store`, and `__pycache__` while building the skills manifest.

### Peer Client

`peer.Normalize(input, defaultPort)` should accept:

- `192.168.1.23`
- `192.168.1.23:28888`
- `http://192.168.1.23:28888`
- `http://192.168.1.23:28888/manifest`

Normalize all valid inputs to a base URL like:

```text
http://192.168.1.23:28888
```

Reject:

- Empty input.
- HTTPS URLs.
- URLs with unsupported schemes.
- Inputs that cannot become a valid host and port.

`TestConnection(ip)` should call `/health` with a short timeout, for example 3 seconds.

`FetchRemoteManifest(ip)` should call `/manifest`, validate schema/app fields, and return available resources to the frontend.

### Sync Pipeline

`SyncFromPeer(req)` should be implemented as a strict pipeline:

1. Normalize peer input.
2. Fetch manifest.
3. Validate selected resources are enabled in the manifest.
4. Create a temporary download directory.
5. Download selected files to temp paths.
6. Verify every downloaded file with `sha256` from the manifest.
7. Create timestamped backup directory under `~/.agentpal/backups/YYYYMMDD-HHMMSS/`.
8. Back up existing target resources that will be replaced.
9. Apply verified temp files to the target Codex directory.
10. Write history entry to `~/.agentpal/history.json`.
11. Remove temp directory.

If any download or verification step fails, do not modify the target Codex directory.

If backup succeeds but apply fails, return an explicit partial failure with the backup path so the UI can show recovery information.

### Backup And Apply Rules

For selected resources:

- Back up existing `<target>/config.toml` before overwrite.
- Back up existing `<target>/auth.json` before overwrite.
- Back up existing `<target>/skills/` as a whole directory before replacement.

Apply rules:

- Ensure target Codex directory exists before writing.
- For `config.toml` and `auth.json`, write files atomically where practical: write temp file in target directory, then rename.
- For `skills/`, remove the existing target `skills/` only after downloaded skills have been fully verified and backup has completed.
- Preserve file contents exactly; do not parse or modify Codex files.

### Security Checks

Centralize path checks in `internal/security`.

For skill relative paths, reject:

- Empty paths.
- Paths containing `..` as a path segment.
- Absolute paths.
- Windows drive paths such as `C:\...`.
- Paths using backslashes as traversal separators.

For filesystem operations:

- Resolve and clean the selected base directory.
- Join child paths under the base directory.
- Verify the final clean path remains under the expected base.
- Ignore `.git`, `.DS_Store`, and `__pycache__` during skills traversal.

## Frontend Design

Build a simple two-page app with navigation between Share and Sync.

### Shared UI Components

- `ResourceChecklist`: renders config/auth/skills selection and statuses.
- `Notice`: renders warning, success, and error messages.
- `DirectoryPicker`: text input plus Browse button if useful.

Avoid complex state libraries for v1. Local React state is enough.

### Share Page State

Fields:

- Codex directory, default `~/.codex`.
- Inspection result.
- Selected resources.
- Local IP list.
- Sharing status.
- Error/success notice.

Flow:

1. On page load, call `InspectCodexDir("~/.codex")` and `GetLocalIPs()`.
2. Default-select `config.toml` and `skills/` only if found.
3. Keep `auth.json` unselected even if found.
4. Re-inspect when the directory changes or browse completes.
5. On Open Sharing, call `StartSharing` with explicit selections.
6. When running, show IP and port clearly.
7. Stop Sharing calls `StopSharing` and resets running status.

Required warnings:

- If `auth.json` is selected, show that it may contain credentials.
- If no resources are selected, disable Open Sharing.

### Sync Page State

Fields:

- Peer IP input.
- Target Codex directory, default `~/.codex`.
- Connection status.
- Remote manifest.
- Selected remote resources.
- Sync result.
- Error/success notice.

Flow:

1. User enters peer IP.
2. Test Connection calls `TestConnection`.
3. Fetch manifest after a successful test, or via an explicit refresh.
4. Default-select every available remote item.
5. Show the required replacement warning before Sync.
6. Sync calls `SyncFromPeer`.
7. Show synced items and backup path on success.

Required warnings:

- Selected items will be backed up and replaced.
- `skills/` replaces the whole target skills directory.
- `auth.json` is sensitive if available or selected.

## Local State

Store local state under `~/.agentpal`:

- `config.json`: last Codex directory, last peer IP, port.
- `history.json`: append-only sync history list.
- `backups/`: timestamped backups.

Keep state writes simple:

- Create `~/.agentpal` on first use.
- Read missing files as empty/default state.
- Write JSON with indentation for manual inspection.

## Error Handling

Return user-facing errors from Wails methods. Prefer clear messages over low-level Go errors.

Examples:

- `config.toml was selected but was not found in the sharing directory`
- `remote manifest does not include skills`
- `downloaded skills/translation/SKILL.md failed sha256 verification`
- `target skills directory was backed up but replacement failed; backup is at ...`

The frontend should show errors in a persistent notice, not only in console output.

## Testing Strategy

### Unit Tests

Prioritize these packages:

- `internal/peer`: input normalization.
- `internal/security`: safe relative path validation and base containment.
- `internal/share`: manifest generation and ignored paths.
- `internal/sync`: hash verification, backup layout, and apply behavior.
- `internal/codex`: directory inspection.

### Integration Tests

Add a Go integration test that:

1. Creates a temporary source Codex directory.
2. Starts a share server on a random local port.
3. Fetches health and manifest through the peer client.
4. Syncs to a temporary target Codex directory.
5. Verifies files match and backup directory is created when target files pre-exist.

### Manual Checks

Before considering MVP done:

- Start sharing with only `config.toml`.
- Start sharing with `skills/` containing ignored files and nested directories.
- Confirm `auth.json` is not selected by default.
- Sync config and skills to an empty target directory.
- Sync over existing config and skills and verify backup contents.
- Try malicious skill paths in tests, including `../x`, `/tmp/x`, and `C:\x`.

## Implementation Phases

### Phase 1: Project Skeleton

- Initialize Wails Go/TypeScript project.
- Add proposed `internal/` packages.
- Add app constants: name, version, default port, state directory.
- Wire basic Wails API methods with placeholder implementations.

Deliverable: app launches and frontend can call backend.

### Phase 2: Filesystem And State

- Implement `~` expansion and default paths.
- Implement Codex directory inspection.
- Implement AgentPal config/history read/write.
- Implement local IP enumeration.

Deliverable: Share page can inspect local Codex resources and show local IPs.

### Phase 3: Share Server

- Implement manifest generation with hashes and file counts.
- Implement `/health`, `/manifest`, and `/files/...` endpoints.
- Add path safety checks for skills.
- Add start/stop server lifecycle through Wails.

Deliverable: another process can fetch manifest and selected files by HTTP.

### Phase 4: Peer Client

- Implement peer input normalization.
- Implement health check and manifest fetch.
- Add frontend Sync page connection test and available resource checklist.

Deliverable: receiving app can discover what the sharing app exposes.

### Phase 5: Sync Pipeline

- Implement temp downloads.
- Verify SHA-256 hashes.
- Implement backup and apply rules.
- Append sync history.
- Return structured sync result.

Deliverable: receiving app can safely sync selected resources.

### Phase 6: Frontend Polish

- Add final Share and Sync layouts.
- Add required warnings and disabled states.
- Add success/error notices.
- Add backup path display after sync.

Deliverable: MVP UX matches README examples and safety expectations.

### Phase 7: Verification And Packaging

- Add Go tests for core backend packages.
- Run Wails dev build on macOS.
- Verify Windows path validation through tests.
- Prepare release notes listing v1 limitations.

Deliverable: testable MVP ready for local trusted-network use.

## Key Risks

- `auth.json` can contain credentials, so default selection and warnings must be correct.
- Directory traversal bugs could expose or overwrite files outside selected directories.
- Replacing `skills/` can remove local-only skills from the active directory; backup visibility is important.
- Local firewalls may block inbound port `28888`; UI should keep connection errors understandable.
- Wails folder picker and open-folder behavior may need small OS-specific handling.

## Done Criteria

The MVP is done when:

- Share page can expose selected Codex resources over HTTP.
- Sync page can test a peer, fetch manifest, select resources, and sync.
- Every downloaded file is verified before apply.
- Existing target resources are backed up before replacement.
- `auth.json` is never selected by default and always shown as sensitive.
- Path traversal and absolute skill paths are rejected by tests.
- Local state and history are written under `~/.agentpal`.

## Immediate Next Steps

1. Create the Go/Wails project skeleton and wire the backend API surface.
2. Implement path utilities and Codex inspection first, since both share and sync depend on them.
3. Add manifest generation and peer normalization before building the sync pipeline.
