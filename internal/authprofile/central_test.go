package authprofile

import (
	"os"
	"path/filepath"
	"testing"
)

func centralFixture() CentralProfile {
	return CentralProfile{
		Name:       "yangpu",
		Provider:   "yangpu",
		Type:       "api_key",
		AuthJSON:   []byte(`{"OPENAI_API_KEY":"sk-central"}`),
		ConfigTOML: []byte("model_provider = \"yangpu\"\n\n[model_providers.yangpu]\nname = \"yangpu\"\nbase_url = \"https://central.example/v1\"\n"),
	}
}

func TestAddCentralCreatesReservedProfile(t *testing.T) {
	store := NewStore(t.TempDir())
	profile, err := store.AddCentral(centralFixture())
	if err != nil {
		t.Fatal(err)
	}
	if profile.ReservedName != "yangpu" || profile.Source != "central_login" || profile.ContentHash == "" {
		t.Fatalf("unexpected profile: %#v", profile)
	}
	found, ok, err := store.FindByReservedName("yangpu")
	if err != nil || !ok || found.ID != profile.ID {
		t.Fatalf("find: ok=%v err=%v found=%#v", ok, err, found)
	}
	if _, ok, _ := store.FindByReservedName("other"); ok {
		t.Fatal("unexpected match for other")
	}
}

func TestAddCentralRejectsProviderMismatch(t *testing.T) {
	store := NewStore(t.TempDir())
	bad := centralFixture()
	bad.ConfigTOML = []byte("model_provider = \"other\"\n\n[model_providers.other]\nbase_url = \"u\"\n")
	if _, err := store.AddCentral(bad); err == nil {
		t.Fatal("provider mismatch accepted")
	}
}

func TestOverwriteCentralReplacesFilesAndKeepsID(t *testing.T) {
	store := NewStore(t.TempDir())
	profile, err := store.AddCentral(centralFixture())
	if err != nil {
		t.Fatal(err)
	}
	next := centralFixture()
	next.AuthJSON = []byte(`{"OPENAI_API_KEY":"sk-updated"}`)
	next.ConfigTOML = []byte("model_provider = \"yangpu\"\n\n[model_providers.yangpu]\nname = \"yangpu\"\nbase_url = \"https://updated.example/v1\"\n")
	updated, err := store.OverwriteCentral(profile.ID, next)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != profile.ID {
		t.Fatalf("id changed: %q -> %q", profile.ID, updated.ID)
	}
	data, err := os.ReadFile(filepath.Join(store.Root, profile.ID, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"OPENAI_API_KEY":"sk-updated"}` {
		t.Fatalf("auth.json not overwritten: %s", data)
	}
	if updated.ContentHash == profile.ContentHash {
		t.Fatal("content hash did not change")
	}
}

func TestContentHashForReadsStoredFiles(t *testing.T) {
	store := NewStore(t.TempDir())
	profile, err := store.AddCentral(centralFixture())
	if err != nil {
		t.Fatal(err)
	}
	hash, err := store.ContentHashFor(profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if hash != profile.ContentHash {
		t.Fatalf("hash=%q profile=%q", hash, profile.ContentHash)
	}
}

func TestSyncCentralAddsCompleteProfile(t *testing.T) {
	store := NewStore(t.TempDir())
	profile, status, err := store.SyncCentral(centralFixture())
	if err != nil {
		t.Fatal(err)
	}
	if status != "added" || profile.Source != "central_login" || profile.ReservedName != "yangpu" || profile.ContentHash == "" {
		t.Fatalf("profile=%#v status=%q", profile, status)
	}
	for name, expected := range map[string]string{
		"auth.json":        string(centralFixture().AuthJSON),
		"config-auth.toml": string(centralFixture().ConfigTOML),
	} {
		got, err := os.ReadFile(filepath.Join(store.Root, profile.ID, name))
		if err != nil || string(got) != expected {
			t.Fatalf("%s=%q err=%v", name, got, err)
		}
	}
}

func TestSyncCentralKeepsIDAndReportsContentState(t *testing.T) {
	store := NewStore(t.TempDir())
	first, status, err := store.SyncCentral(centralFixture())
	if err != nil || status != "added" {
		t.Fatalf("first sync profile=%#v status=%q err=%v", first, status, err)
	}
	unchanged, status, err := store.SyncCentral(centralFixture())
	if err != nil || status != "synced" || unchanged.ID != first.ID {
		t.Fatalf("unchanged sync profile=%#v status=%q err=%v", unchanged, status, err)
	}
	next := centralFixture()
	next.AuthJSON = []byte(`{"OPENAI_API_KEY":"sk-updated"}`)
	next.ConfigTOML = []byte("model_provider = \"yangpu\"\n\n[model_providers.yangpu]\nname = \"yangpu\"\nbase_url = \"https://updated.example/v1\"\n")
	updated, status, err := store.SyncCentral(next)
	if err != nil || status != "needs_sync" || updated.ID != first.ID || updated.ContentHash == first.ContentHash {
		t.Fatalf("updated sync profile=%#v status=%q err=%v", updated, status, err)
	}
	for name, expected := range map[string]string{"auth.json": string(next.AuthJSON), "config-auth.toml": string(next.ConfigTOML)} {
		got, err := os.ReadFile(filepath.Join(store.Root, first.ID, name))
		if err != nil || string(got) != expected {
			t.Fatalf("%s=%q err=%v", name, got, err)
		}
	}
}

func TestSyncCentralUpdatesActiveProfileCache(t *testing.T) {
	store := NewStore(t.TempDir())
	first, _, err := store.SyncCentral(centralFixture())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Root, "active.json"), []byte(first.ID), 0o600); err != nil {
		t.Fatal(err)
	}
	next := centralFixture()
	next.AuthJSON = []byte(`{"OPENAI_API_KEY":"sk-active-updated"}`)
	updated, status, err := store.SyncCentral(next)
	if err != nil || status != "needs_sync" || updated.ID != first.ID {
		t.Fatalf("updated=%#v status=%q err=%v", updated, status, err)
	}
	auth, err := os.ReadFile(filepath.Join(store.Root, first.ID, "auth.json"))
	if err != nil || string(auth) != string(next.AuthJSON) {
		t.Fatalf("active cache auth=%s err=%v", auth, err)
	}
}

func TestSyncCentralNeverOverwritesManualProfile(t *testing.T) {
	store := NewStore(t.TempDir())
	manual, err := store.CreateAPIKey(APIKeyRequest{Name: "yangpu", Provider: "openai", BaseURL: "https://local.example/v1", APIKey: "sk-local"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.SyncCentral(centralFixture()); err == nil {
		t.Fatal("expected a same-name manual profile conflict")
	}
	after, err := readSavedProfile(filepath.Join(store.Root, manual.ID))
	if err != nil || after.Source != "manual" || after.APIKeyPreview != manual.APIKeyPreview {
		t.Fatalf("manual profile changed: %#v err=%v", after, err)
	}
}

func TestSyncCentralInvalidUpdatePreservesExistingFiles(t *testing.T) {
	store := NewStore(t.TempDir())
	profile, _, err := store.SyncCentral(centralFixture())
	if err != nil {
		t.Fatal(err)
	}
	bad := centralFixture()
	bad.AuthJSON = []byte(`{"OPENAI_API_KEY":"sk-new"}`)
	bad.ConfigTOML = []byte("model_provider = \"other\"\n")
	if _, _, err := store.SyncCentral(bad); err == nil {
		t.Fatal("invalid central profile was accepted")
	}
	stored, err := readSavedProfile(filepath.Join(store.Root, profile.ID))
	if err != nil || stored.ContentHash != profile.ContentHash {
		t.Fatalf("stored profile changed after rejected update: %#v err=%v", stored, err)
	}
}
