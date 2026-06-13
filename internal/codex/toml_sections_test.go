package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectConfigSections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	data := []byte("model = \"gpt\"\napproval_policy = \"on-request\"\n\n[profiles.default]\nmodel = \"x\"\n\n[mcp_servers.foo]\ncommand = \"foo\"\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	sections, err := InspectConfigSections(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(sections.RootKeys, ",") != "approval_policy,model" {
		t.Fatalf("root keys = %v", sections.RootKeys)
	}
	if strings.Join(sections.Tables, ",") != "mcp_servers.foo,profiles.default" {
		t.Fatalf("tables = %v", sections.Tables)
	}
}

func TestMergeConfigSections(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.toml")
	target := filepath.Join(dir, "target.toml")
	if err := os.WriteFile(source, []byte("model = \"new\"\napproval_policy = \"never\"\n\n[profiles.default]\nmodel = \"new-profile\"\n\n[mcp_servers.new]\ncommand = \"new\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("model = \"old\"\nsandbox_mode = \"workspace-write\"\n\n[profiles.default]\nmodel = \"old-profile\"\n\n[local.only]\nvalue = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := MergeConfigSections(source, target, []string{"model"}, []string{"profiles.default", "mcp_servers.new"}); err != nil {
		t.Fatal(err)
	}
	mergedBytes, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	merged := string(mergedBytes)
	for _, want := range []string{"model = \"new\"", "sandbox_mode = \"workspace-write\"", "[profiles.default]", "model = \"new-profile\"", "[local.only]", "[mcp_servers.new]"} {
		if !strings.Contains(merged, want) {
			t.Fatalf("merged config missing %q:\n%s", want, merged)
		}
	}
	if strings.Contains(merged, "model = \"old\"") || strings.Contains(merged, "old-profile") || strings.Contains(merged, "approval_policy") {
		t.Fatalf("merged config retained replaced or unselected values:\n%s", merged)
	}
}
