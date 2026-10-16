package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinOpenAIAuthRoundTrip(t *testing.T) {
	fragment, summary, err := ExtractAuthConfigFromBytes(nil)
	if err != nil || summary.Provider != "openai" {
		t.Fatalf("extract builtin: %s, %#v, %v", fragment, summary, err)
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	original := "model_provider = \"gateway\"\n[mcp_servers.demo]\ncommand = \"demo\"\n[model_providers.openai]\nbase_url = \"https://gateway.invalid\"\n[model_providers.gateway]\nname = \"gateway\"\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := CompareProviderConfig(path, fragment); err != nil {
		t.Fatal(err)
	}
	if err := MergeAuthConfig(path, fragment); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "gateway.invalid") || !strings.Contains(string(data), "mcp_servers.demo") || !strings.Contains(string(data), "model_providers.gateway") {
		t.Fatalf("bad merge: %s", data)
	}
	conflict, _, _, _, err := CompareProviderConfig(path, fragment)
	if err != nil || conflict {
		t.Fatalf("builtin conflict: %v %v", conflict, err)
	}
}
