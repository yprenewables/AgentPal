package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agentpal/internal/modelmanagement"
)

func TestModelManagementUsesSettingsDirectoryAndRechecksAuth(t *testing.T) {
	app, state, _ := setupCurrentAuthApp(t, `{"OPENAI_API_KEY":"secret"}`, "model_provider='corp'\n[model_providers.corp]\nbase_url='https://example.com/v1'")
	initial, err := app.GetModelManagementState()
	if err != nil || !initial.Available || initial.FileExists {
		t.Fatalf("state=%+v err=%v", initial, err)
	}
	saved, err := app.SaveModels(modelmanagement.SaveRequest{FileHash: initial.FileHash, RawModels: []string{`{"slug":"test","custom":9007199254740993,"visibility":"list"}`}, VisibilityOverrides: map[int]string{0: "hide"}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(state.CodexDir, "models.json"))
	if err != nil || !strings.Contains(string(data), "9007199254740993") || !strings.Contains(string(data), `"hide"`) {
		t.Fatalf("data=%s err=%v", data, err)
	}
	if len(saved.RawModels) != 1 {
		t.Fatalf("saved=%+v", saved)
	}
	os.WriteFile(filepath.Join(state.CodexDir, "auth.json"), []byte(`{"auth_mode":"chatgpt"}`), 0600)
	blocked, err := app.GetModelManagementState()
	if err != nil || blocked.Available || len(blocked.RawModels) != 0 {
		t.Fatalf("blocked=%+v err=%v", blocked, err)
	}
	if _, err = app.SaveModels(modelmanagement.SaveRequest{FileHash: saved.FileHash, RawModels: []string{}}); err == nil {
		t.Fatal("save did not recheck auth")
	}
}
