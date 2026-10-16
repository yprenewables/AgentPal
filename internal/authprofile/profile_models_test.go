package authprofile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileModelsRoundTripAndMissingCatalog(t *testing.T) {
	s := NewStore(t.TempDir())
	dir := t.TempDir()
	a, err := s.CreateAPIKey(APIKeyRequest{Name: "A", Provider: "a", BaseURL: "https://a.test", APIKey: "key-a"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateAPIKey(APIKeyRequest{Name: "B", Provider: "b", BaseURL: "https://b.test", APIKey: "key-b"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Activate(dir, a.ID, true); err != nil {
		t.Fatal(err)
	}
	modelsA := []byte(`{"models":[{"slug":"a","custom":9007199254740993}],"custom":"preserved"}`)
	os.WriteFile(filepath.Join(dir, "models.json"), modelsA, 0600)
	if _, err := s.Activate(dir, b.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "models.json")); !os.IsNotExist(err) {
		t.Fatal("B inherited A models")
	}
	config, _ := os.ReadFile(filepath.Join(dir, "config.toml"))
	if strings.Contains(string(config), "model_catalog_json") {
		t.Fatal("B inherited catalog reference")
	}
	modelsB := []byte(`{"models":[{"slug":"b"}]}`)
	os.WriteFile(filepath.Join(dir, "models.json"), modelsB, 0600)
	for _, tc := range []struct {
		id   string
		data []byte
	}{{a.ID, modelsA}, {b.ID, modelsB}} {
		if _, err := s.Activate(dir, tc.id, true); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(dir, "models.json"))
		if err != nil || string(data) != string(tc.data) {
			t.Fatalf("wrong models: %s, %v", data, err)
		}
		config, _ := os.ReadFile(filepath.Join(dir, "config.toml"))
		if !strings.Contains(string(config), "model_catalog_json") {
			t.Fatal("missing catalog reference")
		}
	}
}

func TestProfileModelsRollbackAfterCatalogWriteFailure(t *testing.T) {
	s := NewStore(t.TempDir())
	dir := t.TempDir()
	a, _ := s.CreateAPIKey(APIKeyRequest{Name: "A", Provider: "a", BaseURL: "https://a.test", APIKey: "key-a"})
	b, _ := s.CreateAPIKey(APIKeyRequest{Name: "B", Provider: "b", BaseURL: "https://b.test", APIKey: "key-b"})
	s.Activate(dir, a.ID, true)
	original := []byte(`{"models":[{"slug":"a"}]}`)
	os.WriteFile(filepath.Join(dir, "models.json"), original, 0600)
	// Target catalog is unreadable as a file; switching must fail without changing active state.
	os.Mkdir(filepath.Join(s.Root, b.ID, "models.json"), 0700)
	beforeAuth, _ := os.ReadFile(filepath.Join(dir, "auth.json"))
	beforeConfig, _ := os.ReadFile(filepath.Join(dir, "config.toml"))
	if _, err := s.Activate(dir, b.ID, true); err == nil {
		t.Fatal("expected failure")
	}
	for path, expected := range map[string][]byte{filepath.Join(dir, "auth.json"): beforeAuth, filepath.Join(dir, "config.toml"): beforeConfig, filepath.Join(dir, "models.json"): original, filepath.Join(s.Root, "active.json"): []byte(a.ID)} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != string(expected) {
			t.Fatalf("not restored: %s, %v", path, err)
		}
	}
}

func TestReplaceProfilePreservesModels(t *testing.T) {
	s := NewStore(t.TempDir())
	p, _ := s.CreateAPIKey(APIKeyRequest{Name: "A", Provider: "a", BaseURL: "https://a.test", APIKey: "key-a"})
	path := filepath.Join(s.Root, p.ID, "models.json")
	models := []byte(`{"models":[{"slug":"a"}]}`)
	os.WriteFile(path, models, 0600)
	auth, config, _ := s.GetRawContent(p.ID)
	if err := s.replaceProfile(p, []byte(auth), []byte(config)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != string(models) {
		t.Fatalf("models lost: %s, %v", data, err)
	}
}

func TestProfileModelsMigratesExistingCurrentCatalog(t *testing.T) {
	s := NewStore(t.TempDir())
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "auth.json"), []byte(`{"OPENAI_API_KEY":"legacy"}`), 0600)
	os.WriteFile(filepath.Join(dir, "config.toml"), []byte("model_provider = \"legacy\"\n[model_providers.legacy]\nname = \"legacy\"\nbase_url = \"https://legacy.test\"\n"), 0600)
	models := []byte(`{"models":[{"slug":"legacy"}]}`)
	os.WriteFile(filepath.Join(dir, "models.json"), models, 0600)
	b, _ := s.CreateAPIKey(APIKeyRequest{Name: "B", Provider: "b", BaseURL: "https://b.test", APIKey: "key-b"})
	if _, err := s.Activate(dir, b.ID, true); err != nil {
		t.Fatal(err)
	}
	profiles, _ := s.List()
	for _, p := range profiles {
		if p.Name == "legacy" {
			if _, err := s.Activate(dir, p.ID, true); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(dir, "models.json"))
			if err != nil || string(data) != string(models) {
				t.Fatalf("legacy models lost: %s, %v", data, err)
			}
			return
		}
	}
	t.Fatal("legacy profile not imported")
}

func TestCurrentModelsSaveFailureRestoresWorkingFiles(t *testing.T) {
	s := NewStore(t.TempDir())
	dir := t.TempDir()
	a, _ := s.CreateAPIKey(APIKeyRequest{Name: "A", Provider: "a", BaseURL: "https://a.test", APIKey: "key-a"})
	s.Activate(dir, a.ID, true)
	models := []byte(`{"models":[{"slug":"original"}]}`)
	os.WriteFile(filepath.Join(dir, "models.json"), models, 0600)
	config, _ := os.ReadFile(filepath.Join(dir, "config.toml"))
	err := s.WithCurrentModels(dir, func() error {
		os.WriteFile(filepath.Join(dir, "models.json"), []byte(`{"models":[]}`), 0600)
		os.WriteFile(filepath.Join(dir, "config.toml"), []byte("invalid"), 0600)
		return os.ErrPermission
	})
	if err == nil {
		t.Fatal("expected failure")
	}
	for name, expected := range map[string][]byte{"models.json": models, "config.toml": config} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(data) != string(expected) {
			t.Fatalf("not restored: %s, %v", name, err)
		}
	}
}
