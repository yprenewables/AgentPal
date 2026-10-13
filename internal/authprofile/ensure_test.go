package authprofile

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
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

func TestReconcileCurrentCreatesLocalProfileWhenDefaultNameBelongsToCentral(t *testing.T) {
	codexDir := t.TempDir()
	writeCurrentAuth(t, codexDir, `{"OPENAI_API_KEY":"sk-current-secret"}`, "model_provider = \"corp\"\n\n[model_providers.corp]\nbase_url = \"https://current.example/v1\"\n")
	store := NewStore(t.TempDir())
	central, err := store.AddCentral(CentralProfile{Name: "corp", Provider: "corp", Type: "api_key", AuthJSON: []byte(`{"OPENAI_API_KEY":"sk-central-secret"}`), ConfigTOML: []byte("model_provider = \"corp\"\n\n[model_providers.corp]\nbase_url = \"https://central.example/v1\"\n")})
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

func TestReconcileCurrentAfterCentralActivationDoesNotCreateLocalDuplicate(t *testing.T) {
	codexDir := t.TempDir()
	writeCurrentAuth(t, codexDir, `{"OPENAI_API_KEY":"sk-old"}`, "model_provider = \"old\"\n\n[model_providers.old]\nbase_url = \"https://old.example/v1\"\n")
	store := NewStore(t.TempDir())
	manual, err := store.CreateAPIKey(APIKeyRequest{Name: "Personal corp", Provider: "yangpu", BaseURL: "https://local.example/v1", APIKey: "sk-local"})
	if err != nil {
		t.Fatal(err)
	}
	fixture := centralFixture()
	fixture.AuthJSON = []byte(`{"OPENAI_API_KEY":"sk-mem-central"}`)
	central, err := store.AddCentral(fixture)
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Activate(codexDir, central.ID, true)
	if err != nil || !result.OK {
		t.Fatalf("result=%#v err=%v", result, err)
	}

	profiles, status, err := store.ReconcileCurrent(codexDir)
	if err != nil || len(profiles) != 2 || status.CurrentProfileID != central.ID {
		t.Fatalf("profiles=%#v status=%#v err=%v", profiles, status, err)
	}
	foundCentral := false
	foundManual := false
	for _, profile := range profiles {
		if profile.Name == "yangpu (Local)" {
			t.Fatalf("Central activation created a derived local profile: %#v", profiles)
		}
		if profile.ID == central.ID {
			foundCentral = profile.Source == "central_login"
		}
		if profile.ID == manual.ID && (profile.Name != manual.Name || profile.ContentHash != manual.ContentHash) {
			t.Fatalf("manual profile changed: before=%#v after=%#v", manual, profile)
		}
		if profile.ID == manual.ID {
			foundManual = true
		}
	}
	if !foundCentral || !foundManual {
		t.Fatalf("expected Central and manual profiles: %#v", profiles)
	}
}

func TestReconcileCurrentAfterCentralActivationNormalizesMissingProviderName(t *testing.T) {
	codexDir := t.TempDir()
	writeCurrentAuth(t, codexDir, `{"OPENAI_API_KEY":"sk-old"}`, "model_provider = \"old\"\n\n[model_providers.old]\nbase_url = \"https://old.example/v1\"\n")
	store := NewStore(t.TempDir())
	fixture := centralFixture()
	fixture.AuthJSON = []byte(`{"OPENAI_API_KEY":"sk-mem-central"}`)
	fixture.ConfigTOML = []byte("model_provider = \"yangpu\"\n\n[model_providers.yangpu]\nbase_url = \"https://central.example/v1\"\n")
	central, err := store.AddCentral(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Activate(codexDir, central.ID, true); err != nil {
		t.Fatal(err)
	}

	profiles, status, err := store.ReconcileCurrent(codexDir)
	if err != nil || len(profiles) != 1 || status.CurrentProfileID != central.ID {
		t.Fatalf("profiles=%#v status=%#v err=%v", profiles, status, err)
	}
}

func TestReconcileCurrentDoesNotKeepCentralSelectedAfterExternalCredentialChange(t *testing.T) {
	codexDir := t.TempDir()
	writeCurrentAuth(t, codexDir, `{"OPENAI_API_KEY":"sk-old"}`, "model_provider = \"old\"\n\n[model_providers.old]\nbase_url = \"https://old.example/v1\"\n")
	store := NewStore(t.TempDir())
	fixture := centralFixture()
	fixture.AuthJSON = []byte(`{"OPENAI_API_KEY":"sk-mem-central"}`)
	central, err := store.AddCentral(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Activate(codexDir, central.ID, true); err != nil {
		t.Fatal(err)
	}
	writeCurrentAuth(t, codexDir, `{"OPENAI_API_KEY":"sk-external"}`, "model_provider = \"yangpu\"\n\n[model_providers.yangpu]\nbase_url = \"https://external.example/v1\"\n")

	profiles, status, err := store.ReconcileCurrent(codexDir)
	if err != nil || len(profiles) != 2 || status.CurrentProfileID == central.ID {
		t.Fatalf("profiles=%#v status=%#v err=%v", profiles, status, err)
	}
	for _, profile := range profiles {
		if profile.ID == status.CurrentProfileID && (profile.Source == "central_login" || profile.Name != "yangpu (Local)") {
			t.Fatalf("external credentials were attributed to Central: %#v", profile)
		}
	}
}

func TestReconcileCurrentRepairsLegacyCollisionWithoutCurrentCredentials(t *testing.T) {
	codexDir := t.TempDir()
	writeCurrentAuth(t, codexDir, `{}`, "model_provider = \"yangpu\"\n\n[model_providers.yangpu]\nbase_url = \"https://local.example/v1\"\n")
	store := NewStore(t.TempDir())
	local, err := store.CreateAPIKey(APIKeyRequest{Name: "yangpu", Provider: "yangpu", BaseURL: "https://local.example/v1", APIKey: "sk-local"})
	if err != nil {
		t.Fatal(err)
	}
	central, err := store.AddCentral(centralFixture())
	if err != nil {
		t.Fatal(err)
	}

	profiles, status, err := store.ReconcileCurrent(codexDir)
	if err != nil || len(profiles) != 2 || status.CurrentProfileID != "" {
		t.Fatalf("profiles=%#v status=%#v err=%v", profiles, status, err)
	}
	for _, profile := range profiles {
		switch profile.ID {
		case local.ID:
			if profile.Name != "yangpu (Local)" {
				t.Fatalf("local profile was not renamed: %#v", profile)
			}
		case central.ID:
			if profile.Name != "yangpu" {
				t.Fatalf("Central profile lost its name: %#v", profile)
			}
		}
	}
}

func TestReconcileCurrentMergesLegacyIdenticalCentralCollision(t *testing.T) {
	codexDir := t.TempDir()
	fixture := centralFixture()
	writeCurrentAuth(t, codexDir, string(fixture.AuthJSON), string(fixture.ConfigTOML))
	store := NewStore(t.TempDir())
	local, err := store.CreateRaw(RawProfileRequest{Name: fixture.Name, AuthJSON: string(fixture.AuthJSON), ConfigTOML: string(fixture.ConfigTOML)})
	if err != nil {
		t.Fatal(err)
	}
	central, err := store.AddCentral(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.writeActiveProfileID(local.ID); err != nil {
		t.Fatal(err)
	}

	profiles, status, err := store.ReconcileCurrent(codexDir)
	if err != nil || len(profiles) != 1 || profiles[0].ID != central.ID || status.CurrentProfileID != central.ID || profiles[0].Source != "central_login" {
		t.Fatalf("profiles=%#v status=%#v err=%v", profiles, status, err)
	}
	if _, err := os.Stat(filepath.Join(store.Root, local.ID)); !os.IsNotExist(err) {
		t.Fatalf("duplicate local profile still exists: %v", err)
	}
}

func TestReconcileCurrentRenamesLegacyDifferentCentralCollision(t *testing.T) {
	codexDir := t.TempDir()
	store := NewStore(t.TempDir())
	local, err := store.CreateAPIKey(APIKeyRequest{Name: "yangpu", Provider: "yangpu", BaseURL: "https://local.example/v1", APIKey: "sk-local"})
	if err != nil {
		t.Fatal(err)
	}
	localAuth, localConfig, err := store.GetRawContent(local.ID)
	if err != nil {
		t.Fatal(err)
	}
	writeCurrentAuth(t, codexDir, localAuth, localConfig)
	central, err := store.AddCentral(centralFixture())
	if err != nil {
		t.Fatal(err)
	}

	profiles, status, err := store.ReconcileCurrent(codexDir)
	if err != nil || len(profiles) != 2 || status.CurrentProfileID != local.ID {
		t.Fatalf("profiles=%#v status=%#v err=%v", profiles, status, err)
	}
	for _, profile := range profiles {
		switch profile.ID {
		case local.ID:
			if profile.Name != "yangpu (Local)" || profile.ContentHash != local.ContentHash {
				t.Fatalf("local profile was not preserved: before=%#v after=%#v", local, profile)
			}
		case central.ID:
			if profile.Name != "yangpu" || profile.Source != "central_login" {
				t.Fatalf("Central profile changed: %#v", profile)
			}
		}
	}
}

func TestReconcileCurrentUpdatesDerivedLocalProfileWithoutDuplicates(t *testing.T) {
	codexDir := t.TempDir()
	store := NewStore(t.TempDir())
	_, err := store.AddCentral(CentralProfile{
		Name: "corp", Provider: "corp", Type: "api_key",
		AuthJSON:   []byte(`{"OPENAI_API_KEY":"sk-central-secret"}`),
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

func TestReconcileCurrentRejectsCentralProfileUsingDerivedLocalName(t *testing.T) {
	codexDir := t.TempDir()
	writeCurrentAuth(t, codexDir, `{"OPENAI_API_KEY":"sk-current-secret"}`, "model_provider = \"corp\"\n\n[model_providers.corp]\nbase_url = \"https://current.example/v1\"\n")
	store := NewStore(t.TempDir())
	_, err := store.AddCentral(CentralProfile{
		Name: "corp", Provider: "corp", Type: "api_key",
		AuthJSON:   []byte(`{"OPENAI_API_KEY":"sk-central-secret"}`),
		ConfigTOML: []byte("model_provider = \"corp\"\n\n[model_providers.corp]\nbase_url = \"https://central.example/v1\"\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	derived := AuthProfile{ID: uuid.NewString(), Name: "corp (Local)", Type: "api_key", Provider: "corp", Source: "central_login", ReservedName: "corp (Local)", CreatedAt: now, UpdatedAt: now}
	derivedDir := filepath.Join(store.Root, derived.ID)
	if err := os.MkdirAll(derivedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeProfile(derivedDir, derived, []byte(`{"OPENAI_API_KEY":"sk-derived-central"}`), []byte("model_provider = \"corp\"\n\n[model_providers.corp]\nbase_url = \"https://derived.example/v1\"\n")); err != nil {
		t.Fatal(err)
	}
	beforeDerivedAuth, beforeDerivedConfig, err := store.GetRawContent(derived.ID)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = store.ReconcileCurrent(codexDir)
	if err == nil || err.Error() != `cannot automatically replace Central profile "corp (Local)"` {
		t.Fatalf("err=%v", err)
	}
	afterDerivedAuth, afterDerivedConfig, err := store.GetRawContent(derived.ID)
	if err != nil {
		t.Fatal(err)
	}
	if beforeDerivedAuth != afterDerivedAuth || beforeDerivedConfig != afterDerivedConfig {
		t.Fatal("derived Central profile changed")
	}
	profiles, err := store.List()
	if err != nil || len(profiles) != 2 {
		t.Fatalf("profiles=%#v err=%v", profiles, err)
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
