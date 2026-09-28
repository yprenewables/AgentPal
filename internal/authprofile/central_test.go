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
