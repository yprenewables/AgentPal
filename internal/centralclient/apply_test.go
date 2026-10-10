package centralclient

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"agentpal/internal/centralnode"
	"agentpal/internal/centraltest"
	"agentpal/internal/types"
)

func TestApplyConfigurationDownloadsAndBacksUpSelectedChanges(t *testing.T) {
	source := centraltest.MakeCodexDir(t)
	target := centraltest.MakeCodexDir(t)
	if err := os.WriteFile(filepath.Join(target, "skills", "translate", "SKILL.md"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := centralnode.BuildManifest(source, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	server := centralnode.NewServer(types.ServerOptions{CodexDir: source, Port: 28888})
	ts := httptest.NewServer(server.Handler())
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
