package security

import "testing"

func TestValidateSkillRelPath(t *testing.T) {
	valid := []string{"translation/SKILL.md", "a/b/c.txt"}
	for _, path := range valid {
		if err := ValidateSkillRelPath(path); err != nil {
			t.Fatalf("ValidateSkillRelPath(%q) returned error: %v", path, err)
		}
	}
}

func TestValidateSkillRelPathRejectsUnsafePaths(t *testing.T) {
	unsafe := []string{"", "../x", "a/../x", "/tmp/x", `C:\x`, `a\..\x`, "a//b", "./x"}
	for _, path := range unsafe {
		if err := ValidateSkillRelPath(path); err == nil {
			t.Fatalf("ValidateSkillRelPath(%q) expected an error", path)
		}
	}
}

func TestSafeJoinRejectsEscape(t *testing.T) {
	if _, err := SafeJoin(t.TempDir(), "..", "x"); err == nil {
		t.Fatal("SafeJoin expected an error for escaping path")
	}
}
