package centralclient

import (
	"os"
	"path/filepath"
	"testing"

	"agentpal/internal/centralscan"
	"agentpal/internal/centraltest"
	"agentpal/internal/codex"
	"agentpal/internal/types"
)

func TestCompareReportsRemoteAndLocalStates(t *testing.T) {
	dir := centraltest.MakeCodexDir(t)
	manifest := buildTestManifest(t, dir)
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

// buildTestManifest 用客户端侧的扫描逻辑构造中心清单夹具，不依赖服务端代码。
func buildTestManifest(t *testing.T, dir string) types.CenterManifest {
	t.Helper()
	skills, _ := centralscan.ScanSkills(filepath.Join(dir, "skills"), nil)
	entries, err := codex.ReadMCP(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	mcp := make([]types.CenterMCP, 0, len(entries))
	for _, entry := range entries {
		mcp = append(mcp, types.CenterMCP{Name: entry.Name, SHA256: entry.SHA256, Size: int64(len(entry.TOML))})
	}
	return types.CenterManifest{Schema: 1, Skills: skills, MCP: mcp}
}
