package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"time"

	"agentpal/internal/authprofile"
	appconfig "agentpal/internal/config"
	"agentpal/internal/login"
)

func TestDisconnectedRefreshHasEmptyItems(t *testing.T) {
	got := disconnectedRefresh("connection refused")
	if got.Connected {
		t.Fatal("disconnected refresh reported connected")
	}
	if got.Diff.Items == nil {
		t.Fatal("disconnected refresh items must serialize as [] instead of null")
	}
}

func TestProfileTypeKeepsEachSavedProfileConfiguration(t *testing.T) {
	profile := authprofile.AuthProfile{ID: "profile-1", Name: "Saved", Type: "api_key", Provider: "saved-provider", APIKeyPreview: "sk-••••1234", BaseURL: "https://saved.example/v1"}
	got := profileType(profile)
	if got.Provider != "saved-provider" || got.APIKeyPreview != "sk-••••1234" || got.BaseURL != "https://saved.example/v1" {
		t.Fatalf("profile mapping lost saved config: %#v", got)
	}
}

func TestProfileListDoesNotOverwriteSavedProfilesWithCurrentAuth(t *testing.T) {
	profiles := []authprofile.AuthProfile{
		{ID: "active", Name: "Active", Type: "api_key", Provider: "saved-active", APIKeyPreview: "sk-••••aaaa", BaseURL: "https://active.example/v1"},
		{ID: "other", Name: "Other", Type: "api_key", Provider: "saved-other", APIKeyPreview: "sk-••••bbbb", BaseURL: "https://other.example/v1"},
	}
	current := authprofile.CurrentStatus{CurrentProfileID: "active", APIKeyImported: true, APIKeyPreview: "sk-••••live", BaseURL: "https://live.example/v1"}
	got := profileTypesFromStore(profiles, current)
	if len(got) != 2 {
		t.Fatalf("profiles=%#v", got)
	}
	if !got[0].Active || got[0].APIKeyPreview != "sk-••••aaaa" || got[0].BaseURL != "https://active.example/v1" {
		t.Fatalf("active profile did not retain its saved snapshot: %#v", got[0])
	}
	if got[1].Active || got[1].Provider != "saved-other" || got[1].APIKeyPreview != "sk-••••bbbb" || got[1].BaseURL != "https://other.example/v1" {
		t.Fatalf("other profile was overwritten by current auth: %#v", got[1])
	}
}

func setupCurrentAuthApp(t *testing.T, auth, config string) (*App, appconfig.State, string) {
	t.Helper()
	home := t.TempDir()
	configRoot := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGENTPAL_CONFIG_DIR", configRoot)
	state := appconfig.State{CodexDir: filepath.Join(configRoot, "codex"), CentralHost: "127.0.0.1", CentralPort: 8765}
	if err := appconfig.WriteState(state); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(state.CodexDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state.CodexDir, "auth.json"), []byte(auth), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state.CodexDir, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	return NewApp(), state, filepath.Join(home, ".agentpal", "auth-profiles")
}

func TestListAuthProfilesImportsAndSelectsCurrentAPIKey(t *testing.T) {
	app, _, _ := setupCurrentAuthApp(t, `{"OPENAI_API_KEY":"sk-current-secret"}`, "model_provider = \"corp\"\n\n[model_providers.corp]\nbase_url = \"https://corp.example/v1\"\n")

	profiles, err := app.ListAuthProfiles()
	if err != nil || len(profiles) != 1 || profiles[0].Name != "corp" || !profiles[0].Active {
		t.Fatalf("profiles=%#v err=%v", profiles, err)
	}
}

func TestListAuthProfilesUpdatesSameNameAndSelectsIt(t *testing.T) {
	app, _, profilesRoot := setupCurrentAuthApp(t, `{"OPENAI_API_KEY":"sk-current-secret"}`, "model_provider = \"corp\"\n\n[model_providers.corp]\nbase_url = \"https://current.example/v1\"\n")
	old, err := authprofile.NewStore(profilesRoot).CreateAPIKey(authprofile.APIKeyRequest{Name: "corp", Provider: "corp", BaseURL: "https://old.example/v1", APIKey: "sk-old-secret"})
	if err != nil {
		t.Fatal(err)
	}

	profiles, err := app.ListAuthProfiles()
	if err != nil || len(profiles) != 1 || profiles[0].ID != old.ID || profiles[0].BaseURL != "https://current.example/v1" || !profiles[0].Active {
		t.Fatalf("old=%#v profiles=%#v err=%v", old, profiles, err)
	}
}

func TestListAuthProfilesImportsLocalProfileBesideSameNameCentral(t *testing.T) {
	app, _, profilesRoot := setupCurrentAuthApp(t, `{"OPENAI_API_KEY":"sk-current-secret"}`, "model_provider = \"corp\"\n\n[model_providers.corp]\nbase_url = \"https://current.example/v1\"\n")
	_, err := authprofile.NewStore(profilesRoot).AddCentral(authprofile.CentralProfile{
		Name: "corp", Provider: "corp", Type: "api_key",
		AuthJSON:   []byte(`{"OPENAI_API_KEY":"sk-central-secret"}`),
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

func TestListAuthProfilesAfterCentralActivationDoesNotCreateLocalDuplicate(t *testing.T) {
	app, state, profilesRoot := setupCurrentAuthApp(t, `{"OPENAI_API_KEY":"sk-old"}`, "model_provider = \"old\"\n\n[model_providers.old]\nbase_url = \"https://old.example/v1\"\n")
	store := authprofile.NewStore(profilesRoot)
	manual, err := store.CreateAPIKey(authprofile.APIKeyRequest{Name: "Personal memory", Provider: "memory", BaseURL: "https://local.example/v1", APIKey: "sk-local"})
	if err != nil {
		t.Fatal(err)
	}
	central, err := store.AddCentral(authprofile.CentralProfile{
		Name: "memory", Provider: "memory", Type: "api_key",
		AuthJSON:   []byte(`{"OPENAI_API_KEY":"sk-mem-central"}`),
		ConfigTOML: []byte("model_provider = \"memory\"\n\n[model_providers.memory]\nname = \"memory\"\nbase_url = \"https://memory.example/v1\"\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := app.ActivateAuthProfileWithConflict(central.ID, true)
	if err != nil || !result.OK {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if _, err := os.Stat(filepath.Join(state.CodexDir, "auth.json")); err != nil {
		t.Fatal(err)
	}

	profiles, err := app.ListAuthProfiles()
	if err != nil || len(profiles) != 2 {
		t.Fatalf("profiles=%#v err=%v", profiles, err)
	}
	foundCentral := false
	foundManual := false
	for _, profile := range profiles {
		if profile.Name == "memory (Local)" {
			t.Fatalf("Central activation created a derived local profile: %#v", profiles)
		}
		if profile.ID == central.ID && (!profile.Active || profile.Source != "central_login") {
			t.Fatalf("Central profile is not current: %#v", profile)
		}
		if profile.ID == central.ID {
			foundCentral = true
		}
		if profile.ID == manual.ID && (profile.Name != manual.Name || profile.ContentHash != manual.ContentHash || profile.Active) {
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

func TestGetCurrentAuthStatusDoesNotImportProfile(t *testing.T) {
	app, _, profilesRoot := setupCurrentAuthApp(t, `{"OPENAI_API_KEY":"sk-current-secret"}`, "model_provider = \"corp\"\n\n[model_providers.corp]\nbase_url = \"https://corp.example/v1\"\n")

	status, err := app.GetCurrentAuthStatus()
	if err != nil || status.APIKeyPreview == "" {
		t.Fatalf("status=%#v err=%v", status, err)
	}
	profiles, err := authprofile.NewStore(profilesRoot).List()
	if err != nil || len(profiles) != 0 {
		t.Fatalf("profiles=%#v err=%v", profiles, err)
	}
}

func TestCentralProfileSyncCachesAndLogoutPreservesProfiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	configRoot := t.TempDir()
	t.Setenv("AGENTPAL_CONFIG_DIR", configRoot)
	state := appconfig.State{CodexDir: filepath.Join(configRoot, "codex"), CentralHost: "127.0.0.1", CentralPort: 8765}
	if err := appconfig.WriteState(state); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(state.CodexDir, 0o700); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v1/auth-profiles":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"name": "work", "provider": "work", "type": "api_key", "contentHash": "hash"}})
		case "/v1/auth-profiles/work":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "work", "provider": "work", "type": "api_key",
				"auth":           map[string]any{"OPENAI_API_KEY": "sk-test"},
				"configAuthToml": "model_provider = \"work\"\n\n[model_providers.work]\nname = \"work\"\nbase_url = \"https://example.test/v1\"\nenv_key = \"OPENAI_API_KEY\"\nwire_api = \"responses\"\n",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	centralHost, centralPort, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(centralPort)
	if err != nil {
		t.Fatal(err)
	}
	state.CentralHost, state.CentralPort = centralHost, port
	if err := appconfig.WriteState(state); err != nil {
		t.Fatal(err)
	}
	app := NewApp()
	if _, err := app.SyncCentralAuthProfiles(); err == nil {
		t.Fatal("sync should reject unauthenticated requests")
	}
	stateRoot, err := appconfig.StateDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := login.Write(stateRoot, login.State{AccessToken: "test-token", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	report, err := app.SyncCentralAuthProfiles()
	if err != nil || len(report.Items) != 1 || report.Items[0].Status != "added" {
		t.Fatalf("sync report=%#v err=%v", report, err)
	}
	local, err := app.ListAuthProfiles()
	if err != nil || len(local) != 1 || local[0].Source != "central_login" || local[0].ReservedName != "work" {
		t.Fatalf("local list does not include central cache: %#v err=%v", local, err)
	}
	if err := app.LogoutCentralLogin(); err != nil {
		t.Fatal(err)
	}
	local, err = app.ListAuthProfiles()
	if err != nil || len(local) != 1 || local[0].Source != "central_login" {
		t.Fatalf("logout removed cached central profile: %#v err=%v", local, err)
	}
	configPath := filepath.Join(state.CodexDir, "config.toml")
	config := "model_provider = \"work\"\n\n[model_providers.work]\nname = \"work\"\nbase_url = \"https://example.test/v1\"\nenv_key = \"OPENAI_API_KEY\"\nwire_api = \"responses\"\n"
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	profilesRoot := filepath.Join(os.Getenv("HOME"), ".agentpal", "auth-profiles")
	result, err := authprofile.NewStore(profilesRoot).Activate(state.CodexDir, local[0].ID)
	if err != nil || !result.OK {
		t.Fatalf("offline cached activation=%#v err=%v", result, err)
	}
}
