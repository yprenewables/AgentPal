# AgentPal Central Client Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace AgentPal's peer-to-peer sharing workflow with a dual-role Codex configuration system: one center serves live Skills/MCP configuration and stores explicit authentication backups, while clients preview and apply configuration changes and manage local authentication profiles.

**Architecture:** One binary reads `~/.agentpal/config.json` and starts in `server` or `client` mode. Backend packages own live manifest generation, structured TOML comparison, transactional filesystem changes, profile switching, and backup APIs; React renders separate server/client shells and calls only typed Wails methods. Legacy `share`, `peer`, and `sync` code remains until the replacement passes end-to-end tests, then is removed.

**Tech Stack:** Go 1.22, Wails v2.12, React, TypeScript, Vite, `github.com/pelletier/go-toml/v2`, Go `httptest`.

## Global Constraints

- First release supports Codex only, using `~/.codex` by default.
- Network transport is HTTP on trusted LAN only; default port is `28888`.
- Skills and MCP changes are previewed and manually applied; local-only resources are preserved unless explicitly selected for deletion.
- Authentication profiles are local-first; remote upload, restore, and deletion are always explicit operations.
- Remote authentication backups are plaintext and unauthenticated in v1; UI and README must display this risk.
- Profile switching changes `auth.json` plus authentication/model/provider configuration only; it must preserve MCP, Projects, and unrelated configuration.
- All writes use `0700` directories and `0600` sensitive files where supported.
- Downloaded content is SHA-256 verified before local configuration changes.
- Each apply or profile switch creates a backup and restores the original files if replacement fails.
- UI uses the approved A2 `Configuration / Authentication / Backups` tab layout, a top-right menu, and small bottom-right version text.
- Do not maintain the old point-to-point Share/Sync behavior after migration.
- Commit steps below are executed only when the user has explicitly authorized commits.

## File Map

- `internal/config/config.go`: persisted role, center address, Codex directory, device name, and server backup directory.
- `internal/codex/mcp.go`: structured MCP extraction, normalization, fragment generation, upsert, and deletion.
- `internal/codex/auth_config.go`: fixed authentication-related root keys/provider table extraction and merge.
- `internal/central/manifest.go`: live center manifest generation for Skills and MCP.
- `internal/central/server.go`: health, manifest, Skills/MCP download, and backup HTTP handlers.
- `internal/central/client.go`: typed HTTP client for center APIs.
- `internal/central/diff.go`: local-vs-center Skills/MCP comparison.
- `internal/central/apply.go`: verified download, backup, selected apply/delete, rollback, and operation result.
- `internal/authprofile/store.go`: local profile metadata and secure filesystem storage.
- `internal/authprofile/import.go`: import current Codex auth state and create API-key profiles.
- `internal/authprofile/switch.go`: validate and transactionally activate a profile.
- `internal/authprofile/archive.go`: deterministic profile archive encoding/decoding used by center backups.
- `internal/types/types.go`: stable HTTP/Wails request and response types.
- `app.go`: role startup and Wails-facing orchestration only.
- `frontend/src/App.tsx`: selects client/server shell from backend bootstrap data.
- `frontend/src/ClientApp.tsx`: A2 client tabs and connection state.
- `frontend/src/ServerApp.tsx`: center status page.
- `frontend/src/components.tsx`: shared menu, notices, dialogs, resource rows, and version status.
- `frontend/src/wails.ts`: complete typed backend contract.
- `frontend/src/style.css`: responsive approved visual design.

---

### Task 1: Persisted Role And Bootstrap Configuration

**Files:**
- Modify: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Modify: `internal/types/types.go`
- Modify: `app.go`

**Interfaces:**
- Produces: `config.State{Mode, CodexDir, CentralHost, CentralPort, Port, DeviceName, BackupDir}`.
- Produces: `config.ReadState() (State, error)`, `config.WriteState(State) error`, and `config.Validate(State) error`.
- Produces: `App.GetBootstrap() (types.Bootstrap, error)` and `App.SaveSettings(types.SettingsRequest) (types.Bootstrap, error)`.

- [ ] **Step 1: Write failing state tests**

Use a package-level test override `stateDirOverride` so tests do not touch the user's home. Cover missing-file defaults, server/client validation, JSON round trip, `0600` config mode, and ignored legacy `last_peer_ip`:

```go
func TestReadStateReturnsClientDefaults(t *testing.T) {
    stateDirOverride = t.TempDir()
    t.Cleanup(func() { stateDirOverride = "" })
    got, err := ReadState()
    if err != nil { t.Fatal(err) }
    if got.Mode != "client" || got.CodexDir != "~/.codex" || got.CentralPort != 28888 {
        t.Fatalf("unexpected defaults: %#v", got)
    }
}

func TestValidateRequiresCenterForClient(t *testing.T) {
    err := Validate(State{Mode: "client", CodexDir: "~/.codex", CentralPort: 28888, DeviceName: "mac"})
    if err == nil || err.Error() != "central_host is required in client mode" { t.Fatalf("got %v", err) }
}
```

- [ ] **Step 2: Run the config tests and verify failure**

Run: `go test ./internal/config -run 'Test(ReadState|Validate)' -v`

Expected: FAIL because the new fields, override, and validation function do not exist.

- [ ] **Step 3: Implement state defaults and validation**

Use this exact state shape and normalize zero values after JSON decoding:

```go
type State struct {
    Mode        string `json:"mode"`
    CodexDir    string `json:"codex_dir"`
    CentralHost string `json:"central_host,omitempty"`
    CentralPort int    `json:"central_port,omitempty"`
    Port        int    `json:"port,omitempty"`
    DeviceName  string `json:"device_name"`
    BackupDir   string `json:"backup_dir,omitempty"`
}

func defaultState() State {
    hostname, _ := os.Hostname()
    return State{Mode: "client", CodexDir: "~/.codex", CentralPort: constants.DefaultPort, Port: constants.DefaultPort, DeviceName: hostname, BackupDir: "~/.agentpal/server/auth-backups"}
}
```

`Validate` accepts only `client` and `server`, requires `codex_dir` and `device_name`, requires `central_host` and a port in `1..65535` for clients, and requires a listen port and backup directory for servers. Add `Bootstrap` and `SettingsRequest` types containing JSON names used by the frontend.

- [ ] **Step 4: Expose bootstrap/settings through Wails**

Implement `GetBootstrap` by reading and validating state, expanding Codex/backup paths for display, and returning app version. Implement `SaveSettings` by mapping the request to `config.State`, validating, writing, and returning bootstrap. Do not dynamically change role in a running process; return `restartRequired: true` when mode, server port, or published Codex directory changes.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/config ./...`

Expected: PASS with no existing package regressions.

- [ ] **Step 6: Commit when authorized**

```bash
git add internal/config/config.go internal/config/config_test.go internal/types/types.go app.go
git commit -m "feat: add AgentPal role configuration"
```

### Task 2: Structured MCP And Authentication Config Operations

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Create: `internal/codex/mcp.go`
- Create: `internal/codex/mcp_test.go`
- Create: `internal/codex/auth_config.go`
- Create: `internal/codex/auth_config_test.go`
- Modify: `internal/codex/toml_sections.go`

**Interfaces:**
- Produces: `codex.MCPEntry{Name, TOML, SHA256}`.
- Produces: `codex.ReadMCP(path string) ([]MCPEntry, error)`.
- Produces: `codex.UpsertMCP(configPath string, entries []MCPEntry, deleteNames []string) error`.
- Produces: `codex.ExtractAuthConfig(configPath string) ([]byte, AuthSummary, error)`.
- Produces: `codex.MergeAuthConfig(targetPath string, fragment []byte) error`.

- [ ] **Step 1: Add failing MCP tests**

Test nested MCP tables, whitespace-independent hashes, selected upsert, selected delete, and preservation of `[projects]` and comments outside modified MCP sections:

```go
func TestReadMCPNormalizesEquivalentTables(t *testing.T) {
    a := writeTempConfig(t, `[mcp_servers.docs]
command = "npx"
args = ["server"]
`)
    b := writeTempConfig(t, `[mcp_servers.docs]
args=["server"]
command="npx"
`)
    left, _ := ReadMCP(a)
    right, _ := ReadMCP(b)
    if left[0].SHA256 != right[0].SHA256 { t.Fatal("equivalent MCP tables differ") }
}
```

- [ ] **Step 2: Add failing auth-fragment tests**

Define authentication root keys exactly as `model`, `model_provider`, `model_reasoning_effort`, `model_reasoning_summary`, and `model_verbosity`. Include only the selected `[model_providers.<model_provider>]` subtree. Assert that merging the fragment changes those values but preserves `[mcp_servers.*]`, `[projects.*]`, comments, and unrelated root keys.

- [ ] **Step 3: Run tests and verify failure**

Run: `go test ./internal/codex -run 'Test(ReadMCP|UpsertMCP|ExtractAuthConfig|MergeAuthConfig)' -v`

Expected: FAIL because the new APIs do not exist.

- [ ] **Step 4: Add the TOML parser and implement structured reads**

Run: `go get github.com/pelletier/go-toml/v2@v2.2.4`

Decode into `map[string]any`, obtain `mcp_servers`, canonicalize each named value with `toml.Marshal`, and hash canonical bytes. Generate an MCP fragment whose only top-level key is `mcp_servers` and one selected server. Reject non-table `mcp_servers` values with `mcp_servers must be a table`.

- [ ] **Step 5: Implement precise table replacement/deletion**

Extend the existing section parser with prefix-aware operations:

```go
func ReplaceTableSubtrees(target []byte, replacements map[string][]byte, deletePrefixes []string) ([]byte, error)
```

A prefix `mcp_servers.docs` owns that table and descendants such as `mcp_servers.docs.env`. Preserve all non-owned bytes in original order. Append new replacement blocks at the end with one blank line separator. Write through a same-directory temporary file with mode `0600`, then rename.

- [ ] **Step 6: Implement auth extraction and merge**

Use structured TOML only to identify `model_provider`; use root-assignment and table-subtree operations to preserve unrelated source text. Return `AuthSummary{Provider, Model}`. Reject a configured provider whose `[model_providers.<name>]` table is absent. Merge by replacing the fixed auth root keys and the selected provider subtree; do not remove unrelated provider tables.

- [ ] **Step 7: Run tests and format**

Run: `gofmt -w internal/codex && go test ./internal/codex ./...`

Expected: PASS.

- [ ] **Step 8: Commit when authorized**

```bash
git add go.mod go.sum internal/codex
git commit -m "feat: add structured Codex config operations"
```

### Task 3: Live Center Manifest And Read-Only Configuration API

**Files:**
- Create: `internal/central/manifest.go`
- Create: `internal/central/manifest_test.go`
- Create: `internal/central/server.go`
- Create: `internal/central/server_test.go`
- Modify: `internal/types/types.go`

**Interfaces:**
- Produces: `central.BuildManifest(codexDir string, now time.Time) (types.CenterManifest, error)`.
- Produces: `central.NewServer(types.ServerOptions) *Server`, `(*Server).Start() error`, `(*Server).Stop(context.Context) error`, and `(*Server).Handler() http.Handler`.
- HTTP: `GET /health`, `/v1/config/manifest`, `/v1/config/files/skills/<path>`, `/v1/config/mcp/<name>`.

- [ ] **Step 1: Write failing live-manifest tests**

Create a temporary Codex directory with two Skills, ignored files, two MCP servers, and one malformed Skill path case. Assert manifest protocol version `1`, generated UTC time, sorted resources, per-file hashes, per-Skill aggregate hash, MCP canonical hash, and scan errors without publishing invalid entries.

- [ ] **Step 2: Write failing HTTP tests**

Use `httptest.NewServer(server.Handler())`. After the first manifest request, modify a Skill and assert the second request returns a changed hash without restarting the server. Assert disabled paths and traversal return `404`/`400`, and MCP response hash matches the manifest.

- [ ] **Step 3: Run tests and verify failure**

Run: `go test ./internal/central -run 'Test(BuildManifest|Server)' -v`

Expected: FAIL because package `internal/central` does not exist.

- [ ] **Step 4: Define center protocol types**

Add these stable shapes to `internal/types/types.go`:

```go
type CenterManifest struct {
    Schema int `json:"schema"`; App string `json:"app"`; Version string `json:"version"`
    GeneratedAt time.Time `json:"generatedAt"`; Skills []CenterSkill `json:"skills"`
    MCP []CenterMCP `json:"mcp"`; Errors []ScanError `json:"errors,omitempty"`
}
type CenterSkill struct { Name, SHA256 string; Files []FileEntry; Size int64 }
type CenterMCP struct { Name, SHA256 string; Size int64 }
```

Use ordinary multiline declarations in implementation; the compact form above only fixes field names and JSON contract.

- [ ] **Step 5: Implement manifest generation and endpoints**

Build the manifest on every request. Serve only paths present in that request's freshly generated manifest. For MCP downloads return `application/toml; charset=utf-8`. Health returns role `server`, app version, port, Codex directory accessibility, Skills/MCP counts, backup counts initialized to zero, scan timestamp, and scan errors.

- [ ] **Step 6: Run package and full tests**

Run: `gofmt -w internal/central internal/types && go test ./internal/central ./...`

Expected: PASS.

- [ ] **Step 7: Commit when authorized**

```bash
git add internal/central internal/types/types.go
git commit -m "feat: serve live central configuration"
```

### Task 4: Center Client, Local Diff, And Transactional Apply

**Files:**
- Create: `internal/central/client.go`
- Create: `internal/central/client_test.go`
- Create: `internal/central/diff.go`
- Create: `internal/central/diff_test.go`
- Create: `internal/central/apply.go`
- Create: `internal/central/apply_test.go`
- Modify: `internal/types/types.go`

**Interfaces:**
- Produces: `central.Client.Health`, `Manifest`, `DownloadSkillFile`, and `DownloadMCP`.
- Produces: `central.Compare(codexDir string, manifest types.CenterManifest) (types.ConfigDiff, error)`.
- Produces: `central.Apply(ctx context.Context, client Client, req types.ApplyRequest) (types.ApplyResult, error)`.

- [ ] **Step 1: Write failing HTTP client tests**

Cover center URL normalization, six-second health/manifest timeout, schema/app validation, non-200 errors containing method/path/status, and streaming download without reading unbounded content into memory.

- [ ] **Step 2: Write failing diff tests**

Construct remote and local fixtures for all four states:

```go
const (
    DiffSame      = "same"
    DiffNew       = "new"
    DiffChanged   = "changed"
    DiffLocalOnly = "local_only"
)
```

Assert Skills compare by aggregate file hash and MCP compares by normalized TOML hash. The returned lists are sorted by resource type then name.

- [ ] **Step 3: Write failing apply/rollback tests**

Assert selected new/changed Skills and MCP apply, unselected and local-only resources remain, explicitly selected local-only resources delete, every remote hash is verified, a timestamped backup is created, and an injected replacement failure restores `skills/` and `config.toml` byte-for-byte.

- [ ] **Step 4: Run tests and verify failure**

Run: `go test ./internal/central -run 'Test(Client|Compare|Apply)' -v`

Expected: FAIL because the client/diff/apply APIs do not exist.

- [ ] **Step 5: Implement the client and diff engine**

Normalize host/port to one HTTP base URL, percent-escape path segments, cap JSON responses at 4 MiB, and validate schema `1`. `Compare` scans local Skills and MCP with the same canonical functions used by the server and includes `selected: true` for `new/changed`, `selected: false` for `local_only/same`.

- [ ] **Step 6: Implement transactional apply**

`ApplyRequest` contains `CodexDir`, `CentralHost`, `CentralPort`, and selected `{kind,name,action}` changes where action is `apply` or `delete`. Reject `delete` for non-`local_only` entries. Download to `os.MkdirTemp`, verify every file, create `~/.agentpal/backups/<timestamp>-config/`, copy only affected local paths plus `config.toml`, then apply. On any apply error, restore all affected paths from backup and return `RolledBack: true`.

- [ ] **Step 7: Run tests**

Run: `gofmt -w internal/central internal/types && go test ./internal/central ./...`

Expected: PASS.

- [ ] **Step 8: Commit when authorized**

```bash
git add internal/central internal/types/types.go
git commit -m "feat: preview and apply central configuration"
```

### Task 5: Local Authentication Profile Store And Creation

**Files:**
- Create: `internal/authprofile/store.go`
- Create: `internal/authprofile/store_test.go`
- Create: `internal/authprofile/import.go`
- Create: `internal/authprofile/import_test.go`
- Modify: `internal/types/types.go`

**Interfaces:**
- Produces: `authprofile.Store{Root string}` with `List`, `Get`, `Rename`, and `Delete`.
- Produces: `Store.ImportCurrent(codexDir, name string) (types.AuthProfile, error)`.
- Produces: `Store.CreateAPIKey(codexDir string, req types.APIKeyProfileRequest) (types.AuthProfile, error)`.

- [ ] **Step 1: Write failing secure-store tests**

Assert generated UUID directory names, sorted profile list, metadata round trip, `0700` directories, `0600` files, rename without directory rename, refusal to delete the active profile, and rejection of profile IDs containing path separators.

- [ ] **Step 2: Write failing import/API-key tests**

For current import, copy `auth.json`, extract only auth-related TOML, and record summary/source `local_import`. For API key creation, require non-empty name, key, model, provider, and valid HTTP(S) Base URL. Generate:

```json
{"OPENAI_API_KEY":"sk-test"}
```

and a TOML fragment containing `model`, `model_provider`, and `[model_providers.<provider>]` with `name`, `base_url`, `env_key = "OPENAI_API_KEY"`, and `wire_api = "responses"`.

- [ ] **Step 3: Run tests and verify failure**

Run: `go test ./internal/authprofile -run 'Test(Store|ImportCurrent|CreateAPIKey)' -v`

Expected: FAIL because package `authprofile` does not exist.

- [ ] **Step 4: Implement profile metadata and secure writes**

Use `github.com/google/uuid` already present in the module. Write `profile.json`, `auth.json`, and `config-auth.toml` to a temporary sibling directory, fsync/close files, chmod, then rename. Store active profile ID in `~/.agentpal/auth-profiles/active.json`; an imported current profile becomes active only when no active marker exists.

- [ ] **Step 5: Implement import and API-key profile creation**

Validate `auth.json` as a JSON object and `config-auth.toml` via the TOML parser before persisting. Never mutate the source Codex directory during creation.

- [ ] **Step 6: Run tests**

Run: `gofmt -w internal/authprofile internal/types && go test ./internal/authprofile ./...`

Expected: PASS.

- [ ] **Step 7: Commit when authorized**

```bash
git add internal/authprofile internal/types/types.go
git commit -m "feat: manage local authentication profiles"
```

### Task 6: Transactional Authentication Switching

**Files:**
- Create: `internal/authprofile/switch.go`
- Create: `internal/authprofile/switch_test.go`
- Modify: `internal/authprofile/store.go`

**Interfaces:**
- Produces: `Store.Activate(codexDir, profileID string) (types.ProfileSwitchResult, error)`.

- [ ] **Step 1: Write failing activation tests**

Test ChatGPT and API-key activation, preservation of MCP/Projects/unrelated root keys, backup path contents, active marker update only after success, invalid JSON/TOML rejection before backup, and rollback after an injected failure between auth replacement and config merge.

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./internal/authprofile -run TestActivate -v`

Expected: FAIL because `Activate` does not exist.

- [ ] **Step 3: Implement activation transaction**

Validate profile files, create `~/.agentpal/backups/<timestamp>-auth-switch/`, copy existing `auth.json` and `config.toml`, write the new auth to `auth.json.agentpal.tmp`, merge auth TOML into `config.toml.agentpal.tmp`, then rename both target files. If either rename fails, restore both originals and leave `active.json` unchanged. Return profile ID, display name, backup path, and rollback flag.

- [ ] **Step 4: Run tests**

Run: `gofmt -w internal/authprofile && go test ./internal/authprofile ./...`

Expected: PASS.

- [ ] **Step 5: Commit when authorized**

```bash
git add internal/authprofile
git commit -m "feat: switch Codex authentication profiles safely"
```

### Task 7: Plaintext Center Authentication Backup API

**Files:**
- Create: `internal/authprofile/archive.go`
- Create: `internal/authprofile/archive_test.go`
- Modify: `internal/central/server.go`
- Modify: `internal/central/server_test.go`
- Modify: `internal/central/client.go`
- Modify: `internal/central/client_test.go`
- Modify: `internal/types/types.go`

**Interfaces:**
- Produces: `authprofile.Pack(profileDir string, io.Writer) error` and `authprofile.Unpack(io.Reader, targetRoot string) (types.AuthProfile, error)`.
- HTTP: `GET /v1/auth-backups`, `POST/GET/DELETE /v1/auth-backups/<device>/<profile>`.
- Produces: client methods `ListBackups`, `UploadBackup`, `DownloadBackup`, and `DeleteBackup`.

- [ ] **Step 1: Write failing archive safety tests**

Use ZIP with only `profile.json`, `auth.json`, and `config-auth.toml`. Reject extra files, duplicate entries, absolute names, traversal, symlinks, files larger than 2 MiB, and archives whose total expanded content exceeds 4 MiB. Validate profile JSON/auth JSON/TOML before accepting.

- [ ] **Step 2: Write failing backup API tests**

Assert grouped metadata listing, create with `201`, duplicate without `overwrite=true` returns `409`, explicit overwrite succeeds, download returns an archive, delete returns `204`, unsafe device/profile path returns `400`, and request bodies over 5 MiB return `413`.

- [ ] **Step 3: Run tests and verify failure**

Run: `go test ./internal/authprofile ./internal/central -run 'Test(Pack|Unpack|Backup)' -v`

Expected: FAIL because archive and backup endpoints do not exist.

- [ ] **Step 4: Implement deterministic archive handling**

Sort the three file names, use fixed ZIP metadata where possible, and never extract by joining raw archive paths. Read each allowed entry into bounded memory, validate, then write a temporary profile directory with secure permissions and rename it into place.

- [ ] **Step 5: Implement server storage and center client methods**

Store backups under `<backup_dir>/<escaped-device>/<uuid>/`; keep user-facing device/profile names in validated `profile.json`, not as untrusted filesystem paths. The URL profile segment is the profile ID. Listing returns device name, profile ID/name/type/provider/model, updated time, and byte size only. Restore imports through `authprofile.Unpack` into local store and marks source `center_restore`; it never calls `Activate`.

- [ ] **Step 6: Run tests**

Run: `gofmt -w internal/authprofile internal/central internal/types && go test ./...`

Expected: PASS.

- [ ] **Step 7: Commit when authorized**

```bash
git add internal/authprofile internal/central internal/types/types.go
git commit -m "feat: back up authentication profiles to center"
```

### Task 8: Wails Lifecycle And Complete Backend API

**Files:**
- Modify: `app.go`
- Modify: `main.go`
- Create: `app_test.go`
- Modify: `frontend/src/wails.ts`

**Interfaces:**
- Produces Wails methods: `GetBootstrap`, `SaveSettings`, `RefreshConfiguration`, `ApplyConfiguration`, `ListAuthProfiles`, `ImportCurrentAuth`, `CreateAPIKeyProfile`, `RenameAuthProfile`, `DeleteAuthProfile`, `ActivateAuthProfile`, `ListRemoteBackups`, `UploadAuthProfile`, `RestoreAuthProfile`, `DeleteRemoteBackup`, `GetServerStatus`, `OpenFolder`, `CheckForUpdate`, and `OpenURL`.

- [ ] **Step 1: Write failing app orchestration tests**

Inject config reader, center client, profile store, and server factory into `App`. Assert server mode starts one center server during startup and stops it during shutdown; client mode starts no listener; center-offline refresh returns a structured connection state rather than preventing bootstrap; role-changing settings report restart required.

- [ ] **Step 2: Run tests and verify failure**

Run: `go test . -run TestApp -v`

Expected: FAIL because the injected dependencies and methods do not exist.

- [ ] **Step 3: Replace legacy Wails methods and lifecycle**

Remove `StartSharing`, `StopSharing`, `TestConnection`, `FetchRemoteManifest`, and `SyncFromPeer` from the Wails surface. `startup` reads config and starts `central.Server` only in server mode. Add `shutdown` and bind it from `main.go`. Keep backend errors explicit and frontend-safe.

- [ ] **Step 4: Update the TypeScript contract**

Define exact TypeScript counterparts for every Go boundary type. Group calls in a flat `api` object but do not duplicate business rules in TypeScript. Add a development-only unavailable-backend state when `window.go` is absent.

- [ ] **Step 5: Run tests and frontend typecheck**

Run: `gofmt -w app.go main.go app_test.go && go test ./... && npm run build`

Workdir for npm: `frontend/`

Expected: Go tests PASS and Vite build succeeds.

- [ ] **Step 6: Commit when authorized**

```bash
git add app.go app_test.go main.go frontend/src/wails.ts
git commit -m "feat: expose central client backend APIs"
```

### Task 9: Approved A2 Client UI And Server Status UI

**Files:**
- Modify: `frontend/src/App.tsx`
- Create: `frontend/src/ClientApp.tsx`
- Create: `frontend/src/ServerApp.tsx`
- Modify: `frontend/src/components.tsx`
- Modify: `frontend/src/style.css`
- Modify: `frontend/src/main.tsx`

**Interfaces:**
- Consumes all Wails APIs and types from Task 8.
- Produces client tabs `Configuration`, `Authentication`, and `Backups`, plus a separate server status shell.

- [ ] **Step 1: Build the role shell and connection states**

On mount call `GetBootstrap`; render loading, fatal bootstrap error, `ClientApp`, or `ServerApp`. In `ClientApp`, call `RefreshConfiguration` automatically. Show `connecting`, `connected`, and `offline` states; offline includes the backend error and Retry while all local Authentication actions remain enabled.

- [ ] **Step 2: Build Configuration preview and apply flow**

Default to Configuration. Display center host/port and Skills/MCP change counts. Open a review dialog with status filters and checkboxes; default-select new/changed and leave local-only/same unselected. Selecting local-only changes its action to delete and requires a confirmation sentence before apply. Display operation backup path and rollback result.

- [ ] **Step 3: Build Authentication flows**

List profiles with active state, type, provider, and model. Add dialogs for import current, API-key creation, rename, delete, and activation. API-key input uses `type="password"` and is never copied into notices or logs. Disable active-profile deletion. Activation confirmation names the profile and states that current auth/model settings will be backed up.

- [ ] **Step 4: Build Backups flows and plaintext warning**

Group remote backups by device. Add explicit Upload, Restore, and Delete confirmations. Restore dialog permits changing the local display name and states that restored profiles are not activated. Keep a persistent warning: `Authentication backups contain plaintext credentials and are intended only for a trusted local network.`

- [ ] **Step 5: Build top-right menu, version state, and ServerApp**

The menu contains center/Codex settings, Refresh, and Open backup folder. Put small fixed version text at bottom-right: `vX (latest)`, current version only on check failure, or `a newer version discovered · update`. ServerApp shows listener, published directory, Skills/MCP and backup counts, last scan/errors, rescan, and open-folder actions.

- [ ] **Step 6: Implement responsive styling**

Use cards only for individual resource/profile rows and dialogs, with radius at most `8px`. Keep the application shell unframed. At widths below `680px`, make tabs horizontally scrollable, stack dialog form rows, reserve bottom padding for version text, and guarantee menu/dialog text wraps without overlap. Use zero letter spacing and no viewport-scaled font sizes.

- [ ] **Step 7: Build and inspect generated output**

Run: `npm run build`

Workdir: `frontend/`

Expected: TypeScript and Vite build succeed with no errors.

- [ ] **Step 8: Commit when authorized**

```bash
git add frontend/src
git commit -m "feat: redesign AgentPal as central client"
```

### Task 10: Remove Peer-To-Peer Product Surface And Update Documentation

**Files:**
- Delete: `internal/share/manifest.go`
- Delete: `internal/share/manifest_test.go`
- Delete: `internal/share/server.go`
- Delete: `internal/share/snapshot.go`
- Delete: `internal/peer/client.go`
- Delete: `internal/peer/normalize.go`
- Delete: `internal/peer/normalize_test.go`
- Delete: `internal/sync/sync.go`
- Delete: `internal/sync/skills_test.go`
- Rewrite: `README.md`
- Delete: `IMPLEMENTATION_PLAN.md`

**Interfaces:**
- Removes all Share/Peer/Sync APIs after central replacement is green.
- Documents only the supported Server/Client configuration and trusted-LAN security model.

- [ ] **Step 1: Prove no production references remain**

Run: `rg 'StartSharing|StopSharing|TestConnection|FetchRemoteManifest|SyncFromPeer|ShareRequest|SyncRequest|RemoteManifest' --glob '!docs/superpowers/**'`

Expected: matches only in legacy files listed for deletion. If a new file matches, replace that dependency with `central`/`authprofile` before deleting anything.

- [ ] **Step 2: Delete legacy packages and obsolete frontend exports**

Delete the listed files. Remove legacy types from `internal/types/types.go` and unused resource checklist code from `frontend/src/components.tsx`. Run `go mod tidy` after deletion.

- [ ] **Step 3: Rewrite README around the new product**

Document installation, both `config.json` examples, Server startup behavior, Client tabs, center endpoints, profile locations, backup/rollback paths, plaintext credential warning, firewall port `28888`, and explicit non-goals. Remove point-to-point instructions and old GUI examples.

- [ ] **Step 4: Run full static verification**

Run: `gofmt -w . && go mod tidy && go vet ./... && go test ./... && npm run build`

Workdir for npm: `frontend/`

Expected: all commands succeed and `rg 'Share Codex Configuration|Connect To AgentPal|Peer IP' frontend README.md` returns no matches.

- [ ] **Step 5: Commit when authorized**

```bash
git add -A internal README.md IMPLEMENTATION_PLAN.md frontend/src go.mod go.sum
git commit -m "refactor: remove peer-to-peer sync workflow"
```

### Task 11: End-To-End Verification And Packaging

**Files:**
- Create: `internal/central/e2e_test.go`
- Modify: `internal/constants/constants.go`
- Modify: `build/darwin/Info.plist`
- Modify: `build/windows/info.json`

**Interfaces:**
- Verifies the complete center/client/auth-backup contract.
- Sets release version consistently to `0.2.0`.

- [ ] **Step 1: Add an end-to-end Go test**

Start a real center server on `127.0.0.1:0` with temporary source, backup, client Codex, and local profile directories. Exercise health, live manifest change, diff, selected apply, local-only preservation/deletion, API-key profile creation/activation, upload, listing, download restore into a second profile store, and failed-hash no-change behavior.

- [ ] **Step 2: Run focused and full tests**

Run: `go test ./internal/central -run TestEndToEnd -v && go test -race ./...`

Expected: PASS, including race detector.

- [ ] **Step 3: Update version metadata**

Set `constants.AppVersion`, macOS bundle short/build version, and Windows product/file version to `0.2.0`. Assert `/health`, bootstrap, and right-bottom frontend status all read the backend version rather than duplicating it.

- [ ] **Step 4: Build the desktop application**

Run: `npm run build`

Workdir: `frontend/`

Run: `wails build`

Workdir: repository root.

Expected: frontend and Wails desktop build succeed and produce the platform application under `build/bin/`.

- [ ] **Step 5: Perform browser UI verification**

Run `npm run dev -- --port 5173` from `frontend/`, open `http://127.0.0.1:5173`, and use a development backend fixture or Wails dev runtime. Verify at `980x720` and `390x844`: correct A2 tab shell, no overlap, menu/dialog operation, offline state, diff selection defaults, plaintext warning, profile forms, ServerApp, and bottom-right version status. Check browser console and require zero application errors.

- [ ] **Step 6: Perform two-instance Wails verification**

Use separate temporary HOME directories and configs: one `mode=server`, one `mode=client`. Verify automatic connection, live Skills/MCP refresh, apply with backup, auth switch preserving MCP/Projects, backup upload, restore as inactive profile, and center-offline local profile operation. Record exact commands and outcomes in the release notes or final task report.

- [ ] **Step 7: Run final clean verification**

Run: `git diff --check && go vet ./... && go test -race ./... && npm run build && wails build`

Expected: every command succeeds. Inspect `git status --short` and ensure only intended source/docs/metadata changes are present; do not stage `.superpowers/`, build outputs, credentials, test HOME directories, or user configuration.

- [ ] **Step 8: Commit when authorized**

```bash
git add internal/central/e2e_test.go internal/constants/constants.go build/darwin/Info.plist build/windows/info.json
git commit -m "release: prepare AgentPal v0.2.0"
```
