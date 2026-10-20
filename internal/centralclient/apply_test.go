package centralclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"agentpal/internal/centraltest"
	"agentpal/internal/types"
)

func TestApplyConfigurationDownloadsSelectedChanges(t *testing.T) {
	source := centraltest.MakeCodexDir(t)
	target := centraltest.MakeCodexDir(t)
	if err := os.WriteFile(filepath.Join(target, "skills", "translate", "SKILL.md"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := buildTestManifest(t, source)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/config/files/skills/translate/SKILL.md" {
			_, _ = w.Write([]byte("one"))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()
	client := NewClient(ts.Client(), 2*time.Second)
	result, err := Apply(context.Background(), client, ts.URL, target, manifest, []types.ApplyChange{{Kind: "skill", Name: "translate", Action: "apply"}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatalf("result: %#v", result)
	}
	data, _ := os.ReadFile(filepath.Join(target, "skills", "translate", "SKILL.md"))
	if string(data) != "one" {
		t.Fatalf("skill = %q", data)
	}
}

func TestApplyConfigurationRejectsUnsupportedChanges(t *testing.T) {
	for _, change := range []types.ApplyChange{
		{Kind: "mcp", Name: "remove", Action: "delete"},
		{Kind: "mcp", Name: "remove", Action: "apply"},
		{Kind: "skill", Name: "translate", Action: "delete"},
		{Kind: "skill", Name: "translate", Action: "upload"},
		{Kind: "skill", Name: "translate", Action: "apply"},
	} {
		t.Run(change.Kind+"_"+change.Action, func(t *testing.T) {
			target := centraltest.MakeCodexDir(t)
			configPath := filepath.Join(target, "config.toml")
			if err := os.WriteFile(configPath, []byte("[mcp_servers.remove]\ncommand = \"remove\"\n\n[mcp_servers.keep]\ncommand = \"keep\"\n"), 0o600); err != nil {
				t.Fatal(err)
			}

			result, err := Apply(context.Background(), NewClient(nil, 2*time.Second), "http://unused", target, types.CenterManifest{}, []types.ApplyChange{change})
			if err == nil || result.OK {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			config, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(config) != "[mcp_servers.remove]\ncommand = \"remove\"\n\n[mcp_servers.keep]\ncommand = \"keep\"\n" {
				t.Fatalf("config=%s", config)
			}
			skill, err := os.ReadFile(filepath.Join(target, "skills", "translate", "SKILL.md"))
			if err != nil || string(skill) != "one" {
				t.Fatalf("local skill changed: %q, %v", skill, err)
			}
		})
	}
}
