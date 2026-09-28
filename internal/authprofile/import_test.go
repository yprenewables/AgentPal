package authprofile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportCurrentCopiesAuthAndConfigFragment(t *testing.T) {
	codexDir := t.TempDir()
	authJSON := []byte("{\n  \"tokens\": {\"id\": \"x\"},\n  \"nested\": {\"session\": \"preserved\"}\n}\n")
	_ = os.WriteFile(filepath.Join(codexDir, "auth.json"), authJSON, 0o600)
	_ = os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte("model = \"gpt-5\"\nmodel_provider = \"corp\"\nmodel_reasoning_effort = \"high\"\nmodel_reasoning_summary = \"not-in-profile\"\nmodel_verbosity = \"not-in-profile\"\n\n[model_providers.corp]\nbase_url = \"http://models\"\n\n[model_providers.corp.headers]\nX-Team = \"platform\"\n\n[model_providers.other]\nbase_url = \"http://other\"\n\n[mcp_servers.docs]\ncommand = \"npx\"\n"), 0o600)
	store := NewStore(t.TempDir())
	profile, err := store.ImportCurrent(codexDir, "Current")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Source != "local_import" || profile.Provider != "corp" {
		t.Fatalf("profile: %#v", profile)
	}
	if profile.ChatGPTUserID != "" || profile.APIKeyPreview != "" || profile.BaseURL != "" {
		t.Fatalf("ChatGPT profile identity fields: %#v", profile)
	}
	if _, err := os.Stat(filepath.Join(store.Root, profile.ID, "auth.json")); err != nil {
		t.Fatalf("saved auth.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.Root, profile.ID, "config-auth.toml")); err != nil {
		t.Fatalf("saved config-auth.toml: %v", err)
	}
	savedAuth, err := os.ReadFile(filepath.Join(store.Root, profile.ID, "auth.json"))
	if err != nil || string(savedAuth) != string(authJSON) {
		t.Fatalf("auth.json was not preserved byte-for-byte: %q err=%v", savedAuth, err)
	}
	savedConfig, err := os.ReadFile(filepath.Join(store.Root, profile.ID, "config-auth.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(savedConfig), "model =") || strings.Contains(string(savedConfig), "model_reasoning_effort") || !strings.Contains(string(savedConfig), "model_provider = \"corp\"") || !strings.Contains(string(savedConfig), "[model_providers.corp.headers]") || strings.Contains(string(savedConfig), "model_reasoning_summary") || strings.Contains(string(savedConfig), "model_verbosity") || strings.Contains(string(savedConfig), "model_providers.other") {
		t.Fatalf("unexpected profile config:\n%s", savedConfig)
	}
}
