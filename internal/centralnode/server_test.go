package centralnode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"agentpal/internal/centraltest"
	"agentpal/internal/types"
)

func TestServerManifestIsLiveAndFilesAreRestricted(t *testing.T) {
	dir := centraltest.MakeCodexDir(t)
	server := NewServer(types.ServerOptions{CodexDir: dir, Port: 28888})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	first := fetchManifest(t, httpServer.URL)
	if err := os.WriteFile(filepath.Join(dir, "skills", "translate", "SKILL.md"), []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	second := fetchManifest(t, httpServer.URL)
	if first.Skills[0].SHA256 == second.Skills[0].SHA256 {
		t.Fatal("manifest did not reflect source change")
	}

	resp, err := http.Get(httpServer.URL + "/v1/config/files/skills/translate/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("skill status = %d", resp.StatusCode)
	}
	resp, err = http.Get(httpServer.URL + "/v1/config/files/skills/missing/file")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing status = %d", resp.StatusCode)
	}
}

func fetchManifest(t *testing.T, base string) types.CenterManifest {
	t.Helper()
	resp, err := http.Get(base + "/v1/config/manifest")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var manifest types.CenterManifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}
