package authprofile

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestSyncCentralSerializesWithCurrentReconciliation(t *testing.T) {
	store := NewStore(t.TempDir())
	reconcileMu.Lock()
	done := make(chan error, 1)
	go func() {
		_, _, err := store.SyncCentral(centralFixture())
		done <- err
	}()

	select {
	case err := <-done:
		reconcileMu.Unlock()
		t.Fatalf("SyncCentral bypassed reconciliation lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	reconcileMu.Unlock()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("SyncCentral did not continue after reconciliation lock was released")
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
	if err != nil || status != "needs_sync" || updated.ID != first.ID || updated.ContentHash != first.ContentHash {
		t.Fatalf("updated sync profile=%#v status=%q err=%v", updated, status, err)
	}
	for name, expected := range map[string]string{"auth.json": string(centralFixture().AuthJSON), "config-auth.toml": string(centralFixture().ConfigTOML)} {
		got, err := os.ReadFile(filepath.Join(store.Root, first.ID, name))
		if err != nil || string(got) != expected {
			t.Fatalf("%s=%q err=%v", name, got, err)
		}
	}
}

func TestSyncCentralPreservesActiveProfileUntilExplicitUpdate(t *testing.T) {
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
	if err != nil || string(auth) != string(centralFixture().AuthJSON) {
		t.Fatalf("active cache auth=%s err=%v", auth, err)
	}
	for i := 0; i < 2; i++ {
		if _, status, err := store.SyncCentral(next); err != nil || status != "needs_sync" {
			t.Fatalf("repeat check status=%s err=%v", status, err)
		}
	}
	if _, err := store.OverwriteCentral(first.ID, next); err != nil {
		t.Fatal(err)
	}
	if _, status, err := store.SyncCentral(next); err != nil || status != "synced" {
		t.Fatalf("after update status=%s err=%v", status, err)
	}
}

func TestSyncCentralConvertsIdenticalSameNameLocalProfile(t *testing.T) {
	store := NewStore(t.TempDir())
	local, err := store.CreateRaw(RawProfileRequest{Name: "yangpu", AuthJSON: string(centralFixture().AuthJSON), ConfigTOML: string(centralFixture().ConfigTOML)})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.writeActiveProfileID(local.ID); err != nil {
		t.Fatal(err)
	}

	profile, status, err := store.SyncCentral(centralFixture())
	if err != nil || status != "added" || profile.ID != local.ID || profile.Source != "central_login" || profile.ReservedName != "yangpu" {
		t.Fatalf("profile=%#v status=%q err=%v", profile, status, err)
	}
	profiles, err := store.List()
	if err != nil || len(profiles) != 1 || !profiles[0].Active {
		t.Fatalf("profiles=%#v err=%v", profiles, err)
	}
}

func TestSyncCentralRenamesDifferentSameNameLocalProfile(t *testing.T) {
	store := NewStore(t.TempDir())
	local, err := store.CreateAPIKey(APIKeyRequest{Name: "yangpu", Provider: "yangpu", BaseURL: "https://local.example/v1", APIKey: "sk-local"})
	if err != nil {
		t.Fatal(err)
	}

	central, status, err := store.SyncCentral(centralFixture())
	if err != nil || status != "added" || central.Name != "yangpu" {
		t.Fatalf("central=%#v status=%q err=%v", central, status, err)
	}
	profiles, err := store.List()
	if err != nil || len(profiles) != 2 {
		t.Fatalf("profiles=%#v err=%v", profiles, err)
	}
	for _, profile := range profiles {
		if profile.ID == local.ID {
			if profile.Name != "yangpu (Local)" || profile.ContentHash != local.ContentHash || profile.Source != local.Source {
				t.Fatalf("local profile was not preserved: before=%#v after=%#v", local, profile)
			}
			return
		}
	}
	t.Fatalf("renamed local profile not found: %#v", profiles)
}

func TestSyncCentralRejectsOccupiedLocalSuffixWithoutChanges(t *testing.T) {
	store := NewStore(t.TempDir())
	local, err := store.CreateAPIKey(APIKeyRequest{Name: "yangpu", Provider: "yangpu", BaseURL: "https://local.example/v1", APIKey: "sk-local"})
	if err != nil {
		t.Fatal(err)
	}
	occupied, err := store.CreateAPIKey(APIKeyRequest{Name: "yangpu (Local)", Provider: "other", BaseURL: "https://other.example/v1", APIKey: "sk-other"})
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := store.SyncCentral(centralFixture()); err == nil || err.Error() != `profile name "yangpu (Local)" is already in use` {
		t.Fatalf("err=%v", err)
	}
	profiles, err := store.List()
	if err != nil || len(profiles) != 2 {
		t.Fatalf("profiles=%#v err=%v", profiles, err)
	}
	for _, want := range []AuthProfile{local, occupied} {
		for _, got := range profiles {
			if got.ID == want.ID && (got.Name != want.Name || got.ContentHash != want.ContentHash) {
				t.Fatalf("profile changed: before=%#v after=%#v", want, got)
			}
		}
	}
}

func TestSyncCentralRejectsMultipleSameNameLocalsWithoutChanges(t *testing.T) {
	store := NewStore(t.TempDir())
	first, err := store.CreateAPIKey(APIKeyRequest{Name: "yangpu", Provider: "yangpu", BaseURL: "https://first.example/v1", APIKey: "sk-first"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateAPIKey(APIKeyRequest{Name: "yangpu", Provider: "yangpu", BaseURL: "https://second.example/v1", APIKey: "sk-second"})
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := store.SyncCentral(centralFixture()); err == nil || err.Error() != `multiple local profiles use Central name "yangpu"` {
		t.Fatalf("err=%v", err)
	}
	profiles, err := store.List()
	if err != nil || len(profiles) != 2 {
		t.Fatalf("profiles=%#v err=%v", profiles, err)
	}
	for _, want := range []AuthProfile{first, second} {
		for _, got := range profiles {
			if got.ID == want.ID && (got.Name != want.Name || got.ContentHash != want.ContentHash) {
				t.Fatalf("profile changed: before=%#v after=%#v", want, got)
			}
		}
	}
}

func TestSyncCentralUsesReservedNameWhenCachedDisplayNameIsStale(t *testing.T) {
	store := NewStore(t.TempDir())
	central, err := store.AddCentral(centralFixture())
	if err != nil {
		t.Fatal(err)
	}
	auth, config, err := store.GetRawContent(central.ID)
	if err != nil {
		t.Fatal(err)
	}
	central.Name = "stale-name"
	if err := writeProfile(filepath.Join(store.Root, central.ID), central, []byte(auth), []byte(config)); err != nil {
		t.Fatal(err)
	}
	local, err := store.CreateAPIKey(APIKeyRequest{Name: "yangpu", Provider: "yangpu", BaseURL: "https://local.example/v1", APIKey: "sk-local"})
	if err != nil {
		t.Fatal(err)
	}

	synced, status, err := store.SyncCentral(centralFixture())
	if err != nil || status != "synced" || synced.ID != central.ID || synced.Name != "yangpu" {
		t.Fatalf("synced=%#v status=%q err=%v", synced, status, err)
	}
	profiles, err := store.List()
	if err != nil || len(profiles) != 2 {
		t.Fatalf("profiles=%#v err=%v", profiles, err)
	}
	for _, profile := range profiles {
		if profile.ID == local.ID && profile.Name != "yangpu (Local)" {
			t.Fatalf("local profile did not yield the reserved name: %#v", profile)
		}
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
