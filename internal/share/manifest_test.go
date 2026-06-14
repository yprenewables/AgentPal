package share

import (
	"os"
	"path/filepath"
	"testing"

	"agentpal/internal/types"
)

func TestBuildManifest(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("config"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "skills", "translation"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills", "translation", "SKILL.md"), []byte("skill"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "skills", ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills", ".git", "ignored"), []byte("ignore"), 0o600); err != nil {
		t.Fatal(err)
	}

	manifest, _, err := BuildManifest(types.ShareRequest{CodexDir: dir, ShareConfig: true, ShareSkills: true, Skills: []string{"translation"}})
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.Shared.Config.Enabled || manifest.Shared.Config.SHA256 == "" {
		t.Fatalf("expected config to be enabled with hash: %+v", manifest.Shared.Config)
	}
	if manifest.Shared.Auth.Enabled || !manifest.Shared.Auth.Sensitive {
		t.Fatalf("expected auth disabled but sensitive: %+v", manifest.Shared.Auth)
	}
	if manifest.Shared.Skills.Count != 1 || manifest.Shared.Skills.Files[0].Path != "translation/SKILL.md" {
		t.Fatalf("unexpected skills manifest: %+v", manifest.Shared.Skills)
	}
	if len(manifest.Shared.Skills.Skills) != 1 || manifest.Shared.Skills.Skills[0] != "translation" {
		t.Fatalf("unexpected skill list: %+v", manifest.Shared.Skills.Skills)
	}
}
