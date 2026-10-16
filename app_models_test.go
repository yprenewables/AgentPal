package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agentpal/internal/modelcatalog"
	"agentpal/internal/modelmanagement"
)

type staticCatalog struct{ catalog *modelcatalog.Catalog }

func (s staticCatalog) Catalog(context.Context) (*modelcatalog.Catalog, error) {
	return s.catalog, nil
}

func TestSaveModelsPassesUpdateParamsThroughApp(t *testing.T) {
	entry, ok := modelcatalog.BuildInfo("openai/gpt-6-sol", "", 1050000, []string{"text"}, []string{"high"}, "high")
	if !ok {
		t.Fatal("fixture entry rejected")
	}
	original := modelmanagement.NewCatalogSource
	modelmanagement.NewCatalogSource = func() modelmanagement.CatalogSource {
		return staticCatalog{catalog: modelcatalog.NewCatalog([]modelcatalog.Entry{entry})}
	}
	defer func() { modelmanagement.NewCatalogSource = original }()

	app, state, _ := setupCurrentAuthApp(t, `{"OPENAI_API_KEY":"secret"}`, "model_provider='corp'\n[model_providers.corp]\nbase_url='https://example.com/v1'")
	initial, err := app.GetModelManagementState()
	if err != nil {
		t.Fatal(err)
	}
	saved, err := app.SaveModels(modelmanagement.SaveRequest{FileHash: initial.FileHash, RawModels: []string{`{"slug":"gpt-6-sol"}`}, UpdateParams: true})
	if err != nil || len(saved.UnmatchedModels) != 0 {
		t.Fatalf("saved=%+v err=%v", saved, err)
	}
	data, err := os.ReadFile(filepath.Join(state.CodexDir, "models.json"))
	if err != nil || !strings.Contains(string(data), "272000") {
		t.Fatalf("data=%s err=%v", data, err)
	}
	profiles, err := app.ListAuthProfiles()
	if err != nil {
		t.Fatal(err)
	}
	for _, profile := range profiles {
		if profile.Active {
			savedData, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".agentpal", "auth-profiles", profile.ID, "models.json"))
			if err != nil || string(savedData) != string(data) {
				t.Fatalf("profile models not saved: %s, %v", savedData, err)
			}
			return
		}
	}
	t.Fatal("no current profile")
}

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
