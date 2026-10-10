# AgentPal Current Auth Profile Auto-Import Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Automatically persist the authentication currently used by local Codex in AgentPal's profile list and mark the exact persisted content current whenever the list is loaded.

**Architecture:** Add a store-level reconciliation transaction that serializes list reconciliation, builds one normalized snapshot from Codex `auth.json` plus the complete active-provider config fragment, and compares its canonical content hash with saved profiles. It creates a missing profile or replaces a same-name local profile through a validated staging/backup transaction, then reloads and verifies the exact persisted hash before `App.ListAuthProfiles` returns it to React.

**Tech Stack:** Go 1.23, Wails v2, React, TypeScript, Vite, `go-toml/v2`

## Global Constraints

- Reconcile on every `ListAuthProfiles()` call.
- Name API Key profiles with `model_provider`; name ChatGPT profiles with ChatGPT User ID.
- API Key takes precedence when API Key and ChatGPT token coexist.
- Preserve a same-name local profile's ID, `CreatedAt`, and `Source` while replacing authentication content.
- Never overwrite a `source=central_login` profile during automatic import.
- Keep `GetCurrentAuthStatus()` and `InspectCurrent()` free of write side effects.
- Preserve the complete active provider fragment, including nested tables and headers; never copy MCP, Projects, model settings, or unrelated providers.
- Keep existing backend manual import APIs for compatibility, but remove all frontend manual Import controls.
- Do not commit unless the user explicitly requests a commit.

---

## File Map

- Create `internal/authprofile/ensure.go`: snapshot construction, exact matching, serialized reconciliation, and validated profile replacement.
- Create `internal/authprofile/ensure_test.go`: creation, exact no-op, credential rotation, replacement, precedence, collision, and insufficient-config tests.
- Modify `internal/authprofile/store.go`: exclude hidden reconciliation staging and backup directories from normal profile listing.
- Modify `app.go`: consume reconciled profiles from the store.
- Modify `app_test.go`: verify list loading imports and selects while status inspection remains read-only.
- Modify `frontend/src/App.tsx`: delete the unused legacy authentication component and remove manual Import UI from the active component.

### Task 1: Build and Exactly Match the Current Authentication Snapshot

**Files:**
- Create: `internal/authprofile/ensure.go`
- Create: `internal/authprofile/ensure_test.go`
- Reuse: `internal/authprofile/current.go`
- Reuse: `internal/authprofile/content.go`
- Reuse: `internal/codex/auth_config.go`

**Interfaces:**
- Produces `type currentSnapshot struct { name, profileType, provider string; authJSON, configTOML []byte; contentHash string; status CurrentStatus }`.
- Produces `func readCurrentSnapshot(codexDir string) (currentSnapshot, bool, error)`. The Boolean is false only when no recognizable credentials exist.
- Produces `func exactCurrentStatus(snapshot currentSnapshot, profiles []AuthProfile) CurrentStatus`.

- [ ] **Step 1: Write failing snapshot tests**

Create `internal/authprofile/ensure_test.go` with a `writeCurrentAuth` helper and these tests:

```go
func TestReadCurrentSnapshotNormalizesAPIKeyAndPreservesProviderFragment(t *testing.T) {
    codexDir := t.TempDir()
    writeCurrentAuth(t, codexDir,
        `{"tokens":{"id_token":"header.eyJzdWIiOiJ1c2VyLTEyMyJ9.sig"},"OPENAI_API_KEY":"sk-current-secret"}`,
        "model = \"gpt-5\"\nmodel_provider = \"corp\"\n\n[model_providers.corp]\nbase_url = \"https://corp.example/v1\"\nwire_api = \"responses\"\n\n[model_providers.corp.headers]\nX-Team = \"platform\"\n\n[mcp_servers.docs]\ncommand = \"npx\"\n",
    )

    snapshot, ok, err := readCurrentSnapshot(codexDir)
    if err != nil || !ok {
        t.Fatalf("ok=%v err=%v", ok, err)
    }
    if snapshot.name != "corp" || snapshot.profileType != "api_key" {
        t.Fatalf("snapshot=%#v", snapshot)
    }
    if !strings.Contains(string(snapshot.configTOML), "[model_providers.corp.headers]") || strings.Contains(string(snapshot.configTOML), "mcp_servers") || strings.Contains(string(snapshot.configTOML), "model =") {
        t.Fatalf("config=%s", snapshot.configTOML)
    }
    if string(snapshot.authJSON) != `{"OPENAI_API_KEY":"sk-current-secret"}` {
        t.Fatalf("auth=%s", snapshot.authJSON)
    }
}
```

Add `TestReadCurrentSnapshotUsesConfigAPIKeyBeforeChatGPT`, where the key exists only in the active provider's config table and a ChatGPT token also exists; assert type `api_key`. Add `TestReadCurrentSnapshotIgnoresKeysOutsideActiveProvider`, where an unrelated provider and MCP/header values contain `sk-...`; assert those values are ignored. Add `TestReadCurrentSnapshotPrefersActiveProviderKey`, where active and unrelated providers both have keys; assert the active one is stored. Add `TestReadCurrentSnapshotNamesChatGPTWithUserID`, with no API Key; assert name `user-123`, type `chatgpt`, and byte-preserved auth JSON.

- [ ] **Step 2: Run snapshot tests and verify RED**

Run: `go test ./internal/authprofile -run 'TestReadCurrentSnapshot' -count=1`

Expected: build failure because `readCurrentSnapshot` is undefined.

- [ ] **Step 3: Implement snapshot construction**

In `ensure.go`:

1. Read and parse `auth.json`; tolerate a missing file as empty, matching `InspectCurrent`.
2. Read and parse `config.toml`.
3. Resolve API Key auth-first, then search only `providerTable(config, summary.Provider)`; never call `findAPIKey` on the complete config map.
4. Resolve ChatGPT User ID and expiry with `findUserID` and `findExpiresAt`.
5. Return `(zero, false, nil)` when neither credential identity exists.
6. Call `codex.ExtractAuthConfigFromBytes(configData)`; propagate missing provider/provider-table errors consistently for both types.
7. For API Key, marshal exactly `map[string]any{"OPENAI_API_KEY": key}` and name it with `summary.Provider`.
8. For ChatGPT, retain original auth bytes and name it with User ID.
9. Compute `ComputeContentHash(authJSON, configFragment)`.
10. Fill `CurrentStatus` summary fields without assigning `CurrentProfileID` yet.

- [ ] **Step 4: Add and implement exact matching**

Write `TestExactCurrentStatusRequiresMatchingContentHash`: two profiles share provider, masked key identity, and Base URL, but only one has `ContentHash == snapshot.contentHash`; assert only that profile ID is returned. Write a ChatGPT token-rotation variant where User ID stays the same but the hash changes; assert no current ID.

Implement:

```go
func exactCurrentStatus(snapshot currentSnapshot, profiles []AuthProfile) CurrentStatus {
    status := snapshot.status
    for _, profile := range profiles {
        if profile.ContentHash != snapshot.contentHash {
            continue
        }
        status.CurrentProfileID = profile.ID
        status.APIKeyImported = snapshot.profileType == "api_key"
        status.ChatGPTImported = snapshot.profileType == "chatgpt"
        break
    }
    return status
}
```

- [ ] **Step 5: Run Task 1 tests**

Run: `go test ./internal/authprofile -run 'Test(ReadCurrentSnapshot|ExactCurrentStatus)' -count=1`

Expected: PASS.

### Task 2: Reconcile Missing and Rotated Profiles Atomically

**Files:**
- Modify: `internal/authprofile/ensure.go`
- Modify: `internal/authprofile/ensure_test.go`
- Modify: `internal/authprofile/store.go:41-64`
- Reference: `internal/authprofile/central.go:108-140`

**Interfaces:**
- Produces `func (s *Store) ReconcileCurrent(codexDir string) ([]AuthProfile, CurrentStatus, error)`.
- Uses package-level `var reconcileMu sync.Mutex` to serialize the full list/read/match/write/reload sequence in this desktop process.
- Produces `func (s *Store) writeCurrentSnapshot(snapshot currentSnapshot, existing *AuthProfile) (AuthProfile, error)`.

- [ ] **Step 1: Write failing creation and exact no-op tests**

Add:

- `TestReconcileCurrentCreatesAPIKeyUsingProviderName`: assert one `manual`/`api_key` profile named `corp`, exact nested provider headers preserved, and returned `CurrentProfileID` equals its ID.
- `TestReconcileCurrentCreatesChatGPTUsingUserID`: assert one `local_import`/`chatgpt` profile named `user-123` and selected.
- `TestReconcileCurrentDoesNotRewriteExactProfile`: reconcile twice, compare `profile.json` bytes and `UpdatedAt`, and assert the second call does not alter either.
- `TestReconcileCurrentDoesNothingWithoutCredentials`: `{}` auth creates no profile.

- [ ] **Step 2: Run creation tests and verify RED**

Run: `go test ./internal/authprofile -run 'TestReconcileCurrent(Creates|Does)' -count=1`

Expected: build failure because `ReconcileCurrent` is undefined.

- [ ] **Step 3: Implement serialized creation and verification**

Implement `ReconcileCurrent` as one locked transaction:

```go
reconcileMu.Lock()
defer reconcileMu.Unlock()

profiles, err := s.List()
// Read snapshot. If no credentials, return profiles and InspectCurrent status.
// Return immediately when exactCurrentStatus supplies CurrentProfileID.
// Collect every case-sensitive snapshot.name match before writing.
// Reject if any matching profile is Central or more than one local match exists.
// Update the one local match or create a new profile.
// Reload Codex snapshot and profiles, then rerun exactCurrentStatus.
// Retry the whole reconciliation once if Codex changed during the write.
// Return "current Codex authentication changed during reconciliation" if it changes twice.
```

For creation, generate a UUID and metadata using one timestamp. API Key source is `manual`; ChatGPT source is `local_import`. Pass the snapshot bytes to `writeCurrentSnapshot` so creation and replacement preserve identical content.

- [ ] **Step 4: Write failing same-name replacement tests**

Add `TestReconcileCurrentReplacesRotatedSameNameProfile`: create stale `corp`, record ID, source, and `CreatedAt`, then reconcile a new key and provider headers under the same provider. Assert list length remains one; ID, source, and `CreatedAt` are unchanged; `UpdatedAt` advances; raw auth/config equal the new snapshot; and it is selected.

Add `TestReconcileCurrentReplacesRotatedChatGPTToken`: import an old token for `user-123`, change only token content while retaining User ID, reconcile, and assert same ID with new exact hash. This proves identity-only matching does not suppress refresh.

Add `TestReconcileCurrentRejectsSameNameCentralProfile`: create a Central cache named `corp`, record all three files, reconcile a different current `corp`, assert error `cannot automatically replace Central profile "corp"`, and assert the files are unchanged.

Add `TestReconcileCurrentRejectsAmbiguousLocalName`: create two non-Central profiles named `corp`, reconcile a different current `corp`, assert `multiple local profiles use generated name "corp"`, and assert neither profile changes. Add a mixed local/Central duplicate test and assert the Central conflict wins regardless of creation order. Add `Corp` versus `corp` coverage to prove matching is case-sensitive.

- [ ] **Step 5: Run replacement tests and verify RED**

Run: `go test ./internal/authprofile -run 'TestReconcileCurrent(Replaces|Rejects)' -count=1`

Expected: FAIL until same-name replacement is implemented.

- [ ] **Step 6: Implement validated staged replacement**

In `writeCurrentSnapshot`:

1. Reject `existing.Source == "central_login"` before creating temporary files.
2. For replacement, copy metadata and preserve ID, `CreatedAt`, and `Source`; update `Name`, `Type`, `Provider`, `UpdatedAt`; clear `ReservedName`; let `writeProfile` recompute `ContentHash`.
3. For creation, create new metadata with UUID, source by type, and matching creation/update timestamps.
4. Create a hidden staging directory under `filepath.Dir(finalDir)` using prefix `.agentpal-auth-import-` and a hidden backup path using prefix `.agentpal-auth-backup-`.
5. Call `writeProfile(staging, metadata, snapshot.authJSON, snapshot.configTOML)` and `readSavedProfile(staging)` before any rename.
6. For creation, rename staging directly to the UUID final directory.
7. For replacement, rename final to a unique backup, rename staging to final, then call `readSavedProfile(final)`.
8. If the second rename or post-install read fails, remove the failed final path, rename backup back to final, and verify it with `readSavedProfile`; combine the initiating and rollback errors if restoration fails.
9. Remove backup only after post-install validation succeeds. Treat backup cleanup failure as non-fatal because the new profile is already committed and valid.
10. Update `Store.List` to ignore directories whose names start with `.agentpal-auth-import-` or `.agentpal-auth-backup-`, so interrupted transaction artifacts never appear as profiles.

- [ ] **Step 7: Add insufficient-config and concurrent-call tests**

Add `TestReconcileCurrentRejectsMissingProviderTable` for both API Key and ChatGPT cases; assert no profile directory is created. Add `TestReconcileCurrentConcurrentCallsCreateOneProfile`: start two goroutines against the same store/Codex directory, wait for both, and assert both return no error and the store contains exactly one profile. Add `TestStoreListIgnoresReconcileArtifacts` with valid-looking profiles under both hidden transaction prefixes and assert neither is returned.

Add an internal `reconcileCurrent(codexDir string, afterWrite func())` implementation used by public `ReconcileCurrent(codexDir)` with a nil hook. In `TestReconcileCurrentRetriesWhenCodexChanges`, use the hook to replace Codex auth after the first profile write; assert reconciliation retries and returns the second credential selected. In `TestReconcileCurrentRejectsRepeatedCodexChanges`, mutate on both attempts and assert the explicit instability error.

- [ ] **Step 8: Run all authprofile tests**

Run: `go test ./internal/authprofile -count=1`

Expected: PASS.

### Task 3: Reconcile in `ListAuthProfiles` Only

**Files:**
- Modify: `app.go:173-220`
- Modify: `app_test.go`

**Interfaces:**
- Consumes `(*authprofile.Store).ReconcileCurrent(codexDir string) ([]AuthProfile, CurrentStatus, error)`.
- Keeps `GetCurrentAuthStatus()` on existing `List` plus read-only `InspectCurrent` behavior.

- [ ] **Step 1: Write failing App integration tests**

Add a helper that sets isolated `HOME` and `AGENTPAL_CONFIG_DIR`, writes app state, and creates Codex auth files. Then add:

- `TestListAuthProfilesImportsAndSelectsCurrentAPIKey`: assert exactly one returned profile named `corp` with `Active == true`.
- `TestListAuthProfilesUpdatesSameNameAndSelectsIt`: pre-create stale `corp`, assert returned list remains length one, preserves ID, contains current Base URL, and marks it active.
- `TestGetCurrentAuthStatusDoesNotImportProfile`: call only status, then assert profile store contains zero profile directories.

- [ ] **Step 2: Run App tests and verify RED**

Run: `go test . -run 'Test(ListAuthProfiles|GetCurrentAuthStatusDoesNotImportProfile)' -count=1`

Expected: the list-import tests FAIL while the read-only status test passes.

- [ ] **Step 3: Replace list/inspect flow with reconciliation**

In `ListAuthProfiles`, replace its direct `store.List()` plus `InspectCurrent()` block with:

```go
profiles, status, err := authprofile.NewStore(root).ReconcileCurrent(dir)
if err != nil {
    return nil, err
}
return profileTypesFromStore(profiles, status), nil
```

Do not change `GetCurrentAuthStatus`.

- [ ] **Step 4: Run App tests and verify GREEN**

Run: `go test . -run 'Test(ListAuthProfiles|GetCurrentAuthStatusDoesNotImportProfile)' -count=1`

Expected: PASS.

### Task 4: Remove Manual Import UI

**Files:**
- Modify: `frontend/src/App.tsx:2,58-67,69-70,159-169,246-260`

**Interfaces:**
- Consumes the reconciled `profiles` returned by `ListAuthProfiles()`.
- Leaves backend bindings in `frontend/src/wails.ts` unchanged for compatibility.

- [ ] **Step 1: Delete the unused legacy component**

Delete the entire unused `AuthenticationTab` function at `App.tsx:58-67`. It is not rendered; `ClientApp` renders `AuthenticationTabCompact`.

- [ ] **Step 2: Remove active-component import behavior**

From `AuthenticationTabCompact`, remove `current` state, its `GetCurrentAuthStatus` effect, `importCurrentAPIKey`, `importCurrentChatGPT`, `hasCurrentProfile`, and both `.current-auth` banners. Remove the now-unused `CurrentAuthStatus` type import.

- [ ] **Step 3: Build and scan the frontend**

Run `npm run build` in `frontend`; expect success. Then run:

```bash
rg -n 'importCurrentAPIKey|importCurrentChatGPT|Current API Key imported|Current ChatGPT login imported|>Import<' frontend/src/App.tsx
```

Expected: no matches.

### Task 5: Full Verification

**Files:**
- Verify only; leave unrelated dirty-worktree changes untouched.

- [ ] **Step 1: Format changed Go files**

Run: `gofmt -w app.go app_test.go internal/authprofile/ensure.go internal/authprofile/ensure_test.go`

- [ ] **Step 2: Run backend verification**

Run: `go test ./... -count=1`

Expected: every package passes.

- [ ] **Step 3: Run frontend verification**

Run `npm run build` in `frontend`.

Expected: `tsc && vite build` succeeds.

- [ ] **Step 4: Check patch integrity and scope**

Run: `git diff --check`

Expected: no whitespace errors.

Inspect:

```bash
git diff -- app.go app_test.go internal/authprofile/ensure.go internal/authprofile/ensure_test.go frontend/src/App.tsx docs/superpowers/specs/2026-10-09-agentpal-auto-import-current-auth-profile-design.md docs/superpowers/plans/2026-10-09-agentpal-auto-import-current-auth-profile.md
```

Expected: only approved auto-import behavior, tests, UI cleanup, spec, and plan are present.
