package codex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("config"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte("auth"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "skills", "translation"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills", "translation", "SKILL.md"), []byte("skill"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills", ".DS_Store"), []byte("ignore"), 0o600); err != nil {
		t.Fatal(err)
	}

	inspection, err := InspectDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !inspection.Config.Exists || !inspection.Auth.Exists || !inspection.Skills.Exists {
		t.Fatalf("expected all resources to exist: %+v", inspection)
	}
	if !inspection.Auth.Sensitive {
		t.Fatal("auth status should be sensitive")
	}
	if inspection.Skills.Count != 1 {
		t.Fatalf("skills count = %d, want 1", inspection.Skills.Count)
	}
}
