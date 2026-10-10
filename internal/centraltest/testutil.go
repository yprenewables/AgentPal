package centraltest

import (
	"os"
	"path/filepath"
	"testing"
)

// MakeCodexDir 构造一个最小的 Codex 目录，供中心服务端与客户端测试共用。
func MakeCodexDir(t *testing.T) string {
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
