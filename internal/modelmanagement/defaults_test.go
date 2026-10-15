package modelmanagement

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultsPreserveConfigAndRejectConflicts(t *testing.T) {
	original := "# keep header\n\"model\" = '''old''' # model comment\nmodel_reasoning_effort = 'low' # effort comment\nnotes = '''\nmodel = \"not a setting\"\n[not_a_table]\n'''\n" + config + "\n[mcp_servers.demo]\ncommand = 'keep'\n[profiles.demo]\nmodel = 'profile-model'\n"
	s := fixture(t, keyAuth, original, "")
	state := s.Defaults()
	if !state.Available || state.Model != "old" || state.Effort != "low" {
		t.Fatalf("state=%+v", state)
	}
	saved, err := s.SaveDefaults(DefaultRequest{ConfigHash: state.ConfigHash, Model: "new-model", Effort: "xhigh"})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Model != "new-model" || saved.Effort != "xhigh" {
		t.Fatalf("saved=%+v", saved)
	}
	data, _ := os.ReadFile(filepath.Join(s.Dir, "config.toml"))
	for _, text := range []string{"# keep header", "# model comment", "# effort comment", "model = \"not a setting\"", "[not_a_table]", "command = 'keep'", "model = 'profile-model'", config} {
		if !strings.Contains(string(data), text) {
			t.Fatalf("lost %s: %s", text, data)
		}
	}
	if _, err = s.SaveDefaults(DefaultRequest{ConfigHash: state.ConfigHash, Model: "overwrite"}); err == nil {
		t.Fatal("overwrote conflict")
	}
	info, _ := os.Stat(filepath.Join(s.Dir, "config.toml"))
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
}

func TestDefaultsInsertClearAndValidation(t *testing.T) {
	s := fixture(t, keyAuth, config, "")
	state := s.Defaults()
	saved, err := s.SaveDefaults(DefaultRequest{ConfigHash: state.ConfigHash, Model: "custom/model", Effort: "high"})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Model != "custom/model" || saved.Effort != "high" {
		t.Fatalf("saved=%+v", saved)
	}
	cleared, err := s.SaveDefaults(DefaultRequest{ConfigHash: saved.ConfigHash})
	if err != nil || cleared.Model != "" || cleared.Effort != "" {
		t.Fatalf("cleared=%+v err=%v", cleared, err)
	}
	if _, err = s.SaveDefaults(DefaultRequest{ConfigHash: cleared.ConfigHash, Effort: "bogus"}); err == nil {
		t.Fatal("invalid effort accepted")
	}
	os.WriteFile(filepath.Join(s.Dir, "auth.json"), []byte(`{"auth_mode":"chatgpt"}`), 0600)
	if s.Defaults().Available {
		t.Fatal("ChatGPT enabled defaults")
	}
	if _, err = s.SaveDefaults(DefaultRequest{ConfigHash: cleared.ConfigHash, Model: "bad"}); err == nil {
		t.Fatal("ChatGPT save accepted")
	}
	s = fixture(t, keyAuth, "model = [", "")
	if s.Defaults().Available {
		t.Fatal("invalid config accepted")
	}
}
