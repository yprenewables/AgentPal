package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func useTempStateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	stateDirOverride = dir
	t.Cleanup(func() { stateDirOverride = "" })
	return dir
}

func TestReadStateReturnsClientDefaults(t *testing.T) {
	useTempStateDir(t)

	got, err := ReadState()
	if err != nil {
		t.Fatal(err)
	}
	if got.CodexDir != "~/.codex" || got.CentralHost != "127.0.0.1" || got.CentralPort != 28888 {
		t.Fatalf("unexpected defaults: %#v", got)
	}
	if got.DeviceName == "" {
		t.Fatal("default device name is empty")
	}
}

func TestReadStateNormalizesMissingNewFieldsAndIgnoresLegacyPeer(t *testing.T) {
	dir := useTempStateDir(t)
	data := []byte(`{"codex_dir":"/tmp/codex","last_peer_ip":"192.0.2.9","central_host":"center.local","device_name":"mac"}`)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ReadState()
	if err != nil {
		t.Fatal(err)
	}
	if got.CentralPort != 28888 {
		t.Fatalf("missing defaults were not normalized: %#v", got)
	}
	if got.CentralHost != "center.local" {
		t.Fatalf("unexpected central host: %q", got.CentralHost)
	}
}

func TestWriteStateRoundTripsAndUsesPrivateMode(t *testing.T) {
	dir := useTempStateDir(t)
	want := State{
		CodexDir:    "/tmp/codex",
		CentralHost: "center.local",
		CentralPort: 30000,
		DeviceName:  "mac",
	}
	if err := WriteState(want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %o, want 600", info.Mode().Perm())
	}
	got, err := ReadState()
	if err != nil {
		t.Fatal(err)
	}
	if got.CodexDir != want.CodexDir || got.CentralHost != want.CentralHost || got.CentralPort != want.CentralPort || got.DeviceName != want.DeviceName {
		t.Fatalf("round trip mismatch: got %#v want %#v", got, want)
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if _, exists := raw["last_peer_ip"]; exists {
		t.Fatal("legacy last_peer_ip was written")
	}
}

func TestValidateRequiresCenterHost(t *testing.T) {
	err := Validate(State{CodexDir: "~/.codex", CentralPort: 28888, DeviceName: "mac"})
	if err == nil || err.Error() != "central_host is required" {
		t.Fatalf("got %v", err)
	}
}

func TestValidateRejectsInvalidPorts(t *testing.T) {
	tests := []State{
		{CodexDir: "~/.codex", CentralHost: "center", CentralPort: 70000, DeviceName: "mac"},
		{CodexDir: "~/.codex", CentralHost: "center", CentralPort: -1, DeviceName: "mac"},
		{CodexDir: "~/.codex", CentralHost: "center", CentralPort: 0, DeviceName: "mac"},
	}
	for _, state := range tests {
		if err := Validate(state); err == nil {
			t.Fatalf("Validate(%#v) succeeded", state)
		}
	}
}
