package authprofile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestChatGPTLoginIsolatedAndPreservesAPIKey(t *testing.T) {
	dir := t.TempDir()
	original := []byte(`{"OPENAI_API_KEY":"sk-original"}`)
	os.WriteFile(filepath.Join(dir, "auth.json"), original, 0600)
	os.WriteFile(filepath.Join(dir, "config.toml"), []byte("model_provider = \"gateway\"\n[model_providers.gateway]\nname = \"gateway\"\nbase_url = \"https://example.com\"\n[mcp_servers.demo]\ncommand = \"demo\"\n"), 0600)
	store := NewStore(t.TempDir())
	models := []byte(`{"models":[{"slug":"api-only"}]}`)
	os.WriteFile(filepath.Join(dir, "models.json"), models, 0600)
	profile, err := store.LoginChatGPT(context.Background(), dir, func(ctx context.Context, home string) error {
		if home == dir {
			t.Fatal("login uses active directory")
		}
		current, _ := os.ReadFile(filepath.Join(dir, "auth.json"))
		if string(current) != string(original) {
			t.Fatal("active auth changed before login")
		}
		return os.WriteFile(filepath.Join(home, "auth.json"), []byte(`{"auth_mode":"chatgpt","tokens":{"id_token":"header.eyJzdWIiOiJ1c2VyLTEyMyJ9.sig","access_token":"access","refresh_token":"refresh","account_id":"user-123"}}`), 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Type != "chatgpt" || profile.Provider != "openai" {
		t.Fatalf("profile: %#v", profile)
	}
	if _, err := os.Stat(filepath.Join(dir, "models.json")); !os.IsNotExist(err) {
		t.Fatal("ChatGPT inherited API models")
	}
	profiles, status, err := store.ReconcileCurrent(dir)
	if err != nil || len(profiles) != 2 || status.CurrentProfileID != profile.ID {
		t.Fatalf("profiles: %#v, status: %#v, error: %v", profiles, status, err)
	}
	for _, p := range profiles {
		if p.Type == "api_key" {
			if _, err := store.Activate(dir, p.ID, true); err != nil {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(filepath.Join(dir, "auth.json"))
			if string(data) != string(original) {
				t.Fatalf("API Key lost: %s", data)
			}
			catalog, err := os.ReadFile(filepath.Join(dir, "models.json"))
			if err != nil || string(catalog) != string(models) {
				t.Fatalf("API models lost: %s, %v", catalog, err)
			}
		}
	}
}

func TestChatGPTLoginFailureLeavesCurrentUntouched(t *testing.T) {
	for _, kind := range []string{"failure", "cancel", "invalid"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			original := []byte(`{"OPENAI_API_KEY":"sk-original"}`)
			os.WriteFile(filepath.Join(dir, "auth.json"), original, 0600)
			var temporary string
			_, err := NewStore(t.TempDir()).LoginChatGPT(context.Background(), dir, func(ctx context.Context, home string) error {
				temporary = home
				if kind == "cancel" {
					return context.Canceled
				}
				if kind == "failure" {
					return errors.New("failed")
				}
				return os.WriteFile(filepath.Join(home, "auth.json"), []byte(`{"OPENAI_API_KEY":"wrong"}`), 0600)
			})
			if err == nil {
				t.Fatal("expected failure")
			}
			data, _ := os.ReadFile(filepath.Join(dir, "auth.json"))
			if string(data) != string(original) {
				t.Fatal("auth changed")
			}
			if _, err := os.Stat(temporary); !os.IsNotExist(err) {
				t.Fatal("temporary credentials not cleaned")
			}
		})
	}
}

func TestChatGPTLoginRollsBackWhenConfigWriteFails(t *testing.T) {
	dir := t.TempDir()
	original := []byte(`{"OPENAI_API_KEY":"sk-original"}`)
	config := []byte("model_provider = \"gateway\"\n[model_providers.gateway]\nname = \"gateway\"\n")
	os.WriteFile(filepath.Join(dir, "auth.json"), original, 0600)
	os.WriteFile(filepath.Join(dir, "config.toml"), config, 0600)
	// Force failure after auth.json has been replaced.
	os.Mkdir(filepath.Join(dir, "config.toml.agentpal.tmp"), 0700)
	_, err := NewStore(t.TempDir()).LoginChatGPT(context.Background(), dir, func(ctx context.Context, home string) error {
		return os.WriteFile(filepath.Join(home, "auth.json"), []byte(`{"tokens":{"access_token":"a","refresh_token":"r","id_token":"header.eyJzdWIiOiJ1c2VyLTEyMyJ9.sig"}}`), 0600)
	})
	if err == nil {
		t.Fatal("expected config write error")
	}
	for name, expected := range map[string][]byte{"auth.json": original, "config.toml": config} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(data) != string(expected) {
			t.Fatalf("%s was not restored: %s, %v", name, data, err)
		}
	}
}
