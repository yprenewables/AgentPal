package sync

import (
	"os"
	"path/filepath"
	"testing"

	"agentpal/internal/types"
)

func TestApplySkillsReplacesSelectedAndKeepsOthers(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source", "skills")
	target := filepath.Join(dir, "target", "skills")
	if err := os.MkdirAll(filepath.Join(source, "shared"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, "new"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(target, "shared"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(target, "local"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "shared", "SKILL.md"), []byte("remote"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "new", "SKILL.md"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "shared", "local.txt"), []byte("remove"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "local", "SKILL.md"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := applySkills(types.SyncRequest{Skills: []string{"shared", "new"}}, source, target); err != nil {
		t.Fatal(err)
	}
	shared, err := os.ReadFile(filepath.Join(target, "shared", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(shared) != "remote" {
		t.Fatalf("shared skill = %q", shared)
	}
	if _, err := os.Stat(filepath.Join(target, "shared", "local.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected old same-name skill contents to be removed, err=%v", err)
	}
	local, err := os.ReadFile(filepath.Join(target, "local", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(local) != "keep" {
		t.Fatalf("local skill = %q", local)
	}
	newSkill, err := os.ReadFile(filepath.Join(target, "new", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(newSkill) != "new" {
		t.Fatalf("new skill = %q", newSkill)
	}
}
