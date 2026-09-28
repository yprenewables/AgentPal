package authprofile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestActivatePreservesMCPAndProjects(t *testing.T) {
	codexDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(codexDir, "auth.json"), []byte(`{"old":true}`), 0o600)
	_ = os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte("model = \"old\"\nmodel_provider = \"oldp\"\n\n[mcp_servers.keep]\ncommand = \"local\"\n\n[projects.demo]\ntrust_level = \"trusted\"\n"), 0o600)
	store := NewStore(t.TempDir())
	profile, err := store.CreateAPIKey(APIKeyRequest{Name: "New", Provider: "corp", BaseURL: "http://models", APIKey: "sk-new"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Activate(codexDir, profile.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.BackupPath == "" {
		t.Fatal("missing backup")
	}
	data, _ := os.ReadFile(filepath.Join(codexDir, "config.toml"))
	got := string(data)
	if !strings.Contains(got, "[mcp_servers.keep]") || !strings.Contains(got, "[projects.demo]") || !strings.Contains(got, "model = \"old\"") || !strings.Contains(got, "[model_providers.corp]") || !strings.Contains(got, "name = \"corp\"") {
		t.Fatalf("config: %s", got)
	}
}

func TestActivateStopsBeforeChangingAnythingOnProviderConflict(t *testing.T) {
	codexDir := t.TempDir()
	originalAuth := []byte(`{"old":true}`)
	originalConfig := []byte("model = \"old\"\nmodel_provider = \"corp\"\n\n[model_providers.corp]\nbase_url = \"http://local\"\n")
	_ = os.WriteFile(filepath.Join(codexDir, "auth.json"), originalAuth, 0o600)
	_ = os.WriteFile(filepath.Join(codexDir, "config.toml"), originalConfig, 0o600)
	store := NewStore(t.TempDir())
	profile, err := store.CreateAPIKey(APIKeyRequest{Name: "New", Provider: "corp", BaseURL: "http://profile", APIKey: "sk-new"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Activate(codexDir, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !result.ProviderConflict || result.OK {
		t.Fatalf("expected conflict without activation: %#v", result)
	}
	auth, _ := os.ReadFile(filepath.Join(codexDir, "auth.json"))
	config, _ := os.ReadFile(filepath.Join(codexDir, "config.toml"))
	if string(auth) != string(originalAuth) || string(config) != string(originalConfig) {
		t.Fatalf("conflict changed local files: auth=%s config=%s", auth, config)
	}
}
