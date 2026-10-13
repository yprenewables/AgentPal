# AgentPal Central Profile Switch Selection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ensure a confirmed switch to a Central profile selects that Central profile after list reload without creating a derived `(Local)` profile.

**Architecture:** Keep `active.json` as evidence of the last profile successfully activated by AgentPal. When exact profile hashing fails because activation normalized the Provider fragment, `ReconcileCurrent` will normalize only the active Central profile with the same functions used during activation, compare that normalized content to the current Codex snapshot, and select it on a match. Existing local profiles remain untouched.

**Tech Stack:** Go, Wails application backend, `pelletier/go-toml`, Go `testing`, React/TypeScript build verification

## Global Constraints

- Do not change the frontend confirmation dialog, buttons, or layout.
- Do not delete, rename, or overwrite existing local profiles when switching to Central.
- Do not modify the cached Central `auth.json` or `config-auth.toml` during activation reconciliation.
- Do not identify Central ownership from provider name alone.
- Preserve unrelated worktree changes and extend the current Central name-ownership implementation in place.

---

### Task 1: Reproduce Central Switch Selection With Existing Local Profiles

**Files:**
- Modify: `internal/authprofile/ensure_test.go`
- Modify: `app_test.go`

**Interfaces:**
- Consumes: `(*Store).Activate(codexDir, profileID string, allowProviderConflict ...bool)`, `(*Store).ReconcileCurrent(codexDir string)`, `(*App).ActivateAuthProfileWithConflict(id string, allowProviderConflict bool)`, and `(*App).ListAuthProfiles()`.
- Produces: Regression coverage proving the active Central ID survives normalized Provider writeback and unrelated local profiles are preserved.

- [ ] **Step 1: Add a failing store-level regression test**

Extend `TestReconcileCurrentAfterCentralActivationDoesNotCreateLocalDuplicate` so the store contains a manual profile with the same provider but a distinct name before Central activation. After `Activate` and `ReconcileCurrent`, assert exactly two profiles remain, the manual profile is unchanged, no profile is named `yangpu (Local)`, and `status.CurrentProfileID` equals the Central ID.

```go
manual, err := store.CreateAPIKey(APIKeyRequest{
	Name: "Personal corp",
	Provider: "yangpu",
	BaseURL: "https://local.example/v1",
	APIKey: "sk-local",
})
if err != nil {
	t.Fatal(err)
}

profiles, status, err := store.ReconcileCurrent(codexDir)
if err != nil || len(profiles) != 2 || status.CurrentProfileID != central.ID {
	t.Fatalf("profiles=%#v status=%#v err=%v", profiles, status, err)
}
for _, profile := range profiles {
	if profile.Name == "yangpu (Local)" {
		t.Fatalf("Central activation created a derived local profile: %#v", profiles)
	}
	if profile.ID == manual.ID && (profile.Name != manual.Name || profile.ContentHash != manual.ContentHash) {
		t.Fatalf("manual profile changed: before=%#v after=%#v", manual, profile)
	}
}
```

- [ ] **Step 2: Add an app-level conflict-confirmation regression test**

Use `setupCurrentAuthApp` with a local `yangpu` Provider configuration that differs from the Central fixture. Cache the Central profile, create a distinctly named manual profile for the same provider, call `ActivateAuthProfileWithConflict(central.ID, true)`, then call `ListAuthProfiles`. Assert the Central profile is active, the manual profile still exists unchanged, and no additional profile was created.

```go
result, err := app.ActivateAuthProfileWithConflict(central.ID, true)
if err != nil || !result.OK {
	t.Fatalf("result=%#v err=%v", result, err)
}
profiles, err := app.ListAuthProfiles()
if err != nil || len(profiles) != 2 {
	t.Fatalf("profiles=%#v err=%v", profiles, err)
}
```

- [ ] **Step 3: Run the focused tests and verify the regression is exposed**

Run:

```bash
go test ./internal/authprofile -run 'TestReconcileCurrentAfterCentralActivationDoesNotCreateLocalDuplicate' -count=1
go test . -run 'TestListAuthProfilesAfterCentralActivationDoesNotCreateLocalDuplicate' -count=1
```

Expected before the fix: at least one test fails because reconciliation creates or selects a derived local profile instead of selecting the activated Central profile. If both tests already pass because current uncommitted code partially implements the fix, temporarily disable the `activeCentralStatus` call, verify the new test fails for the expected reason, restore the call, and continue without changing production code during the red phase.

### Task 2: Match Only the Active Central Profile After Activation Normalization

**Files:**
- Modify: `internal/authprofile/ensure.go:147-159,241-275`
- Test: `internal/authprofile/ensure_test.go`

**Interfaces:**
- Consumes: `activeProfileID(root string) (string, error)`, `codex.EnsureProviderBearerTokenInFragment(config, auth []byte) ([]byte, error)`, `codex.ExtractAuthConfigFromBytes(config []byte) ([]byte, codex.AuthConfigSummary, error)`, and `ComputeContentHash(authJSON, configTOML []byte) (string, error)`.
- Produces: `(*Store).activeCentralStatus(snapshot currentSnapshot, profiles []AuthProfile) (CurrentStatus, error)`, used only after exact hashing fails.

- [ ] **Step 1: Implement the minimal active-Central fallback**

After `exactCurrentStatus` fails, call `activeCentralStatus`. In `activeCentralStatus`, return no match unless `active.json` points to a listed `source=central_login` profile. Load that profile's cached files, apply activation-equivalent bearer-token normalization, extract the canonical auth fragment, hash it with the cached auth file, and compare it to `snapshot.contentHash`.

```go
centralStatus, err := s.activeCentralStatus(snapshot, profiles)
if err != nil {
	return nil, CurrentStatus{}, err
}
if centralStatus.CurrentProfileID != "" {
	return profiles, centralStatus, nil
}
```

On a match, copy `snapshot.status`, set `CurrentProfileID` to the active Central ID, and set the matching imported flag. On mismatch, return an empty `CurrentStatus` so existing external-modification reconciliation continues.

- [ ] **Step 2: Run focused store tests**

Run:

```bash
go test ./internal/authprofile -run 'TestReconcileCurrentAfterCentralActivation|TestReconcileCurrentCreatesLocalProfileWhenDefaultNameBelongsToCentral|TestReconcileCurrentUpdatesDerivedLocalProfileWithoutDuplicates' -count=1
```

Expected: PASS. The Central activation cases select Central without a new profile; genuinely different current credentials still follow existing local reconciliation behavior.

- [ ] **Step 3: Run the app-level regression test**

Run:

```bash
go test . -run 'TestListAuthProfilesAfterCentralActivationDoesNotCreateLocalDuplicate' -count=1
```

Expected: PASS with the Central profile active and the pre-existing manual profile retained.

### Task 3: Verify the Complete Change Set

**Files:**
- Verify: `internal/authprofile/ensure.go`
- Verify: `internal/authprofile/ensure_test.go`
- Verify: `app_test.go`
- Verify: `frontend/src/App.tsx`

**Interfaces:**
- Consumes: completed backend behavior and existing frontend API calls.
- Produces: evidence that backend tests, application tests, frontend compilation, and whitespace checks pass without frontend behavior changes.

- [ ] **Step 1: Run all auth profile tests**

Run:

```bash
go test ./internal/authprofile -count=1
```

Expected: PASS.

- [ ] **Step 2: Run the complete Go test suite**

Run:

```bash
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 3: Build the frontend**

Run from `frontend/`:

```bash
npm run build
```

Expected: TypeScript and Vite build complete successfully.

- [ ] **Step 4: Check the diff for whitespace errors and scope**

Run:

```bash
git diff --check
```

Expected: `git diff --check` has no output. The status includes only the pre-existing Central profile work plus the approved spec, plan, and regression changes; no generated frontend files or unrelated edits are added.
