# AgentPal Unified Auth Profile List Implementation Plan

> **For agentic workers:** Execute this plan task-by-task in the current AgentPal workspace. Keep pre-existing uncommitted changes intact and verify each task before moving on. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Merge Central and Local authentication profiles into one local profile list, cache complete Central profiles with a source marker, preserve their usability after logout, and give every row a consistent icon-led selection layout.

**Architecture:** The existing auth profile store remains the local source of truth. A logged-in client synchronizes Central profile list/detail responses into that store, updating only entries marked `central_login`; the same list API then serves the unified UI whether logged in or offline. Central profile activation and conflict preview use the same local profile ID path as manual profiles.

**Tech Stack:** Go 1.22+, Wails v2.12, React, TypeScript, Vite, Go `httptest`, existing profile store and Codex config merge utilities.

## Global Constraints

- Keep the local profile root at `~/.agentpal/auth-profiles/<id>/`.
- Mark Central cached profiles with `source: "central_login"` and `reservedName` equal to the remote profile name.
- Cache Central `auth.json` and `config-auth.toml` using the existing 0600 file mode and atomic profile write path.
- Logout clears the Central login session only; it must not delete cached Central profiles.
- Local deletion of a Central cached profile must not call a Central delete endpoint.
- Preserve existing provider conflict confirmation, Codex configuration merge, backup and rollback behavior.
- Never display complete credential values; only existing masked API key previews may be shown.
- Do not overwrite a same-named manual profile when synchronizing a Central profile.

---

## File Map

- `app.go`: list filtering, authentication state, Central sync orchestration, login/logout, profile activation APIs.
- `app_test.go`: app-level Central sync/logout/list behavior with isolated home/config directories and `httptest`.
- `internal/authprofile/central.go`: add/upsert/find operations for Central-owned profile records.
- `internal/authprofile/central_test.go`: store-level creation, update, hash, validation and source ownership behavior.
- `internal/authprofile/store.go`: existing list/delete/raw-content APIs; change only if unified list handling requires it.
- `internal/authprofile/switch.go`: existing ID-based preview and activation, used for all sources.
- `frontend/src/App.tsx`: one profile collection, sync lifecycle, source-aware row actions and unified row markup.
- `frontend/src/wails.ts`: Wails backend and UI result types.
- `frontend/wailsjs/go/main/App.js`, `frontend/wailsjs/go/main/App.d.ts`, `frontend/wailsjs/go/models.ts`: generated Wails binding declarations for restored APIs/types.
- `frontend/src/style.css`: shared row grid, fixed aligned action column, type icons, source badge, selected/unselected states and responsive layout.

## Task 1: Make Central cache store safely upsert by remote name

**Files:**
- Modify: `internal/authprofile/central.go`
- Test: `internal/authprofile/central_test.go`

**Interfaces:**
- `(*Store).FindByReservedName(name string) (AuthProfile, bool, error)` remains the lookup interface.
- Add `(*Store).SyncCentral(c CentralProfile) (AuthProfile, string, error)`. It returns the saved profile and a status: `added`, `synced`, or `needs_sync`.
- `SyncCentral` may update only a profile whose `Source == "central_login"` and `ReservedName == c.Name`; it must not modify a manual profile with the same visible name.

- [ ] **Step 1: Write store tests first**

Add tests that:

1. Syncing a missing Central profile creates it with source `central_login`, reserved name, content hash and full auth/config files.
2. Syncing identical content returns `synced` and retains the profile ID.
3. Syncing changed auth/config returns `needs_sync`, updates both files and retains the profile ID.
4. A same-named manual profile is not overwritten; the sync returns a conflict error.
5. Invalid metadata/config does not change an existing cached profile.

- [ ] **Step 2: Run the focused tests and observe failure**

Run: `go test ./internal/authprofile -run 'TestSyncCentral' -count=1`

Expected: compilation/test failure because `SyncCentral` does not yet exist.

- [ ] **Step 3: Implement atomic upsert**

Validate with `validateCentralProfile`. Find by reserved name. If found, require `Source == "central_login"`, compute/compare content hash and return `synced` without writes when hashes match. For changed content, write a complete profile to a temporary sibling directory and atomically replace the old profile while preserving ID and creation time. For a missing entry, call the existing `AddCentral` path and return `added`.

- [ ] **Step 4: Run store tests**

Run: `go test ./internal/authprofile -count=1`

Expected: PASS, including the existing `AddCentral`, `OverwriteCentral` and content-hash tests.

## Task 2: Restore login-gated Central synchronization and local-first activation

**Files:**
- Modify: `app.go`
- Modify: `internal/authprofile/central.go` only if Task 1 interface needs a narrow helper
- Test: `app_test.go`

**Interfaces:**
- `(*App).SyncCentralAuthProfiles() (types.AuthSyncReport, error)` lists remote metadata, fetches profile details and invokes `Store.SyncCentral`.
- `(*App).ListAuthProfiles() ([]types.AuthProfile, error)` returns manual and Central cached profiles, marking active state from `active.json`/current config as existing code does.
- `(*App).PreviewAuthProfile(id string)` and `(*App).ActivateAuthProfileWithConflict(id string, allowProviderConflict bool)` stay local-store operations.
- `(*App).PreviewCentralAuthProfile` and `(*App).ActivateCentralAuthProfile` are removed after the UI migrates to ID-based local operations.

- [ ] **Step 1: Add app-level failing tests**

Use a local `httptest.Server` with `/v1/auth-profiles` and `/v1/auth-profiles/<name>` responses. Isolate `HOME` and AgentPal config directory in `t.TempDir()`. Cover logged-out sync rejection, successful cache of complete auth/config, changed-hash update, local list containing both profile sources, and logout preserving cached entries.

- [ ] **Step 2: Run app tests and observe failure**

Run: `go test . -run 'TestCentralProfileSync|TestLogoutPreservesCentralProfiles' -count=1`

Expected: FAIL because app sync orchestration is absent and `ListAuthProfiles` filters/deletes Central cache.

- [ ] **Step 3: Implement synchronization orchestration**

Restore `SyncCentralAuthProfiles`. Read the client configuration and login state; reject empty/expired session. Fetch metadata via `central.Client.CentralAuthProfiles`. For each item, fetch detail with `CentralAuthProfile`, marshal `detail.Auth`, construct `authprofile.CentralProfile`, then call `Store.SyncCentral`. Continue past a single detail error and include an `error` item with detail while preserving its previous cache. If listing fails, return an error without changing local records.

- [ ] **Step 4: Remove destructive and realtime-only behavior**

Remove `RemoveCentralProfiles()` invocation from `ListAuthProfiles`; do not delete old cached entries. Return all valid store profiles. Remove realtime-only Central preview/activation APIs and their helper functions. Route all activation through `PreviewAuthProfile` / `ActivateAuthProfileWithConflict`, which read cached files by local ID. Keep `LogoutCentralLogin` limited to clearing login state.

- [ ] **Step 5: Run focused Go tests**

Run: `go test . ./internal/authprofile ./internal/central -count=1`

Expected: PASS; after logout, Central entries remain listed and cached activation uses local files without an HTTP request.

## Task 3: Consolidate authentication UI to a single source-aware profile list

**Files:**
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/wails.ts`
- Modify generated files: `frontend/wailsjs/go/main/App.js`, `frontend/wailsjs/go/main/App.d.ts`, `frontend/wailsjs/go/models.ts`

**Interfaces:**
- UI calls `ListAuthProfiles(): Promise<AuthProfile[]>` for the sole list.
- UI calls `SyncCentralAuthProfiles(): Promise<AuthSyncReport>` only when authenticated or on explicit refresh.
- Each `AuthProfile` row uses `source`, `reservedName`, and `contentHash` already returned by Go.
- Central activation uses `PreviewAuthProfile(id)` and `ActivateAuthProfileWithConflict(id, allowProviderConflict)`.

- [ ] **Step 1: Define sync result UI state**

Use `AuthSyncReport.items` keyed by remote profile name. Keep local profiles visible while a sync runs or fails. Add a loader that calls `SyncCentralAuthProfiles`, stores row statuses, then calls `ListAuthProfiles` to refresh the list.

- [ ] **Step 2: Change authentication lifecycle**

On page initialization, first load `ListAuthProfiles` and `GetCentralLoginState`; if logged in, run sync and reload. On successful login run sync then reload. On logout clear login state and sync status, call `ListAuthProfiles`, and leave cached rows intact. Manual refresh is shown only while logged in.

- [ ] **Step 3: Replace split sections with one profile collection**

Remove the Central profile state/list/empty-state section and remove `ListCentralAuthProfiles`, `PreviewCentralAuthProfile`, and `ActivateCentralAuthProfile` from frontend calls/types. Use the same `.profile-row` markup for all profiles. Keep the section heading `Local profiles` and the create-local-profile action.

- [ ] **Step 4: Make row actions source-aware**

All inactive profiles use the existing ID-based preview and activation flow. Show Edit only for eligible manual API-key profiles. Allow Delete for inactive Central cached profiles through `DeleteAuthProfile`; this deletes only the local entry. Show “Update from server” for Central rows only when logged in and the sync status is `needs_sync`; invoke `ApplyCentralAuthProfile(name)` and reload. Render per-row error/drift status from sync results.

- [ ] **Step 5: Update Wails frontend bindings**

Restore the generated method declarations/wrappers/models for `SyncCentralAuthProfiles` and `ApplyCentralAuthProfile`; remove wrappers for realtime Central list/preview/activation methods. Ensure `AuthSyncItem` and `AuthSyncReport` generated model fields match Go JSON names and optional fields.

- [ ] **Step 6: Run frontend typecheck/build**

Run from `frontend`: `npm run build`

Expected: TypeScript typecheck passes and Vite writes the production bundle.

## Task 4: Unify profile row information, icons, alignment and selection states

**Files:**
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/style.css`

**Interfaces:**
- `profile.type` selects the type icon (`api_key` → key; `chatgpt` → chat/person).
- `profile.source === "central_login"` adds cloud icon and `Central` source badge.
- Profile action area keeps the existing Current, select, and more-actions behaviors.

- [ ] **Step 1: Create a shared row renderer**

Render both source types with a common profile-row structure: fixed-width type icon, flexible main content with profile name as title, source badge, key summary, and fixed-width right action group. API key subtitle includes provider, masked key and base URL; ChatGPT subtitle includes provider, user ID and expiry.

- [ ] **Step 2: Add source/type icons and accessible labels**

Use inline SVG components with `aria-hidden="true"` for decorative type/source marks. Give selection controls accessible labels such as `Use <profile name>` or `Current profile: <profile name>`.

- [ ] **Step 3: Align action controls and styles**

Set every row to a shared grid/flex layout with `align-items: center`; reserve a consistent action column width so controls align vertically. Selected rows get a light background, accent border and Current badge; unselected rows keep neutral background and an enabled select button. Preserve responsive wrapping at the existing 680px breakpoint without moving controls into inconsistent positions.

- [ ] **Step 4: Verify frontend build**

Run from `frontend`: `npm run build`

Expected: PASS with no TypeScript or Vite errors.

## Task 5: End-to-end regression, package and install

**Files:**
- Verify: all files listed above
- No unrelated edits to existing working-tree changes

- [ ] **Step 1: Run full Go tests**

Run at repository root: `go test ./...`

Expected: all Go packages pass.

- [ ] **Step 2: Run frontend production build and diff validation**

Run: `cd frontend && npm run build`

Then run at repository root: `git diff --check`

Expected: both pass.

- [ ] **Step 3: Build macOS arm64 application**

Run: `/Users/yp-mac-02/go/bin/wails build -platform darwin/arm64 -skipbindings`

Expected: `build/bin/AgentPal.app` is packaged successfully. `-skipbindings` is used because the Wails bindings were updated alongside the Go APIs.

- [ ] **Step 4: Verify app artifact and install**

Run: `file build/bin/AgentPal.app/Contents/MacOS/AgentPal` and confirm `Mach-O 64-bit executable arm64`. Then install with `ditto build/bin/AgentPal.app /Applications/AgentPal.app`, and verify the installed binary architecture and app signature with `file` and `codesign -dv --verbose=2 /Applications/AgentPal.app`.

Expected: `/Applications/AgentPal.app` contains the newly built arm64 app bundle.

## Self-Review

- Spec sections 1–3 are covered by Tasks 1–3: cache complete Central credentials, login-gated sync, preservation after logout, and no collisions with manual profiles.
- Spec sections 4 and 6 are covered by Tasks 1–3: store upsert, app orchestration, per-profile sync errors, session checks and local-first activation.
- Spec sections 5 and 7 are covered by Tasks 3–5: one list, Central tag, type icons, key summary, aligned actions, state styles and build/test verification.
- Interfaces are consistent: `SyncCentralAuthProfiles` produces `AuthSyncReport`; UI keys row status by name; activation/preview use local IDs; refresh uses `ApplyCentralAuthProfile(name)`.
- No placeholders remain. The plan does not commit changes because no commit was requested.
