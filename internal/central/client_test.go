package central

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientFetchesAndValidatesManifest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/config/manifest" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schema":1,"app":"AgentPal","version":"0.2.0","generatedAt":"2026-09-21T00:00:00Z","skills":[],"mcp":[]}`))
	}))
	defer server.Close()
	client := NewClient(server.Client(), 2*time.Second)
	manifest, err := client.Manifest(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Schema != 1 || manifest.App != "AgentPal" {
		t.Fatalf("manifest: %#v", manifest)
	}
}

func TestClientRejectsUnsupportedManifest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"schema":9,"app":"Other"}`)) }))
	defer server.Close()
	_, err := NewClient(server.Client(), 2*time.Second).Manifest(server.URL)
	if err == nil {
		t.Fatal("unsupported manifest was accepted")
	}
}
