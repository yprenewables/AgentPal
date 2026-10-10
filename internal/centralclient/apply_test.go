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

func TestApplyConfigurationDownloadsAndBacksUpSelectedChanges(t *testing.T) {
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
	if !result.OK || result.BackupPath == "" {
		t.Fatalf("result: %#v", result)
	}
	data, _ := os.ReadFile(filepath.Join(target, "skills", "translate", "SKILL.md"))
	if string(data) != "one" {
		t.Fatalf("skill = %q", data)
	}
}
