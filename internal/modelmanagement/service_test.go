package modelmanagement

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pelletier/go-toml/v2"
)

func fixture(t *testing.T, auth, config, models string) *Service {
	t.Helper()
	dir := t.TempDir()
	for name, value := range map[string]string{"auth.json": auth, "config.toml": config, "models.json": models} {
		if value != "" {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	return New(dir)
}

const config = "model_provider = 'corp'\n[model_providers.corp]\nname = 'Corporate'\nbase_url = 'https://example.com/v1/'\n"
const keyAuth = `{"OPENAI_API_KEY":"secret-key"}`

func TestSaveConnectsModelCatalog(t *testing.T) {
	for _, prefix := range []string{"", "model_catalog_json = '/old/models.json' # catalog comment\n"} {
		s := fixture(t, keyAuth, prefix+config+"\n[mcp_servers.demo]\ncommand = 'keep'\n", `{"models":[]}`)
		state := s.State()
		if _, err := s.Save(SaveRequest{FileHash: state.FileHash, Models: []json.RawMessage{Template("deepseek-flash")}}); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(s.Dir, "config.toml"))
		if err != nil {
			t.Fatal(err)
		}
		var cfg map[string]any
		if err := toml.Unmarshal(data, &cfg); err != nil {
			t.Fatal(err)
		}
		if cfg["model_catalog_json"] != filepath.Join(s.Dir, "models.json") {
			t.Fatalf("catalog=%v", cfg["model_catalog_json"])
		}
		if !strings.Contains(string(data), config) || !strings.Contains(string(data), "command = 'keep'") {
			t.Fatalf("lost config: %s", data)
		}
		if prefix != "" && !strings.Contains(string(data), "# catalog comment") {
			t.Fatal("lost comment")
		}
		before := string(data)
		state = s.State()
		if _, err := s.Save(SaveRequest{FileHash: state.FileHash, Models: state.Models}); err != nil {
			t.Fatal(err)
		}
		data, _ = os.ReadFile(filepath.Join(s.Dir, "config.toml"))
		if string(data) != before {
			t.Fatal("repeated save changed config")
		}
	}
}

func TestSaveInvalidConfigDoesNotChangeModels(t *testing.T) {
	s := fixture(t, keyAuth, "model = [", `{"models":[]}`)
	state := s.State()
	if _, err := s.Save(SaveRequest{FileHash: state.FileHash, Models: []json.RawMessage{Template("new")}}); err == nil {
		t.Fatal("saved with invalid config")
	}
	data, _ := os.ReadFile(filepath.Join(s.Dir, "models.json"))
	if string(data) != `{"models":[]}` {
		t.Fatal("changed models on config failure")
	}
}

func TestStateAuthenticationAndFiles(t *testing.T) {
	for _, tc := range []struct {
		name, auth, file string
		available, valid bool
	}{
		{"key", keyAuth, `{"models":[{"slug":"old","custom":9007199254740993}],"extra":true}`, true, true},
		{"missing", keyAuth, "", true, true},
		{"invalid", keyAuth, "{", true, false},
		{"array", keyAuth, "[]", true, false},
		{"null", keyAuth, "null", true, false},
		{"missing models", keyAuth, "{}", true, false},
		{"null models", keyAuth, `{"models":null}`, true, false},
		{"wrong models", keyAuth, `{"models":{}}`, true, false},
		{"chatgpt", `{"auth_mode":"chatgpt","tokens":{"access_token":"token"}}`, "{", false, false},
		{"unknown", "{}", "{", false, false},
		{"bad auth", "{", "{", false, false},
		{"unknown mode with key", `{"auth_mode":"unknown","OPENAI_API_KEY":"secret"}`, "{", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := fixture(t, tc.auth, config, tc.file)
			state := s.State()
			if state.Available != tc.available || state.FileValid != tc.valid {
				t.Fatalf("state=%+v", state)
			}
			if !tc.available && (len(state.Models) != 0 || state.FileHash != "") {
				t.Fatalf("leaked models: %+v", state)
			}
			if !tc.valid && state.Error == "" {
				t.Fatal("missing error")
			}
			if tc.available && state.Provider != "Corporate" {
				t.Fatalf("provider=%s", state.Provider)
			}
		})
	}
}

func TestSavePreservesFieldsAndChecksConflict(t *testing.T) {
	s := fixture(t, keyAuth, config, `{"extra":{"integer":9007199254740993},"models":[{"slug":"old","visibility":"list","custom":9007199254740993},{"slug":"delete"}]}`)
	state := s.State()
	var model map[string]json.RawMessage
	json.Unmarshal(state.Models[0], &model)
	model["visibility"] = json.RawMessage(`"hide"`)
	changed, _ := json.Marshal(model)
	saved, err := s.Save(SaveRequest{FileHash: state.FileHash, Models: []json.RawMessage{changed}})
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Models) != 1 || saved.FileHash == state.FileHash {
		t.Fatalf("saved=%+v", saved)
	}
	var migrated map[string]json.RawMessage
	json.Unmarshal(saved.Models[0], &migrated)
	for _, field := range []string{"supported_reasoning_levels", "support_verbosity", "truncation_policy", "experimental_supported_tools", "base_instructions"} {
		if _, ok := migrated[field]; !ok {
			t.Fatalf("saved legacy model missing %s", field)
		}
	}
	data, _ := os.ReadFile(filepath.Join(s.Dir, "models.json"))
	if !strings.Contains(string(data), "9007199254740993") || !strings.Contains(string(data), `"extra"`) || !strings.Contains(string(data), `"hide"`) {
		t.Fatalf("data=%s", data)
	}
	info, _ := os.Stat(filepath.Join(s.Dir, "models.json"))
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode=%v", info.Mode())
	}
	if _, err = s.Save(SaveRequest{FileHash: state.FileHash, Models: state.Models}); err == nil {
		t.Fatal("overwrote conflict")
	}
	files, _ := os.ReadDir(s.Dir)
	for _, f := range files {
		if strings.HasPrefix(f.Name(), ".models-") {
			t.Fatal("temp file leaked")
		}
	}
}

func TestSaveMissingInvalidAndAuthenticationRecheck(t *testing.T) {
	s := fixture(t, keyAuth, config, "")
	state := s.State()
	saved, err := s.Save(SaveRequest{FileHash: state.FileHash, Models: []json.RawMessage{Template("new")}})
	if err != nil || len(saved.Models) != 1 {
		t.Fatalf("saved=%+v err=%v", saved, err)
	}
	os.WriteFile(filepath.Join(s.Dir, "auth.json"), []byte(`{"auth_mode":"chatgpt"}`), 0600)
	if _, err = s.Save(SaveRequest{FileHash: saved.FileHash, Models: []json.RawMessage{}}); err == nil {
		t.Fatal("saved under ChatGPT")
	}
	s = fixture(t, keyAuth, config, "{")
	if _, err = s.Save(SaveRequest{Models: []json.RawMessage{}}); err == nil {
		t.Fatal("overwrote invalid JSON")
	}
	data, _ := os.ReadFile(filepath.Join(s.Dir, "models.json"))
	if string(data) != "{" {
		t.Fatal("changed invalid file")
	}
}

func TestDiscovery(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		fail       bool
	}{
		{"ok", `{"data":[{"id":"a"},{"id":"a"},{"id":"b"}]}`, 200, false},
		{"empty", `{"data":[]}`, 200, false},
		{"http", `secret response`, 401, true},
		{"json", "{", 200, true},
		{"missing", "{}", 200, true},
		{"null", `{"data":null}`, 200, true},
		{"bad id", `{"data":[{}]}`, 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer secret-key" {
					t.Errorf("request=%s auth=%s", r.URL.Path, r.Header.Get("Authorization"))
				}
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer server.Close()
			s := fixture(t, keyAuth, "model_provider='corp'\n[model_providers.corp]\nbase_url='"+server.URL+"/v1///'", "")
			if s.Client.Timeout != 10*time.Second {
				t.Fatal("incorrect timeout")
			}
			ids, err := s.Discover()
			if (err != nil) != tc.fail {
				t.Fatalf("ids=%v err=%v", ids, err)
			}
			if tc.name == "ok" && strings.Join(ids, ",") != "a,b" {
				t.Fatalf("ids=%v", ids)
			}
			if _, err := os.Stat(filepath.Join(s.Dir, "models.json")); !os.IsNotExist(err) {
				t.Fatal("discovery wrote file")
			}
		})
	}
}

func TestDiscoveryMissingParametersAndTimeout(t *testing.T) {
	for _, cfg := range []string{"", "model_provider='corp'", "model_provider='corp'\n[model_providers.corp]\nbase_url='ftp://example.com'"} {
		s := fixture(t, keyAuth, cfg, "")
		if _, err := s.Discover(); err == nil {
			t.Fatal("discovered without valid provider")
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(30 * time.Millisecond)
		w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()
	s := fixture(t, keyAuth, "model_provider='corp'\n[model_providers.corp]\nbase_url='"+server.URL+"'", "")
	s.Client.Timeout = time.Millisecond
	if _, err := s.Discover(); err == nil {
		t.Fatal("timeout not enforced")
	}
}

func TestMergeAndTemplate(t *testing.T) {
	old := json.RawMessage(`{"slug":"old","custom":true}`)
	got, added := Merge([]json.RawMessage{old}, []string{"old", "new", "new"})
	if added != 1 || len(got) != 2 || string(got[0]) != string(old) {
		t.Fatalf("got=%s added=%d", got, added)
	}
	var model map[string]any
	json.Unmarshal(got[1], &model)
	for _, field := range []string{"supported_reasoning_levels", "support_verbosity", "truncation_policy", "experimental_supported_tools", "base_instructions"} {
		if _, ok := model[field]; !ok {
			t.Fatalf("missing Codex required field %s", field)
		}
	}
	expected := `{"slug":"new","display_name":"new","visibility":"hide","supported_in_api":true,"context_window":128000,"input_modalities":["text"]}`
	var want map[string]any
	json.Unmarshal([]byte(expected), &want)
	for key, value := range want {
		a, _ := json.Marshal(model[key])
		b, _ := json.Marshal(value)
		if string(a) != string(b) {
			t.Fatalf("template field %s=%s, want %s", key, a, b)
		}
	}
	retained, n := Merge(got, []string{})
	if len(retained) != 2 || n != 0 {
		t.Fatal("removed unavailable models")
	}
}
