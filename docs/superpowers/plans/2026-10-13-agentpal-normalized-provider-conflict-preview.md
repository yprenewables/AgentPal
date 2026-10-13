# AgentPal Normalized Provider Conflict Preview Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Compare the Provider configuration that profile activation will actually write so an unchanged Central Memory profile does not repeatedly show a replacement warning.

**Architecture:** Add one store helper that loads profile metadata, auth, and config, validates auth JSON, and applies `EnsureProviderBearerTokenInFragment`. Use its normalized fragment for both preview and activation conflict checks and for the eventual merge. Normalize remote profile fragments before their conflict checks as well.

**Tech Stack:** Go, Wails backend, Go `testing`, React/TypeScript build verification, Wails macOS arm64 packaging

## Global Constraints

- Do not modify cached profile files during normalization.
- Do not ignore Provider fields during comparison.
- Keep the existing confirmation copy and frontend interaction unchanged.
- Preserve backup, rollback, and `active.json` behavior.
- A missing or different `experimental_bearer_token` remains a real conflict.
- Non-Memory profiles retain their current comparison behavior.

---

### Task 1: Reproduce the Repeated Memory Provider Warning

**Files:**
- Modify: `internal/authprofile/switch_test.go`

**Interfaces:**
- Consumes: `(*Store).Preview`, `(*Store).Activate`, and `(*Store).CreateRaw`.
- Produces: Regression tests for normalized Memory comparison and the memory-newapi-memory switch sequence.

- [ ] **Step 1: Add a failing normalized preview test**

Create a Memory profile whose cached fragment omits `experimental_bearer_token`, write a local `config.toml` containing the same Provider plus the token derived from profile `auth.json`, and assert `Preview` reports no conflict.

```go
preview, err := store.Preview(codexDir, memory.ID)
if err != nil {
	t.Fatal(err)
}
if preview.ProviderConflict {
	t.Fatalf("normalized Provider should match local config: %#v", preview)
}
```

- [ ] **Step 2: Add a failing round-trip test**

Create conflicting `memory` and `newapi` profiles. Assert the first Memory preview conflicts, activate it with confirmation, activate newapi, then assert the second Memory preview does not conflict.

```go
first, err := store.Preview(codexDir, memory.ID)
if err != nil || !first.ProviderConflict {
	t.Fatalf("first preview=%#v err=%v", first, err)
}
if _, err := store.Activate(codexDir, memory.ID, true); err != nil {
	t.Fatal(err)
}
if _, err := store.Activate(codexDir, newAPI.ID, true); err != nil {
	t.Fatal(err)
}
second, err := store.Preview(codexDir, memory.ID)
if err != nil || second.ProviderConflict {
	t.Fatalf("second preview=%#v err=%v", second, err)
}
```

- [ ] **Step 3: Verify RED**

Run:

```bash
go test ./internal/authprofile -run 'TestPreviewNormalizesMemoryProvider|TestPreviewAfterMemoryNewAPIMemoryRoundTrip' -count=1
```

Expected: FAIL because `Preview` compares the unnormalized cached fragment.

### Task 2: Normalize Local and Remote Conflict Checks

**Files:**
- Modify: `internal/authprofile/switch.go`
- Test: `internal/authprofile/switch_test.go`

**Interfaces:**
- Produces: `(*Store).loadProfileForActivation(profileID string) (AuthProfile, []byte, []byte, error)` returning metadata, auth JSON, and normalized config fragment.
- Consumes: `codex.EnsureProviderBearerTokenInFragment(fragment, authData []byte) ([]byte, error)` and `codex.CompareProviderConfig`.

- [ ] **Step 1: Add the shared profile loader**

Load existing metadata and fragment with `loadProfile`, read `auth.json`, validate it with `json.Unmarshal`, normalize the fragment, and return all three values without writing profile files.

```go
func (s *Store) loadProfileForActivation(profileID string) (AuthProfile, []byte, []byte, error) {
	p, fragment, err := s.loadProfile(profileID)
	if err != nil {
		return AuthProfile{}, nil, nil, err
	}
	auth, err := os.ReadFile(filepath.Join(s.Root, profileID, "auth.json"))
	if err != nil {
		return AuthProfile{}, nil, nil, err
	}
	var parsed any
	if err := json.Unmarshal(auth, &parsed); err != nil {
		return AuthProfile{}, nil, nil, err
	}
	fragment, err = codex.EnsureProviderBearerTokenInFragment(fragment, auth)
	return p, auth, fragment, err
}
```

- [ ] **Step 2: Use normalized fragments in Preview and Activate**

Replace separate `loadProfile`, auth read, auth validation, and late normalization calls with `loadProfileForActivation`. Call `CompareProviderConfig` with the normalized fragment and pass that exact fragment to `MergeAuthConfig`.

- [ ] **Step 3: Normalize remote fragments before comparison**

At the start of `ActivateRemote`, validate auth JSON and call `EnsureProviderBearerTokenInFragment` before `CompareProviderConfig`. Remove the later duplicate normalization and JSON validation.

- [ ] **Step 4: Verify GREEN and conflict boundaries**

Run:

```bash
go test ./internal/authprofile -run 'TestPreview|TestActivate' -count=1
```

Expected: PASS, including existing conflict tests and new round-trip tests.

### Task 3: Verify, Commit, Push, Build, and Install

**Files:**
- Verify: `internal/authprofile/switch.go`
- Verify: `internal/authprofile/switch_test.go`
- Include: `docs/superpowers/specs/2026-10-13-agentpal-normalized-provider-conflict-preview-design.md`
- Include: `docs/superpowers/plans/2026-10-13-agentpal-normalized-provider-conflict-preview.md`

**Interfaces:**
- Produces: tested commit on `main`, synchronized `origin/main`, and matching installed `/Applications/AgentPal.app`.

- [ ] **Step 1: Run full verification**

```bash
go test ./internal/authprofile -count=1
go test ./... -count=1
cd frontend && npm run build
git diff --check
```

Expected: all commands pass.

- [ ] **Step 2: Commit and push only this change**

Explicitly stage the two Go files and two approved documents, inspect the cached diff, commit with `fix: normalize provider conflict previews`, push `main`, and verify local `HEAD` equals remote `refs/heads/main`.

- [ ] **Step 3: Build and install the macOS application**

```bash
/Users/yp-mac-02/go/bin/wails build -clean
pkill -f '^/Applications/AgentPal.app/Contents/MacOS/AgentPal$' || true
ditto build/bin/AgentPal.app /Applications/AgentPal.app
open -n /Applications/AgentPal.app
```

- [ ] **Step 4: Verify the installed application**

Confirm the built and installed binaries are arm64, their SHA-256 hashes match, the app has a valid ad-hoc signature, and one installed AgentPal process is running.
