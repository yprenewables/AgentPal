package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadMCPNormalizesEquivalentTables(t *testing.T) {
	a := writeConfig(t, "[mcp_servers.docs]\ncommand = \"npx\"\nargs = [\"server\"]\n")
	b := writeConfig(t, "[mcp_servers.docs]\nargs=[\"server\"]\ncommand=\"npx\"\n")
	left, err := ReadMCP(a)
	if err != nil {
		t.Fatal(err)
	}
	right, err := ReadMCP(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || left[0].SHA256 != right[0].SHA256 {
		t.Fatalf("not normalized: %#v %#v", left, right)
	}
}

func TestUpsertMCPPreservesUnrelatedConfig(t *testing.T) {
	path := writeConfig(t, "# keep\nmodel = \"old\"\n\n[mcp_servers.old]\ncommand = \"old\"\n\n[projects.demo]\ntrust_level = \"trusted\"\n")
	err := UpsertMCP(path, []MCPEntry{{Name: "docs", TOML: []byte("[mcp_servers.docs]\ncommand = \"new\"\n")}}, []string{"old"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	got := string(data)
	if !containsAll(got, "# keep", "model = \"old\"", "[projects.demo]", "[mcp_servers.docs]") || containsAll(got, "[mcp_servers.old]") {
		t.Fatalf("unexpected config:\n%s", got)
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
