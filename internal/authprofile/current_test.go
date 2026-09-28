package authprofile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectCurrentAuthFindsMaskedAPIKeyAndChatGPTUserID(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(`{"tokens":{"id_token":"header.eyJzdWIiOiJ1c2VyLTEyMyJ9.sig"},"OPENAI_API_KEY":"sk-test-secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("model = \"gpt-5\"\nmodel_provider = \"openai\"\n\n[model_providers.openai]\nenv_key = \"OPENAI_API_KEY\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err := InspectCurrent(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if status.APIKeyPreview != "sk-••••••cret" {
		t.Fatalf("API key preview = %q", status.APIKeyPreview)
	}
	if status.ChatGPTUserID != "user-123" {
		t.Fatalf("user id = %q", status.ChatGPTUserID)
	}
}

func TestInspectCurrentAuthFindsChatGPTExpiry(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(`{"tokens":{"id_token":"header.eyJzdWIiOiJ1c2VyLTEyMyIsImV4cCI6MjAwMDAwMDAwMH0.sig"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err := InspectCurrent(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if status.ChatGPTExpiresAt != "2033-05-18T03:33:20Z" {
		t.Fatalf("expiry = %q", status.ChatGPTExpiresAt)
	}
}

func TestInspectCurrentAuthMatchesLegacyAPIKeyProfileWithoutBaseURL(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(`{"OPENAI_API_KEY":"sk-test-secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("model_provider = \"openai\"\n\n[model_providers.openai]\nbase_url = \"https://api.openai.com/v1\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	profiles := []AuthProfile{{ID: "legacy-api-profile", Type: "api_key", Provider: "openai", APIKeyPreview: "sk-••••••cret"}}
	status, err := InspectCurrent(dir, profiles)
	if err != nil {
		t.Fatal(err)
	}
	if status.CurrentProfileID != "legacy-api-profile" || !status.APIKeyImported {
		t.Fatalf("status: %#v", status)
	}
}
