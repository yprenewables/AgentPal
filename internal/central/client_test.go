package central

import (
	"encoding/json"
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

func TestClientLoginAndProfileCalls(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/auth/login/start", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"loginId": "id-1", "pollToken": "poll-1", "authorizeUrl": "http://casdoor/authorize", "expiresAt": time.Now().Add(time.Minute)})
	})
	mux.HandleFunc("/v1/auth/login/status/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pollToken") != "poll-1" {
			http.Error(w, "bad poll token", http.StatusGone)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "approved", "subject": "user-1", "accessToken": "token", "expiresIn": 3600})
	})
	mux.HandleFunc("/v1/auth-profiles", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{"name": "yangpu", "provider": "yangpu", "type": "api_key", "contentHash": "sha256:x", "updatedAt": time.Now()}})
	})
	mux.HandleFunc("/v1/auth-profiles/yangpu", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "yangpu", "provider": "yangpu", "type": "api_key", "contentHash": "sha256:x", "updatedAt": time.Now(), "auth": map[string]any{"OPENAI_API_KEY": "sk-1"}, "configAuthToml": "model_provider = \"yangpu\"\n"})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient(nil, 5*time.Second)
	start, err := client.StartLogin(server.URL)
	if err != nil || start.LoginID != "id-1" {
		t.Fatalf("start=%#v err=%v", start, err)
	}
	status, err := client.LoginStatus(server.URL, "id-1", "poll-1")
	if err != nil || status.Status != "approved" || status.AccessToken != "token" {
		t.Fatalf("status=%#v err=%v", status, err)
	}
	list, err := client.CentralAuthProfiles(server.URL, "token")
	if err != nil || len(list) != 1 || list[0].Name != "yangpu" {
		t.Fatalf("list=%#v err=%v", list, err)
	}
	detail, err := client.CentralAuthProfile(server.URL, "token", "yangpu")
	if err != nil || detail.Name != "yangpu" || detail.ConfigAuthToml == "" {
		t.Fatalf("detail=%#v err=%v", detail, err)
	}
	if _, err := client.CentralAuthProfiles(server.URL, "wrong"); err == nil {
		t.Fatal("expected unauthorized error")
	}
}
