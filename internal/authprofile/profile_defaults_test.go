package authprofile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileDefaultsRoundTripWithoutModelsFile(t *testing.T) {
	s := NewStore(t.TempDir())
	dir := t.TempDir()
	a, _ := s.CreateAPIKey(APIKeyRequest{Name: "A", Provider: "a", BaseURL: "https://a.test", APIKey: "a"})
	b, _ := s.CreateAPIKey(APIKeyRequest{Name: "B", Provider: "b", BaseURL: "https://b.test", APIKey: "b"})
	s.Activate(dir, a.ID, true)
	configPath := filepath.Join(dir, "config.toml")
	config, _ := os.ReadFile(configPath)
	os.WriteFile(configPath, append([]byte("model = \"model-a\"\nmodel_reasoning_effort = \"high\"\n"), config...), 0600)
	if _, err := s.Activate(dir, b.ID, true); err != nil {
		t.Fatal(err)
	}
	config, _ = os.ReadFile(configPath)
	if strings.Contains(string(config), "model-a") || strings.Contains(string(config), "model_reasoning_effort") {
		t.Fatalf("defaults inherited: %s", config)
	}
	os.WriteFile(configPath, append([]byte("model = \"model-b\"\nmodel_reasoning_effort = \"low\"\n"), config...), 0600)
	for _, tc := range []struct{ id, model, effort string }{{a.ID, "model-a", "high"}, {b.ID, "model-b", "low"}} {
		if _, err := s.Activate(dir, tc.id, true); err != nil {
			t.Fatal(err)
		}
		config, _ = os.ReadFile(configPath)
		if !strings.Contains(string(config), tc.model) || !strings.Contains(string(config), "\""+tc.effort+"\"") {
			t.Fatalf("defaults not restored: %s", config)
		}
	}
}
