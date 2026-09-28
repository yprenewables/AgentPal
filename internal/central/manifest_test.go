package central

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func makeCodexDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "skills", "translate"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills", "translate", "SKILL.md"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("[mcp_servers.docs]\ncommand = \"npx\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestBuildManifestIncludesSkillsAndMCP(t *testing.T) {
	dir := makeCodexDir(t)
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	manifest, err := BuildManifest(dir, now)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Schema != 1 || !manifest.GeneratedAt.Equal(now) {
		t.Fatalf("manifest: %#v", manifest)
	}
	if len(manifest.Skills) != 1 || manifest.Skills[0].Name != "translate" || len(manifest.Skills[0].Files) != 1 {
		t.Fatalf("skills: %#v", manifest.Skills)
	}
	if len(manifest.MCP) != 1 || manifest.MCP[0].Name != "docs" {
		t.Fatalf("mcp: %#v", manifest.MCP)
	}
}
