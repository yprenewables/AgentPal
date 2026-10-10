package authprofile

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func writeCurrentAuth(t *testing.T, codexDir, auth, config string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(codexDir, "auth.json"), []byte(auth), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
}

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

func TestReadCurrentSnapshotUsesOnlyActiveProviderAPIKey(t *testing.T) {
	codexDir := t.TempDir()
	writeCurrentAuth(t, codexDir,
		`{"tokens":{"id_token":"header.eyJzdWIiOiJ1c2VyLTEyMyJ9.sig"}}`,
		"model_provider = \"corp\"\n\n[model_providers.corp]\napi_key = \"sk-active-secret\"\n\n[model_providers.other]\napi_key = \"sk-other-secret\"\n\n[mcp_servers.docs]\ntoken = \"sk-mcp-secret\"\n",
	)

	snapshot, ok, err := readCurrentSnapshot(codexDir)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if snapshot.profileType != "api_key" || string(snapshot.authJSON) != `{"OPENAI_API_KEY":"sk-active-secret"}` {
		t.Fatalf("snapshot=%#v auth=%s", snapshot, snapshot.authJSON)
	}
}

func TestReadCurrentSnapshotIgnoresKeysOutsideActiveProvider(t *testing.T) {
	codexDir := t.TempDir()
	writeCurrentAuth(t, codexDir,
		`{"tokens":{"id_token":"header.eyJzdWIiOiJ1c2VyLTEyMyJ9.sig"}}`,
		"model_provider = \"corp\"\n\n[model_providers.corp]\nbase_url = \"https://corp.example/v1\"\n\n[model_providers.other]\napi_key = \"sk-other-secret\"\n\n[mcp_servers.docs]\ntoken = \"sk-mcp-secret\"\n",
	)

	snapshot, ok, err := readCurrentSnapshot(codexDir)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if snapshot.profileType != "chatgpt" || snapshot.name != "user-123" {
		t.Fatalf("snapshot=%#v", snapshot)
	}
}

func TestReadCurrentSnapshotNamesChatGPTWithUserID(t *testing.T) {
	codexDir := t.TempDir()
	auth := `{"tokens":{"id_token":"header.eyJzdWIiOiJ1c2VyLTEyMyJ9.sig"}}`
	writeCurrentAuth(t, codexDir, auth, "model_provider = \"openai\"\n\n[model_providers.openai]\nbase_url = \"https://api.openai.com/v1\"\n")

	snapshot, ok, err := readCurrentSnapshot(codexDir)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if snapshot.name != "user-123" || snapshot.profileType != "chatgpt" || string(snapshot.authJSON) != auth {
		t.Fatalf("snapshot=%#v auth=%s", snapshot, snapshot.authJSON)
	}
}

func TestExactCurrentStatusRequiresMatchingContentHash(t *testing.T) {
	snapshot := currentSnapshot{
		profileType: "api_key",
		contentHash: "sha256:current",
		status:      CurrentStatus{Provider: "corp", APIKeyPreview: "sk-preview"},
	}
	profiles := []AuthProfile{
		{ID: "stale", Type: "api_key", Provider: "corp", APIKeyPreview: "sk-preview", ContentHash: "sha256:stale"},
		{ID: "current", Type: "api_key", Provider: "corp", APIKeyPreview: "sk-preview", ContentHash: "sha256:current"},
	}

	status := exactCurrentStatus(snapshot, profiles)
	if status.CurrentProfileID != "current" || !status.APIKeyImported {
		t.Fatalf("status=%#v", status)
	}
}

func TestExactCurrentStatusRejectsRotatedChatGPTToken(t *testing.T) {
	snapshot := currentSnapshot{
		profileType: "chatgpt",
		contentHash: "sha256:new-token",
		status:      CurrentStatus{Provider: "openai", ChatGPTUserID: "user-123"},
	}
	profiles := []AuthProfile{{ID: "old", Type: "chatgpt", ChatGPTUserID: "user-123", ContentHash: "sha256:old-token"}}

	status := exactCurrentStatus(snapshot, profiles)
	if status.CurrentProfileID != "" || status.ChatGPTImported {
		t.Fatalf("status=%#v", status)
	}
}

func TestReconcileCurrentCreatesAPIKeyUsingProviderName(t *testing.T) {
	codexDir := t.TempDir()
	writeCurrentAuth(t, codexDir, `{"OPENAI_API_KEY":"sk-current-secret"}`, "model_provider = \"corp\"\n\n[model_providers.corp]\nbase_url = \"https://corp.example/v1\"\n\n[model_providers.corp.headers]\nX-Team = \"platform\"\n")
	store := NewStore(t.TempDir())

	profiles, status, err := store.ReconcileCurrent(codexDir)
	if err != nil || len(profiles) != 1 {
		t.Fatalf("profiles=%#v status=%#v err=%v", profiles, status, err)
	}
	profile := profiles[0]
	if profile.Name != "corp" || profile.Type != "api_key" || profile.Source != "manual" || status.CurrentProfileID != profile.ID {
		t.Fatalf("profile=%#v status=%#v", profile, status)
	}
	_, config, err := store.GetRawContent(profile.ID)
	if err != nil || !strings.Contains(config, "[model_providers.corp.headers]") {
		t.Fatalf("config=%q err=%v", config, err)
	}
}

func TestReconcileCurrentCreatesChatGPTUsingUserID(t *testing.T) {
	codexDir := t.TempDir()
	writeCurrentAuth(t, codexDir, `{"tokens":{"id_token":"header.eyJzdWIiOiJ1c2VyLTEyMyJ9.sig"}}`, "model_provider = \"openai\"\n\n[model_providers.openai]\nbase_url = \"https://api.openai.com/v1\"\n")
	store := NewStore(t.TempDir())

	profiles, status, err := store.ReconcileCurrent(codexDir)
	if err != nil || len(profiles) != 1 {
		t.Fatalf("profiles=%#v status=%#v err=%v", profiles, status, err)
	}
	if profiles[0].Name != "user-123" || profiles[0].Source != "local_import" || status.CurrentProfileID != profiles[0].ID {
		t.Fatalf("profiles=%#v status=%#v", profiles, status)
	}
}

func TestReconcileCurrentDoesNotRewriteExactProfile(t *testing.T) {
	codexDir := t.TempDir()
	writeCurrentAuth(t, codexDir, `{"OPENAI_API_KEY":"sk-current-secret"}`, "model_provider = \"corp\"\n\n[model_providers.corp]\nbase_url = \"https://corp.example/v1\"\n")
	store := NewStore(t.TempDir())
	profiles, _, err := store.ReconcileCurrent(codexDir)
	if err != nil {
		t.Fatal(err)
	}
	metadataPath := filepath.Join(store.Root, profiles[0].ID, "profile.json")
	before, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}

	profiles, status, err := store.ReconcileCurrent(codexDir)
	after, readErr := os.ReadFile(metadataPath)
	if err != nil || readErr != nil || len(profiles) != 1 || status.CurrentProfileID != profiles[0].ID || string(before) != string(after) {
		t.Fatalf("profiles=%#v status=%#v err=%v readErr=%v changed=%v", profiles, status, err, readErr, string(before) != string(after))
	}
	active, err := os.ReadFile(filepath.Join(store.Root, "active.json"))
	if err != nil || string(active) != profiles[0].ID {
		t.Fatalf("active=%q err=%v", active, err)
	}
}

func TestReconcileCurrentDoesNothingWithoutCredentials(t *testing.T) {
	codexDir := t.TempDir()
	writeCurrentAuth(t, codexDir, `{}`, "model_provider = \"openai\"\n")
	store := NewStore(t.TempDir())

	profiles, status, err := store.ReconcileCurrent(codexDir)
	if err != nil || len(profiles) != 0 || status.CurrentProfileID != "" {
		t.Fatalf("profiles=%#v status=%#v err=%v", profiles, status, err)
	}
}

func TestReconcileCurrentReplacesRotatedSameNameProfile(t *testing.T) {
	codexDir := t.TempDir()
	store := NewStore(t.TempDir())
	old, err := store.CreateAPIKey(APIKeyRequest{Name: "corp", Provider: "corp", BaseURL: "https://old.example/v1", APIKey: "sk-old-secret"})
	if err != nil {
		t.Fatal(err)
	}
	writeCurrentAuth(t, codexDir, `{"OPENAI_API_KEY":"sk-new-secret"}`, "model_provider = \"corp\"\n\n[model_providers.corp]\nbase_url = \"https://new.example/v1\"\n")

	profiles, status, err := store.ReconcileCurrent(codexDir)
	if err != nil || len(profiles) != 1 {
		t.Fatalf("profiles=%#v status=%#v err=%v", profiles, status, err)
	}
	if profiles[0].ID != old.ID || profiles[0].CreatedAt != old.CreatedAt || profiles[0].Source != old.Source || profiles[0].BaseURL != "https://new.example/v1" || status.CurrentProfileID != old.ID {
		t.Fatalf("old=%#v profile=%#v status=%#v", old, profiles[0], status)
	}
	auth, _, err := store.GetRawContent(old.ID)
	if err != nil || !strings.Contains(auth, "sk-new-secret") {
		t.Fatalf("auth=%q err=%v", auth, err)
	}
}

func TestReconcileCurrentRejectsSameNameCentralProfile(t *testing.T) {
	codexDir := t.TempDir()
	writeCurrentAuth(t, codexDir, `{"OPENAI_API_KEY":"sk-current-secret"}`, "model_provider = \"corp\"\n\n[model_providers.corp]\nbase_url = \"https://current.example/v1\"\n")
	store := NewStore(t.TempDir())
	central, err := store.AddCentral(CentralProfile{Name: "corp", Provider: "corp", Type: "api_key", AuthJSON: []byte(`{"OPENAI_API_KEY":"sk-central-secret"}`), ConfigTOML: []byte("model_provider = \"corp\"\n\n[model_providers.corp]\nbase_url = \"https://central.example/v1\"\n")})
	if err != nil {
		t.Fatal(err)
	}
	beforeAuth, beforeConfig, _ := store.GetRawContent(central.ID)

	_, _, err = store.ReconcileCurrent(codexDir)
	if err == nil || err.Error() != `cannot automatically replace Central profile "corp"` {
		t.Fatalf("err=%v", err)
	}
	afterAuth, afterConfig, _ := store.GetRawContent(central.ID)
	if beforeAuth != afterAuth || beforeConfig != afterConfig {
		t.Fatal("Central profile changed")
	}
}

func TestReconcileCurrentRejectsAmbiguousLocalName(t *testing.T) {
	codexDir := t.TempDir()
	writeCurrentAuth(t, codexDir, `{"OPENAI_API_KEY":"sk-current-secret"}`, "model_provider = \"corp\"\n\n[model_providers.corp]\nbase_url = \"https://current.example/v1\"\n")
	store := NewStore(t.TempDir())
	_, _ = store.CreateAPIKey(APIKeyRequest{Name: "corp", Provider: "corp", APIKey: "sk-first-secret"})
	_, _ = store.CreateAPIKey(APIKeyRequest{Name: "corp", Provider: "corp", APIKey: "sk-second-secret"})

	_, _, err := store.ReconcileCurrent(codexDir)
	if err == nil || err.Error() != `multiple local profiles use generated name "corp"` {
		t.Fatalf("err=%v", err)
	}
}

func TestReconcileCurrentConcurrentCallsCreateOneProfile(t *testing.T) {
	codexDir := t.TempDir()
	writeCurrentAuth(t, codexDir, `{"OPENAI_API_KEY":"sk-current-secret"}`, "model_provider = \"corp\"\n\n[model_providers.corp]\nbase_url = \"https://corp.example/v1\"\n")
	store := NewStore(t.TempDir())
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := store.ReconcileCurrent(codexDir)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	profiles, err := store.List()
	if err != nil || len(profiles) != 1 {
		t.Fatalf("profiles=%#v err=%v", profiles, err)
	}
}

func TestReconcileCurrentReplacesRotatedChatGPTToken(t *testing.T) {
	codexDir := t.TempDir()
	store := NewStore(t.TempDir())
	writeCurrentAuth(t, codexDir, `{"tokens":{"id_token":"header.eyJzdWIiOiJ1c2VyLTEyMyIsInZlcnNpb24iOiJvbGQifQ.sig"}}`, "model_provider = \"openai\"\n\n[model_providers.openai]\nbase_url = \"https://api.openai.com/v1\"\n")
	oldProfiles, _, err := store.ReconcileCurrent(codexDir)
	if err != nil {
		t.Fatal(err)
	}
	newAuth := `{"tokens":{"id_token":"header.eyJzdWIiOiJ1c2VyLTEyMyIsInZlcnNpb24iOiJuZXcifQ.sig"}}`
	writeCurrentAuth(t, codexDir, newAuth, "model_provider = \"openai\"\n\n[model_providers.openai]\nbase_url = \"https://api.openai.com/v1\"\n")

	profiles, status, err := store.ReconcileCurrent(codexDir)
	if err != nil || len(profiles) != 1 || profiles[0].ID != oldProfiles[0].ID || status.CurrentProfileID != oldProfiles[0].ID {
		t.Fatalf("profiles=%#v status=%#v err=%v", profiles, status, err)
	}
	auth, _, err := store.GetRawContent(profiles[0].ID)
	if err != nil || auth != newAuth {
		t.Fatalf("auth=%q err=%v", auth, err)
	}
}

func TestReconcileCurrentRejectsMissingProviderTable(t *testing.T) {
	for _, auth := range []string{
		`{"OPENAI_API_KEY":"sk-current-secret"}`,
		`{"tokens":{"id_token":"header.eyJzdWIiOiJ1c2VyLTEyMyJ9.sig"}}`,
	} {
		t.Run(auth, func(t *testing.T) {
			codexDir := t.TempDir()
			writeCurrentAuth(t, codexDir, auth, "model_provider = \"openai\"\n")
			store := NewStore(t.TempDir())
			_, _, err := store.ReconcileCurrent(codexDir)
			if err == nil || !strings.Contains(err.Error(), "model provider table was not found") {
				t.Fatalf("err=%v", err)
			}
			profiles, listErr := store.List()
			if listErr != nil || len(profiles) != 0 {
				t.Fatalf("profiles=%#v err=%v", profiles, listErr)
			}
		})
	}
}

func TestStoreListIgnoresReconcileArtifacts(t *testing.T) {
	store := NewStore(t.TempDir())
	profile := AuthProfile{ID: "artifact", Name: "artifact", Type: "api_key", Provider: "corp", Source: "manual"}
	for _, name := range []string{".agentpal-auth-import-one", ".agentpal-auth-backup-one"} {
		dir := filepath.Join(store.Root, name)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := writeProfile(dir, profile, []byte(`{"OPENAI_API_KEY":"sk-artifact"}`), []byte("model_provider = \"corp\"\n\n[model_providers.corp]\nbase_url = \"https://example.test\"\n")); err != nil {
			t.Fatal(err)
		}
	}
	profiles, err := store.List()
	if err != nil || len(profiles) != 0 {
		t.Fatalf("profiles=%#v err=%v", profiles, err)
	}
}

func TestReconcileCurrentRetriesWhenCodexChanges(t *testing.T) {
	codexDir := t.TempDir()
	writeCurrentAuth(t, codexDir, `{"OPENAI_API_KEY":"sk-first-secret"}`, "model_provider = \"first\"\n\n[model_providers.first]\nbase_url = \"https://first.example/v1\"\n")
	store := NewStore(t.TempDir())
	calls := 0
	profiles, status, err := store.reconcileCurrent(codexDir, func() {
		calls++
		if calls == 1 {
			writeCurrentAuth(t, codexDir, `{"OPENAI_API_KEY":"sk-second-secret"}`, "model_provider = \"second\"\n\n[model_providers.second]\nbase_url = \"https://second.example/v1\"\n")
		}
	})
	if err != nil || calls != 2 || len(profiles) != 1 {
		t.Fatalf("profiles=%#v status=%#v calls=%d err=%v", profiles, status, calls, err)
	}
	var selected AuthProfile
	for _, profile := range profiles {
		if profile.ID == status.CurrentProfileID {
			selected = profile
		}
	}
	if selected.Name != "second" {
		t.Fatalf("selected=%#v profiles=%#v status=%#v", selected, profiles, status)
	}
}

func TestReconcileCurrentRejectsRepeatedCodexChanges(t *testing.T) {
	codexDir := t.TempDir()
	writeCurrentAuth(t, codexDir, `{"OPENAI_API_KEY":"sk-first-secret"}`, "model_provider = \"first\"\n\n[model_providers.first]\nbase_url = \"https://first.example/v1\"\n")
	store := NewStore(t.TempDir())
	calls := 0
	_, _, err := store.reconcileCurrent(codexDir, func() {
		calls++
		provider := "second"
		if calls == 2 {
			provider = "third"
		}
		writeCurrentAuth(t, codexDir, `{"OPENAI_API_KEY":"sk-`+provider+`-secret"}`, "model_provider = \""+provider+"\"\n\n[model_providers."+provider+"]\nbase_url = \"https://"+provider+".example/v1\"\n")
	})
	if err == nil || err.Error() != "current Codex authentication changed during reconciliation" || calls != 2 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	profiles, listErr := store.List()
	if listErr != nil || len(profiles) != 0 {
		t.Fatalf("profiles=%#v err=%v", profiles, listErr)
	}
}

func TestReconcileCurrentRollsBackWhenVerificationCannotReadCodex(t *testing.T) {
	codexDir := t.TempDir()
	writeCurrentAuth(t, codexDir, `{"OPENAI_API_KEY":"sk-current-secret"}`, "model_provider = \"current\"\n\n[model_providers.current]\nbase_url = \"https://current.example/v1\"\n")
	store := NewStore(t.TempDir())

	_, _, err := store.reconcileCurrent(codexDir, func() {
		if writeErr := os.WriteFile(filepath.Join(codexDir, "auth.json"), []byte(`{"OPENAI_API_KEY":`), 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}
	})
	if err == nil {
		t.Fatal("expected invalid current auth error")
	}
	profiles, listErr := store.List()
	if listErr != nil || len(profiles) != 0 {
		t.Fatalf("profiles=%#v err=%v", profiles, listErr)
	}
}

func TestReconcileCurrentReplacesStaleActiveMarker(t *testing.T) {
	codexDir := t.TempDir()
	store := NewStore(t.TempDir())
	stale, err := store.CreateAPIKey(APIKeyRequest{Name: "stale", Provider: "stale", APIKey: "sk-stale-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Root, "active.json"), []byte(stale.ID), 0o600); err != nil {
		t.Fatal(err)
	}
	writeCurrentAuth(t, codexDir, `{"OPENAI_API_KEY":"sk-current-secret"}`, "model_provider = \"current\"\n\n[model_providers.current]\nbase_url = \"https://current.example/v1\"\n")

	profiles, status, err := store.ReconcileCurrent(codexDir)
	if err != nil || status.CurrentProfileID == "" || status.CurrentProfileID == stale.ID {
		t.Fatalf("profiles=%#v status=%#v err=%v", profiles, status, err)
	}
	active, err := os.ReadFile(filepath.Join(store.Root, "active.json"))
	if err != nil || string(active) != status.CurrentProfileID {
		t.Fatalf("active=%q status=%#v err=%v", active, status, err)
	}
}
