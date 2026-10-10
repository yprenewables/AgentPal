package centralclient

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"agentpal/internal/centralnode"
	"agentpal/internal/centraltest"
	"agentpal/internal/types"
)

func TestCompareReportsRemoteAndLocalStates(t *testing.T) {
	dir := centraltest.MakeCodexDir(t)
	manifest, err := centralnode.BuildManifest(dir, nowUTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "skills", "local"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills", "local", "SKILL.md"), []byte("local"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills", "translate", "SKILL.md"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	diff, err := Compare(dir, manifest)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]string{}
	for _, item := range diff.Items {
		states[item.Kind+":"+item.Name] = item.Status
	}
	if states["skill:translate"] != types.DiffChanged || states["skill:local"] != types.DiffLocalOnly {
		t.Fatalf("diff: %#v", diff.Items)
	}
}

func nowUTC() time.Time { return time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC) }
