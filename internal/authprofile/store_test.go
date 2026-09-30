package authprofile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreCreatesSecureProfileAndListsIt(t *testing.T) {
	store := NewStore(t.TempDir())
	profile, err := store.CreateAPIKey(APIKeyRequest{Name: "Work", Provider: "openai", BaseURL: "https://api.openai.com/v1", APIKey: "sk-test"})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Name != "Work" || profile.Type != "api_key" {
		t.Fatalf("profile: %#v", profile)
	}
	if profile.BaseURL != "https://api.openai.com/v1" || profile.ChatGPTUserID != "" {
		t.Fatalf("API Key profile identity fields: %#v", profile)
	}
	info, err := os.Stat(filepath.Join(store.Root, profile.ID))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %o", info.Mode().Perm())
	}
	profiles, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("profiles: %#v", profiles)
	}
	config, err := os.ReadFile(filepath.Join(store.Root, profile.ID, "config-auth.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(config), "model =") || strings.Contains(string(config), "model_reasoning_effort") || !strings.Contains(string(config), "model_provider = \"openai\"") || strings.Contains(string(config), "model_reasoning_summary") || strings.Contains(string(config), "model_verbosity") {
		t.Fatalf("unexpected API key profile config:\n%s", config)
	}
}

func TestListUsesSavedAuthAndModelConfigurationAsProfile(t *testing.T) {
	store := NewStore(t.TempDir())
	profile, err := store.CreateAPIKey(APIKeyRequest{Name: "Current", Provider: "stale-provider", BaseURL: "https://stale.example/v1", APIKey: "sk-secret-value"})
	if err != nil {
		t.Fatal(err)
	}
	profileDir := filepath.Join(store.Root, profile.ID)
	if err := os.WriteFile(filepath.Join(profileDir, "config-auth.toml"), []byte("model = \"model-from-backup\"\nmodel_provider = \"provider-from-backup\"\n\n[model_providers.provider-from-backup]\nbase_url = \"https://backup.example/v1\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "auth.json"), []byte(`{"OPENAI_API_KEY":"sk-secret-value"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	profiles, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("profiles=%#v", profiles)
	}
	got := profiles[0]
	if got.Provider != "provider-from-backup" || got.BaseURL != "https://backup.example/v1" {
		t.Fatalf("profile not derived from saved config: %#v", got)
	}
	if got.APIKeyPreview != "sk-••••••alue" {
		t.Fatalf("API key preview=%q", got.APIKeyPreview)
	}
}

func TestListSkipsProfilesMissingSavedAuthOrConfig(t *testing.T) {
	store := NewStore(t.TempDir())
	profile, err := store.CreateAPIKey(APIKeyRequest{Name: "Broken", Provider: "provider", BaseURL: "https://example/v1", APIKey: "sk-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(store.Root, profile.ID, "auth.json")); err != nil {
		t.Fatal(err)
	}
	profiles, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 0 {
		t.Fatalf("incomplete backup was listed: %#v", profiles)
	}
}

func TestStoreRejectsInvalidAPIKeyProfile(t *testing.T) {
	_, err := NewStore(t.TempDir()).CreateAPIKey(APIKeyRequest{Name: "", Provider: "openai", BaseURL: "not-url", APIKey: ""})
	if err == nil {
		t.Fatal("invalid API profile accepted")
	}
}

func TestStoreUpdatesAPIKeyProfileAndKeepsKeyWhenOmitted(t *testing.T) {
	store := NewStore(t.TempDir())
	profile, err := store.CreateAPIKey(APIKeyRequest{Name: "Work", Provider: "openai", BaseURL: "https://api.openai.com/v1", APIKey: "sk-original"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.UpdateAPIKey(profile.ID, APIKeyRequest{Name: "Personal", Provider: "corp", BaseURL: "https://models.example/v1"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != profile.ID || updated.Name != "Personal" || updated.Provider != "corp" || updated.BaseURL != "https://models.example/v1" {
		t.Fatalf("profile metadata was not updated: %#v", updated)
	}
	if updated.APIKeyPreview != maskAPIKey("sk-original") {
		t.Fatalf("existing API key was not preserved: %q", updated.APIKeyPreview)
	}

	updated, err = store.UpdateAPIKey(profile.ID, APIKeyRequest{Name: "Personal", Provider: "corp", BaseURL: "https://models.example/v1", APIKey: "sk-new"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.APIKeyPreview != maskAPIKey("sk-new") {
		t.Fatalf("API key was not replaced: %q", updated.APIKeyPreview)
	}
}

func TestStoreDeleteRemovesInactiveProfile(t *testing.T) {
	store := NewStore(t.TempDir())
	profile, err := store.CreateAPIKey(APIKeyRequest{Name: "Remove", Provider: "openai", BaseURL: "https://api.openai.com/v1", APIKey: "sk-test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(profile.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.Root, profile.ID)); !os.IsNotExist(err) {
		t.Fatalf("profile directory still exists: %v", err)
	}
}

func TestStoreDeleteRejectsActiveProfile(t *testing.T) {
	store := NewStore(t.TempDir())
	profile, err := store.CreateAPIKey(APIKeyRequest{Name: "Current", Provider: "openai", BaseURL: "https://api.openai.com/v1", APIKey: "sk-test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Root, "active.json"), []byte(profile.ID), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(profile.ID); err == nil {
		t.Fatal("active profile deletion succeeded")
	}
	if _, err := os.Stat(filepath.Join(store.Root, profile.ID, "auth.json")); err != nil {
		t.Fatalf("active profile was removed: %v", err)
	}
}

func TestStoreDeleteIgnoresStaleActiveMarkerWhenCurrentProfileDiffers(t *testing.T) {
	store := NewStore(t.TempDir())
	profile, err := store.CreateAPIKey(APIKeyRequest{Name: "Stale", Provider: "openai", BaseURL: "https://api.openai.com/v1", APIKey: "sk-test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Root, "active.json"), []byte(profile.ID), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(profile.ID, "different-current-profile"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.Root, profile.ID)); !os.IsNotExist(err) {
		t.Fatalf("stale profile directory still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.Root, "active.json")); !os.IsNotExist(err) {
		t.Fatalf("stale active marker still exists: %v", err)
	}
}

func TestSavedProfileCarriesContentHashAndReservedName(t *testing.T) {
	store := NewStore(t.TempDir())
	profile, err := store.CreateAPIKey(APIKeyRequest{Name: "yangpu", Provider: "yangpu", BaseURL: "https://example/v1", APIKey: "sk-test"})
	if err != nil {
		t.Fatal(err)
	}
	if profile.ContentHash == "" {
		t.Fatal("created profile has no content hash")
	}
	if profile.ReservedName != "" {
		t.Fatalf("locally created profile must not have reservedName: %#v", profile)
	}
	profiles, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].ContentHash == "" {
		t.Fatalf("listed profile missing content hash: %#v", profiles)
	}
	if profiles[0].ContentHash != profile.ContentHash {
		t.Fatalf("hash mismatch: %q vs %q", profiles[0].ContentHash, profile.ContentHash)
	}
}
