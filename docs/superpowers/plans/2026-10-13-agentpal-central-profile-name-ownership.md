# AgentPal Central Profile Name Ownership Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ensure Central profiles exclusively own their names while preserving or safely renaming any colliding local profile.

**Architecture:** Add one Store-level collision reconciler used by both Central synchronization and current-auth reconciliation. It compares full content hashes, converts identical local profiles into Central profiles without changing their IDs, and transactionally renames different local profiles to `<name> (Local)` before Central import. Startup reconciliation repairs historical duplicates before matching or importing current Codex credentials.

**Tech Stack:** Go 1.22+, Wails v2.12.0, `go-toml/v2`, Go `testing`

## Global Constraints

- Central profiles permanently own their exact display names.
- Never retain a non-Central profile with the exact name of a Central profile.
- Identical content must collapse to one Central profile.
- Different local content must retain its ID, source, credentials, configuration, and `CreatedAt`, changing only its name and `UpdatedAt`.
- The derived local name is exactly `<Central name> (Local)`.
- If the derived name is occupied, return an error without overwriting any profile.
- Do not generate numbered fallback names.
- Do not change Central APIs, frontend behavior, authentication, or Codex non-auth configuration.
- Do not modify the pre-existing uncommitted changes in `docs/superpowers/specs/2026-10-09-agentpal-auto-import-current-auth-profile-design.md`.
- Do not commit unless the user explicitly requests it.

---

## File Map

- Create `internal/authprofile/name_ownership.go`: shared collision detection and transactional profile conversion/rename helpers.
- Create `internal/authprofile/name_ownership_test.go`: focused tests for identical merge, different-content rename, occupied derived name, and ambiguous collisions.
- Modify `internal/authprofile/central.go`: invoke ownership reconciliation before creating a new Central cache.
- Modify `internal/authprofile/central_test.go`: replace the old same-name rejection assertion with Central import behavior tests.
- Modify `internal/authprofile/ensure.go`: repair historical collisions before exact current-auth matching and avoid re-importing an activated Central profile.
- Modify `internal/authprofile/ensure_test.go`: cover startup repair and Central activation reload.
- Modify `app_test.go`: cover the application-level list behavior.

### Task 1: Reconcile One Central Name in the Store

**Files:**
- Create: `internal/authprofile/name_ownership.go`
- Create: `internal/authprofile/name_ownership_test.go`

**Interfaces:**
- Consumes: `(*Store).List()`, `readSavedProfile`, `writeProfile`, `(*Store).writeActiveProfileID`, `AuthProfile.ContentHash`.
- Produces: `func (s *Store) reconcileCentralName(central AuthProfile) ([]AuthProfile, error)` for repairing a Central profile already present in the Store.
- Produces: `func (s *Store) prepareCentralImport(c CentralProfile) (*AuthProfile, func() error, error)` for converting or renaming an existing local profile before first Central sync. A non-nil profile means identical local content was converted in place; the rollback restores any rename if subsequent Central creation fails.

- [ ] **Step 1: Write failing tests for an existing Central profile**

Add tests that manually create a Central profile plus a same-name local fixture and assert:

```go
func TestReconcileCentralNameMergesIdenticalLocalProfile(t *testing.T) {
	store := NewStore(t.TempDir())
	local, err := store.CreateRaw(RawProfileRequest{Name: "yangpu", AuthJSON: string(centralFixture().AuthJSON), ConfigTOML: string(centralFixture().ConfigTOML)})
	if err != nil { t.Fatal(err) }
	central, err := store.AddCentral(centralFixture())
	if err != nil { t.Fatal(err) }
	if err := store.writeActiveProfileID(local.ID); err != nil { t.Fatal(err) }

	profiles, err := store.reconcileCentralName(central)
	if err != nil || len(profiles) != 1 || profiles[0].ID != central.ID || profiles[0].Source != "central_login" {
		t.Fatalf("profiles=%#v err=%v", profiles, err)
	}
	active, err := os.ReadFile(filepath.Join(store.Root, "active.json"))
	if err != nil || string(active) != central.ID { t.Fatalf("active=%q err=%v", active, err) }
}
```

Add a different-content test asserting the local ID is retained, its name becomes `yangpu (Local)`, its auth/config bytes stay unchanged, and Central keeps `yangpu`.

- [ ] **Step 2: Run the focused tests and verify RED**

Run: `go test ./internal/authprofile -run 'TestReconcileCentralName' -count=1`

Expected: FAIL because `reconcileCentralName` does not exist.

- [ ] **Step 3: Implement historical collision reconciliation**

In `name_ownership.go`, implement exact-name partitioning and validation:

```go
func (s *Store) reconcileCentralName(central AuthProfile) ([]AuthProfile, error) {
	profiles, err := s.List()
	if err != nil { return nil, err }
	var matches []AuthProfile
	derivedName := central.Name + " (Local)"
	for _, profile := range profiles {
		if profile.ID != central.ID && profile.Name == central.Name && profile.Source != "central_login" {
			matches = append(matches, profile)
		}
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("multiple local profiles use Central name %q", central.Name)
	}
	if len(matches) == 0 { return profiles, nil }
	local := matches[0]
	if local.ContentHash == central.ContentHash {
		// Repoint active selection before deleting the duplicate local directory.
		// Then remove only local.ID and reload the list.
	}
	// Reject any other profile using derivedName, then rewrite local metadata
	// through a staged directory replacement while preserving auth/config bytes.
}
```

Use a staged directory plus backup rename, following `writeCurrentSnapshot`, so a failed replacement restores the original directory. Update only `Name` and `UpdatedAt` for different content.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run: `go test ./internal/authprofile -run 'TestReconcileCentralName' -count=1`

Expected: PASS.

- [ ] **Step 5: Add collision safety tests**

Add tests asserting:

```go
func TestReconcileCentralNameRejectsOccupiedLocalSuffixWithoutChanges(t *testing.T)
func TestReconcileCentralNameRejectsMultipleSameNameLocalsWithoutChanges(t *testing.T)
```

Snapshot every affected `profile.json`, `auth.json`, and `config-auth.toml` before calling the reconciler, then assert byte-for-byte equality after the expected error.

- [ ] **Step 6: Run all ownership tests**

Run: `go test ./internal/authprofile -run 'TestReconcileCentralName' -count=1`

Expected: PASS.

### Task 2: Apply Ownership Rules During First Central Sync

**Files:**
- Modify: `internal/authprofile/central.go:68-145`
- Modify: `internal/authprofile/central_test.go:117-214`
- Modify: `internal/authprofile/name_ownership.go`

**Interfaces:**
- Consumes: `prepareCentralImport(CentralProfile)` from Task 1.
- Produces: `SyncCentral` behavior that returns one Central profile for identical content or two uniquely named profiles for different content.

- [ ] **Step 1: Replace the old rejection test with failing import tests**

Replace `TestSyncCentralNeverOverwritesManualProfile` with:

```go
func TestSyncCentralConvertsIdenticalSameNameLocalProfile(t *testing.T) {
	store := NewStore(t.TempDir())
	local, err := store.CreateRaw(RawProfileRequest{Name: "yangpu", AuthJSON: string(centralFixture().AuthJSON), ConfigTOML: string(centralFixture().ConfigTOML)})
	if err != nil { t.Fatal(err) }
	if err := store.writeActiveProfileID(local.ID); err != nil { t.Fatal(err) }

	profile, status, err := store.SyncCentral(centralFixture())
	if err != nil || status != "added" || profile.ID != local.ID || profile.Source != "central_login" {
		t.Fatalf("profile=%#v status=%q err=%v", profile, status, err)
	}
	profiles, err := store.List()
	if err != nil || len(profiles) != 1 || !profiles[0].Active { t.Fatalf("profiles=%#v err=%v", profiles, err) }
}

func TestSyncCentralRenamesDifferentSameNameLocalProfile(t *testing.T) {
	store := NewStore(t.TempDir())
	local, err := store.CreateAPIKey(APIKeyRequest{Name: "yangpu", Provider: "yangpu", BaseURL: "https://local.example/v1", APIKey: "sk-local"})
	if err != nil { t.Fatal(err) }
	profile, status, err := store.SyncCentral(centralFixture())
	if err != nil || status != "added" || profile.Name != "yangpu" { t.Fatalf("profile=%#v status=%q err=%v", profile, status, err) }
	profiles, err := store.List()
	if err != nil || len(profiles) != 2 { t.Fatalf("profiles=%#v err=%v", profiles, err) }
	// Assert local.ID still exists as "yangpu (Local)" with the original hash.
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/authprofile -run 'TestSyncCentral(Convert|Rename)' -count=1`

Expected: FAIL because current `SyncCentral` rejects same-name local profiles.

- [ ] **Step 3: Implement Central import preparation**

Before the current `!found` creation branch, call `prepareCentralImport(c)`:

```go
converted, rollback, err := s.prepareCentralImport(c)
if err != nil { return AuthProfile{}, "error", err }
if converted != nil { return *converted, "added", nil }
profile, err := s.AddCentral(c)
if err != nil {
	if rollbackErr := rollback(); rollbackErr != nil {
		return AuthProfile{}, "error", fmt.Errorf("create Central profile: %w; rollback failed: %v", err, rollbackErr)
	}
	return AuthProfile{}, "error", err
}
return profile, "added", nil
```

`prepareCentralImport` must calculate the incoming Central hash, reject ambiguity/occupied suffix before writing, convert identical content in place, or transactionally rename different content and return a rollback closure.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run: `go test ./internal/authprofile -run 'TestSyncCentral(Convert|Rename)' -count=1`

Expected: PASS.

- [ ] **Step 5: Verify existing Central update behavior**

Run: `go test ./internal/authprofile -run 'TestSyncCentral' -count=1`

Expected: PASS, including unchanged and active Central cache update tests.

### Task 3: Repair Historical Collisions Before Current Authentication Import

**Files:**
- Modify: `internal/authprofile/ensure.go:111-216`
- Modify: `internal/authprofile/ensure_test.go:214-319`

**Interfaces:**
- Consumes: `reconcileCentralName(AuthProfile)` from Task 1.
- Produces: `ReconcileCurrent` startup behavior with unique names and correct `CurrentProfileID`.

- [ ] **Step 1: Write failing startup repair tests**

Add fixtures that bypass `SyncCentral` to reproduce legacy disk state, then add:

```go
func TestReconcileCurrentMergesLegacyIdenticalCentralCollision(t *testing.T)
func TestReconcileCurrentRenamesLegacyDifferentCentralCollision(t *testing.T)
func TestReconcileCurrentAfterCentralActivationDoesNotCreateLocalDuplicate(t *testing.T)
```

The activation test must create a Central profile, call `Activate(codexDir, central.ID, true)`, call `ReconcileCurrent(codexDir)`, and assert one profile remains with `status.CurrentProfileID == central.ID`.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/authprofile -run 'TestReconcileCurrent(MergesLegacy|RenamesLegacy|AfterCentralActivation)' -count=1`

Expected: FAIL because startup does not repair historical exact-name collisions; activation may create a local duplicate when the merged config differs from the stored fragment.

- [ ] **Step 3: Repair Central name ownership before current snapshot matching**

At the beginning of each reconciliation attempt, process Central profiles:

```go
for _, profile := range profiles {
	if profile.Source != "central_login" { continue }
	profiles, err = s.reconcileCentralName(profile)
	if err != nil { return nil, CurrentStatus{}, err }
}
```

Reload after any repair. Preserve the existing exact hash match, local import transaction, retry, and active marker behavior.

- [ ] **Step 4: Match an activated Central profile before generating `(Local)`**

When exact stored content hash does not match because activation injects the API key as `experimental_bearer_token`, read `active.json`. If it points to a Central profile, normalize that profile fragment through `codex.EnsureProviderBearerTokenInFragment` with its stored auth and compare the resulting hash to the current snapshot. Only treat it as current when the normalized full hash matches; a real credential/config change must continue into local import.

- [ ] **Step 5: Run focused tests and verify GREEN**

Run: `go test ./internal/authprofile -run 'TestReconcileCurrent(MergesLegacy|RenamesLegacy|AfterCentralActivation)' -count=1`

Expected: PASS.

- [ ] **Step 6: Update obsolete expectations**

Keep `TestReconcileCurrentCreatesLocalProfileWhenDefaultNameBelongsToCentral` for genuinely different current credentials. Update only tests whose fixture represents identical Central content so they expect Central selection rather than a local duplicate.

- [ ] **Step 7: Run the full authprofile package**

Run: `gofmt -w internal/authprofile/name_ownership.go internal/authprofile/name_ownership_test.go internal/authprofile/central.go internal/authprofile/central_test.go internal/authprofile/ensure.go internal/authprofile/ensure_test.go && go test ./internal/authprofile -count=1`

Expected: PASS.

### Task 4: Verify App Integration and Full Regression Suite

**Files:**
- Modify: `app_test.go`
- Verify: `app.go:151-170,264-307`

**Interfaces:**
- Consumes: updated `Store.ReconcileCurrent` and `Store.SyncCentral` behavior.
- Produces: application-level guarantee that Central activation followed by list reload does not create a local duplicate.

- [ ] **Step 1: Add the App-level regression test**

Add a test using `setupCurrentAuthApp`, a Central fixture in the local store, and `ActivateAuthProfileWithConflict`, then assert `ListAuthProfiles()` returns one Central profile marked active and no same-name local profile.

- [ ] **Step 2: Run the App test**

Run: `go test . -run TestListAuthProfilesAfterCentralActivationDoesNotCreateLocalDuplicate -count=1`

Expected: PASS after Task 3.

- [ ] **Step 3: Run all Go tests**

Run: `go test ./... -count=1`

Expected: all packages PASS.

- [ ] **Step 4: Build the frontend**

Run from `frontend/`: `npm run build`

Expected: `tsc && vite build` succeeds.

- [ ] **Step 5: Check formatting and scope**

Run: `git diff --check`

Expected: no whitespace errors.

Run: `git status --short`

Expected: only the new spec/plan and intended Go test/implementation files are changed, plus the pre-existing uncommitted spec modification that must remain untouched.
