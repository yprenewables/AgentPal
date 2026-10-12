# AgentPal Local Profile Central Name Collision Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Automatically import the current local Codex authentication as `<provider> (Local)` when its default profile name is occupied by a different Central profile.

**Architecture:** Keep exact content-hash matching as the first reconciliation step. When no exact match exists, resolve a writable local profile name: use the generated default name normally, but derive `<default> (Local)` if a Central profile owns the default name. Run the existing local create-or-replace transaction against the resolved name while preserving all Central files.

**Tech Stack:** Go 1.22+, Wails v2.12.0, React, TypeScript, Vite, `go-toml/v2`

## Global Constraints

- Never overwrite a `source=central_login` profile during local automatic import.
- Preserve exact content-hash matching before applying any naming rule.
- Use the exact suffix ` (Local)` when the generated default name is occupied by Central.
- Preserve a same-name local profile's ID, `CreatedAt`, and `Source` during credential rotation.
- Do not change Central synchronization, frontend behavior, manual profile operations, or Codex authentication files.
- Do not commit unless the user explicitly requests a commit.

---

## File Map

- Modify `internal/authprofile/ensure.go`: resolve the writable local name before existing same-name reconciliation.
- Modify `internal/authprofile/ensure_test.go`: cover Central collision creation, rotation, repeat loading, derived-name Central collision, and unchanged Central content.
- Verify `app.go` unchanged: `ListAuthProfiles()` already consumes `ReconcileCurrent` and marks the returned exact profile active.
- Verify `docs/superpowers/specs/2026-10-09-agentpal-auto-import-current-auth-profile-design.md`: confirmed behavior is already documented.

### Task 1: Resolve Central Name Collisions to a Local Name

**Files:**
- Modify: `internal/authprofile/ensure.go:131-160`
- Test: `internal/authprofile/ensure_test.go:211-235`

**Interfaces:**
- Consumes: `currentSnapshot.name`, `AuthProfile.Name`, `AuthProfile.Source`, and the existing `writeCurrentSnapshot(currentSnapshot, *AuthProfile)` transaction.
- Produces: reconciliation behavior where a default-name Central collision changes `snapshot.name` to `snapshot.name + " (Local)"` before local matching and writing.

- [ ] **Step 1: Replace the current Central no-op regression with a failing auto-import test**

Update the existing Central collision test to require two profiles, unchanged Central files, and a selected local profile:

```go
func TestReconcileCurrentCreatesLocalProfileWhenDefaultNameBelongsToCentral(t *testing.T) {
	codexDir := t.TempDir()
	writeCurrentAuth(t, codexDir, `{"OPENAI_API_KEY":"sk-current-secret"}`, "model_provider = \"corp\"\n\n[model_providers.corp]\nbase_url = \"https://current.example/v1\"\n")
	store := NewStore(t.TempDir())
	central, err := store.AddCentral(CentralProfile{
		Name: "corp", Provider: "corp", Type: "api_key",
		AuthJSON: []byte(`{"OPENAI_API_KEY":"sk-central-secret"}`),
		ConfigTOML: []byte("model_provider = \"corp\"\n\n[model_providers.corp]\nbase_url = \"https://central.example/v1\"\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	beforeAuth, beforeConfig, _ := store.GetRawContent(central.ID)

	profiles, status, err := store.ReconcileCurrent(codexDir)
	if err != nil || len(profiles) != 2 {
		t.Fatalf("profiles=%#v status=%#v err=%v", profiles, status, err)
	}
	var local AuthProfile
	for _, profile := range profiles {
		if profile.Name == "corp (Local)" {
			local = profile
		}
	}
	if local.ID == "" || local.Source == "central_login" || status.CurrentProfileID != local.ID {
		t.Fatalf("profiles=%#v status=%#v", profiles, status)
	}
	afterAuth, afterConfig, _ := store.GetRawContent(central.ID)
	if beforeAuth != afterAuth || beforeConfig != afterConfig {
		t.Fatal("Central profile changed")
	}
}
```

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```bash
go test ./internal/authprofile -run TestReconcileCurrentCreatesLocalProfileWhenDefaultNameBelongsToCentral -count=1
```

Expected: FAIL because reconciliation currently returns only the Central profile without importing `corp (Local)`.

- [ ] **Step 3: Implement minimal resolved-name reconciliation**

In `reconcileCurrent`, keep exact hash matching first. Replace the current immediate Central return with two passes:

```go
		resolvedName := snapshot.name
		for _, profile := range profiles {
			if profile.Name == snapshot.name && profile.Source == "central_login" {
				resolvedName = snapshot.name + " (Local)"
				break
			}
		}
		snapshot.name = resolvedName

		var localMatches []AuthProfile
		for _, profile := range profiles {
			if profile.Name != snapshot.name {
				continue
			}
			if profile.Source == "central_login" {
				return nil, CurrentStatus{}, fmt.Errorf("cannot automatically replace Central profile %q", snapshot.name)
			}
			localMatches = append(localMatches, profile)
		}
```

Leave the existing ambiguous-local check, staged write, reload, exact verification, rollback, and active-profile write unchanged.

- [ ] **Step 4: Run the focused test and verify GREEN**

Run:

```bash
go test ./internal/authprofile -run TestReconcileCurrentCreatesLocalProfileWhenDefaultNameBelongsToCentral -count=1
```

Expected: PASS.

- [ ] **Step 5: Add rotation and idempotency tests**

Add one test that reconciles twice, rotates the local key, and reconciles again:

```go
func TestReconcileCurrentUpdatesDerivedLocalProfileWithoutDuplicates(t *testing.T) {
	codexDir := t.TempDir()
	store := NewStore(t.TempDir())
	_, err := store.AddCentral(CentralProfile{
		Name: "corp", Provider: "corp", Type: "api_key",
		AuthJSON: []byte(`{"OPENAI_API_KEY":"sk-central-secret"}`),
		ConfigTOML: []byte("model_provider = \"corp\"\n\n[model_providers.corp]\nbase_url = \"https://central.example/v1\"\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	writeCurrentAuth(t, codexDir, `{"OPENAI_API_KEY":"sk-local-one"}`, "model_provider = \"corp\"\n\n[model_providers.corp]\nbase_url = \"https://local.example/v1\"\n")
	first, firstStatus, err := store.ReconcileCurrent(codexDir)
	if err != nil || len(first) != 2 {
		t.Fatalf("profiles=%#v status=%#v err=%v", first, firstStatus, err)
	}
	localID := firstStatus.CurrentProfileID
	second, secondStatus, err := store.ReconcileCurrent(codexDir)
	if err != nil || len(second) != 2 || secondStatus.CurrentProfileID != localID {
		t.Fatalf("profiles=%#v status=%#v err=%v", second, secondStatus, err)
	}

	writeCurrentAuth(t, codexDir, `{"OPENAI_API_KEY":"sk-local-two"}`, "model_provider = \"corp\"\n\n[model_providers.corp]\nbase_url = \"https://rotated.example/v1\"\n")
	rotated, rotatedStatus, err := store.ReconcileCurrent(codexDir)
	if err != nil || len(rotated) != 2 || rotatedStatus.CurrentProfileID != localID {
		t.Fatalf("profiles=%#v status=%#v err=%v", rotated, rotatedStatus, err)
	}
	for _, profile := range rotated {
		if profile.ID == localID && (profile.Name != "corp (Local)" || profile.BaseURL != "https://rotated.example/v1") {
			t.Fatalf("local profile was not updated in place: %#v", profile)
		}
	}
}
```

- [ ] **Step 6: Add a derived-name Central collision test**

Create Central profiles named `corp` and `corp (Local)` by using `AddCentral` for `corp`, then create a valid saved Central fixture for the derived display name through `writeProfile` because reserved Central names intentionally reject spaces. Assert `ReconcileCurrent` returns `cannot automatically replace Central profile "corp (Local)"` and neither Central profile changes.

- [ ] **Step 7: Run all authprofile tests**

Run:

```bash
gofmt -w internal/authprofile/ensure.go internal/authprofile/ensure_test.go
go test ./internal/authprofile -count=1
```

Expected: PASS with no duplicate `corp (Local)` profile and no Central file changes.

### Task 2: Verify App Integration and Installed Application

**Files:**
- Test: `app_test.go`
- Verify: `app.go:151-170`
- Build output: `build/bin/AgentPal.app`
- Installed output: `/Applications/AgentPal.app`

**Interfaces:**
- Consumes: `(*authprofile.Store).ReconcileCurrent(codexDir) ([]AuthProfile, CurrentStatus, error)`.
- Produces: an Authentication list containing Central `memory` plus active local `memory (Local)` without frontend changes.

- [ ] **Step 1: Add a failing App-level list test before relying on Store coverage alone**

Add:

```go
func TestListAuthProfilesImportsLocalProfileBesideSameNameCentral(t *testing.T) {
	app, _, profilesRoot := setupCurrentAuthApp(t, `{"OPENAI_API_KEY":"sk-current-secret"}`, "model_provider = \"corp\"\n\n[model_providers.corp]\nbase_url = \"https://current.example/v1\"\n")
	_, err := authprofile.NewStore(profilesRoot).AddCentral(authprofile.CentralProfile{
		Name: "corp", Provider: "corp", Type: "api_key",
		AuthJSON: []byte(`{"OPENAI_API_KEY":"sk-central-secret"}`),
		ConfigTOML: []byte("model_provider = \"corp\"\n\n[model_providers.corp]\nbase_url = \"https://central.example/v1\"\n"),
	})
	if err != nil {
		t.Fatal(err)
	}

	profiles, err := app.ListAuthProfiles()
	if err != nil || len(profiles) != 2 {
		t.Fatalf("profiles=%#v err=%v", profiles, err)
	}
	for _, profile := range profiles {
		if profile.Name == "corp (Local)" && profile.Active {
			return
		}
	}
	t.Fatalf("active local profile not found: %#v", profiles)
}
```

- [ ] **Step 2: Run the App integration test**

Run:

```bash
go test . -run TestListAuthProfilesImportsLocalProfileBesideSameNameCentral -count=1
```

Expected after Task 1: PASS. No frontend reconciliation is required because `profileTypesFromStore` already maps `CurrentProfileID` to `Active`.

- [ ] **Step 3: Run full verification**

Run:

```bash
go test ./... -count=1
```

Expected: all Go packages PASS.

Run from `frontend/`:

```bash
npm run build
```

Expected: `tsc && vite build` succeeds.

Run from the repository root:

```bash
git diff --check
```

Expected: no whitespace errors.

- [ ] **Step 4: Build and install the Wails application**

Run from the repository root:

```bash
/Users/yp-mac-02/go/bin/wails build -clean
pkill -f '^/Applications/AgentPal.app/Contents/MacOS/AgentPal$' || true
ditto build/bin/AgentPal.app /Applications/AgentPal.app
open -n /Applications/AgentPal.app
```

Expected: Wails build completes, the installed application is replaced, and a new installed AgentPal process starts.

- [ ] **Step 5: Verify the real local profile state without exposing credentials**

Check the process and profile metadata only:

```bash
pgrep -fl '^/Applications/AgentPal.app/Contents/MacOS/AgentPal$'
```

Expected: one running installed AgentPal process.

Inspect `~/.agentpal/auth-profiles/*/profile.json` and assert:

- one Central profile named `memory` remains `source=central_login`;
- one non-Central profile named `memory (Local)` exists;
- `active.json` contains the local profile ID;
- no secret-bearing `auth.json` content is printed.

- [ ] **Step 6: Inspect final scope**

Run:

```bash
git status --short
git diff -- internal/authprofile/ensure.go internal/authprofile/ensure_test.go app_test.go docs/superpowers/specs/2026-10-09-agentpal-auto-import-current-auth-profile-design.md docs/superpowers/plans/2026-10-12-agentpal-local-profile-central-name-collision.md
```

Expected: only the approved naming behavior, tests, specification, and plan are changed; no generated binary or credential file is tracked.
