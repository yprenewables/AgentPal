package types

import (
	"encoding/json"
	"testing"
	"time"
)

// TestCenterManifestJSONContract 固定 HTTP 契约的 JSON 形状。
// agentpal-central 仓库中有一份完全相同的断言，任何一侧字段名/结构变动都会导致该测试失败。
func TestCenterManifestJSONContract(t *testing.T) {
	manifest := CenterManifest{
		Schema:      1,
		App:         "AgentPal",
		Version:     "0.2.0",
		GeneratedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Skills:      []CenterSkill{},
		MCP:         []CenterMCP{},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	const golden = `{"schema":1,"app":"AgentPal","version":"0.2.0","generatedAt":"2026-01-02T03:04:05Z","skills":[],"mcp":[]}`
	if string(data) != golden {
		t.Fatalf("contract drift:\n got %s\nwant %s", data, golden)
	}
}
