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

func TestActivateMemoryUserKeyWithoutTrailingNewline(t *testing.T) {
	codexDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte("model = \"old\"\nmodel_provider = \"oldp\"\n\n[model_providers.oldp]\nbase_url = \"http://old\"\n"), 0o600)
	store := NewStore(t.TempDir())
	profile, err := store.CreateRaw(RawProfileRequest{
		Name:       "team-proxy",
		AuthJSON:   `{"OPENAI_API_KEY":"sk-mem-test"}`,
		ConfigTOML: "model_provider = \"team-proxy\"\n\n[model_providers.team-proxy]\nname = \"team-proxy\"\nbase_url = \"https://memory.example/codex/default\"\nwire_api = \"responses\"",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Activate(codexDir, profile.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatalf("activation failed: %#v", result)
	}
	config, _ := os.ReadFile(filepath.Join(codexDir, "config.toml"))
	if !strings.Contains(string(config), "experimental_bearer_token = \"sk-mem-test\"") {
		t.Fatalf("bearer token missing: %s", config)
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

func TestPreviewNormalizesMemoryProvider(t *testing.T) {
	codexDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte("model_provider = \"newapi\"\n\n[model_providers.memory]\nname = \"memory\"\nbase_url = \"https://memory.example/v1\"\nwire_api = \"responses\"\nexperimental_bearer_token = \"sk-mem-test\"\n\n[model_providers.newapi]\nname = \"newapi\"\nbase_url = \"https://newapi.example/v1\"\n"), 0o600)
	store := NewStore(t.TempDir())
	memory, err := store.CreateRaw(RawProfileRequest{
		Name:       "memory",
		AuthJSON:   `{"OPENAI_API_KEY":"sk-mem-test"}`,
		ConfigTOML: "model_provider = \"memory\"\n\n[model_providers.memory]\nname = \"memory\"\nbase_url = \"https://memory.example/v1\"\nwire_api = \"responses\"\n",
	})
	if err != nil {
		t.Fatal(err)
	}

	preview, err := store.Preview(codexDir, memory.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.ProviderConflict {
		t.Fatalf("normalized Provider should match local config: %#v", preview)
	}
}

func TestPreviewTreatsMissingOrDifferentMemoryTokenAsConflict(t *testing.T) {
	profileConfig := "model_provider = \"memory\"\n\n[model_providers.memory]\nname = \"memory\"\nbase_url = \"https://memory.example/v1\"\nwire_api = \"responses\"\n"
	store := NewStore(t.TempDir())
	memory, err := store.CreateRaw(RawProfileRequest{Name: "memory", AuthJSON: `{"OPENAI_API_KEY":"sk-mem-test"}`, ConfigTOML: profileConfig})
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name     string
		provider string
	}{
		{name: "missing token", provider: profileConfig},
		{name: "different token", provider: profileConfig + "experimental_bearer_token = \"sk-mem-other\"\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			codexDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(test.provider), 0o600); err != nil {
				t.Fatal(err)
			}
			preview, err := store.Preview(codexDir, memory.ID)
			if err != nil || !preview.ProviderConflict {
				t.Fatalf("preview=%#v err=%v", preview, err)
			}
		})
	}
}

func TestPreviewAfterMemoryNewAPIMemoryRoundTrip(t *testing.T) {
	codexDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(codexDir, "auth.json"), []byte(`{"OPENAI_API_KEY":"sk-old"}`), 0o600)
	_ = os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte("model_provider = \"memory\"\n\n[model_providers.memory]\nname = \"memory\"\nbase_url = \"https://old.example/v1\"\nwire_api = \"responses\"\n"), 0o600)
	store := NewStore(t.TempDir())
	memory, err := store.CreateRaw(RawProfileRequest{
		Name:       "memory",
		AuthJSON:   `{"OPENAI_API_KEY":"sk-mem-test"}`,
		ConfigTOML: "model_provider = \"memory\"\n\n[model_providers.memory]\nname = \"memory\"\nbase_url = \"https://memory.example/v1\"\nwire_api = \"responses\"\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	newAPI, err := store.CreateAPIKey(APIKeyRequest{Name: "newapi", Provider: "newapi", BaseURL: "https://newapi.example/v1", APIKey: "sk-newapi"})
	if err != nil {
		t.Fatal(err)
	}

	first, err := store.Preview(codexDir, memory.ID)
	if err != nil || !first.ProviderConflict {
		t.Fatalf("first preview=%#v err=%v", first, err)
	}
	if _, err := store.Activate(codexDir, memory.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Activate(codexDir, newAPI.ID, true); err != nil {
		t.Fatal(err)
	}
	second, err := store.Preview(codexDir, memory.ID)
	if err != nil || second.ProviderConflict {
		t.Fatalf("second preview=%#v err=%v", second, err)
	}
}

func TestActivateRemoteNormalizesMemoryProviderBeforeComparison(t *testing.T) {
	t.Setenv("AGENTPAL_CONFIG_DIR", t.TempDir())
	codexDir := t.TempDir()
	auth := []byte(`{"OPENAI_API_KEY":"sk-mem-test"}`)
	fragment := []byte("model_provider = \"memory\"\n\n[model_providers.memory]\nname = \"memory\"\nbase_url = \"https://memory.example/v1\"\nwire_api = \"responses\"\n")
	local := "model_provider = \"newapi\"\n\n[model_providers.memory]\nname = \"memory\"\nbase_url = \"https://memory.example/v1\"\nwire_api = \"responses\"\nexperimental_bearer_token = \"sk-mem-test\"\n\n[model_providers.newapi]\nname = \"newapi\"\nbase_url = \"https://newapi.example/v1\"\n"
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(local), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := ActivateRemote(codexDir, AuthProfile{Name: "memory", Type: "api_key", Provider: "memory", Source: "central_login"}, auth, fragment, false)
	if err != nil || !result.OK || result.ProviderConflict {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}
