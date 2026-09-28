package login

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoginStateRoundTrip(t *testing.T) {
	root := t.TempDir()
	if state, err := Read(root); err != nil || state.AccessToken != "" {
		t.Fatalf("empty read: %#v err=%v", state, err)
	}
	state := State{Subject: "user-1", AccessToken: "token", ExpiresAt: time.Now().Add(time.Hour).UTC().Truncate(time.Second)}
	if err := Write(root, state); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(root, "login.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
	got, err := Read(root)
	if err != nil || got.Subject != "user-1" || got.AccessToken != "token" {
		t.Fatalf("read: %#v err=%v", got, err)
	}
	if err := Clear(root); err != nil {
		t.Fatal(err)
	}
	if state, err := Read(root); err != nil || state.AccessToken != "" {
		t.Fatalf("read after clear: %#v err=%v", state, err)
	}
}
