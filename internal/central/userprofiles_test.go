package central

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agentpal/internal/types"
)

func writeReservedProfile(t *testing.T, root, sub, name, providerInToml string) {
	t.Helper()
	dir := filepath.Join(root, sub, "auth-profiles", name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"profile.json":     `{"name":"` + name + `","type":"api_key","provider":"` + name + `"}`,
		"auth.json":        `{"OPENAI_API_KEY":"sk-` + name + `"}`,
		"config-auth.toml": "model_provider = \"" + providerInToml + "\"\n\n[model_providers." + providerInToml + "]\nname = \"" + providerInToml + "\"\nbase_url = \"https://example/v1\"\n",
	}
	for file, content := range files {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUserProfileStoreListsMetadataOnly(t *testing.T) {
	root := t.TempDir()
	writeReservedProfile(t, root, "user-1", "yangpu", "yangpu")
	store := newUserProfileStore(root)
	items, err := store.list("user-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "yangpu" || items[0].Provider != "yangpu" || items[0].Type != "api_key" {
		t.Fatalf("items=%#v", items)
	}
	if items[0].ContentHash == "" {
		t.Fatal("missing content hash")
	}
	if _, err := os.Stat(filepath.Join(root, "user-2")); !os.IsNotExist(err) {
		t.Fatal("unexpected cross-user directory")
	}
	if other, err := store.list("user-2"); err != nil || len(other) != 0 {
		t.Fatalf("missing user should list empty: %#v err=%v", other, err)
	}
}

func TestUserProfileStoreSkipsInvalidEntries(t *testing.T) {
	root := t.TempDir()
	writeReservedProfile(t, root, "user-1", "good", "good")
	writeReservedProfile(t, root, "user-1", "mismatch", "other")
	incomplete := filepath.Join(root, "user-1", "auth-profiles", "incomplete")
	if err := os.MkdirAll(incomplete, 0o700); err != nil {
		t.Fatal(err)
	}
	store := newUserProfileStore(root)
	items, err := store.list("user-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "good" {
		t.Fatalf("items=%#v", items)
	}
}

func TestUserProfileStoreReadDetail(t *testing.T) {
	root := t.TempDir()
	writeReservedProfile(t, root, "user-1", "yangpu", "yangpu")
	store := newUserProfileStore(root)
	profile, err := store.read("user-1", "yangpu")
	if err != nil {
		t.Fatal(err)
	}
	if string(profile.AuthJSON) == "" || string(profile.ConfigTOML) == "" || profile.ContentHash == "" {
		t.Fatalf("profile=%#v", profile)
	}
	if profile.UpdatedAt.IsZero() || profile.UpdatedAt.After(time.Now().Add(time.Minute)) {
		t.Fatalf("unexpected updatedAt: %v", profile.UpdatedAt)
	}
}

func TestUserProfileStoreRejectsPathTraversal(t *testing.T) {
	store := newUserProfileStore(t.TempDir())
	if _, err := store.read("user-1", "../secret"); err == nil {
		t.Fatal("path traversal accepted")
	}
	if _, err := store.list("../secret"); err == nil {
		t.Fatal("subject traversal accepted")
	}
}

func TestAuthProfilesRouteIsolatesUsers(t *testing.T) {
	root := t.TempDir()
	writeReservedProfile(t, root, "user-1", "yangpu", "yangpu")
	writeReservedProfile(t, root, "user-2", "other", "other")
	server := NewServer(types.ServerOptions{CodexDir: t.TempDir(), Port: 28888, UserStoreDir: root})

	list := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/auth-profiles", nil)
	req.Header.Set("Authorization", "Bearer "+unsignedJWT(t, map[string]any{"sub": "user-1"}))
	server.Handler().ServeHTTP(list, req)
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	if !strings.Contains(list.Body.String(), "yangpu") || strings.Contains(list.Body.String(), "other") {
		t.Fatalf("list body=%s", list.Body.String())
	}

	detail := httptest.NewRecorder()
	detailReq := httptest.NewRequest(http.MethodGet, "/v1/auth-profiles/yangpu", nil)
	detailReq.Header.Set("Authorization", "Bearer "+unsignedJWT(t, map[string]any{"sub": "user-1"}))
	server.Handler().ServeHTTP(detail, detailReq)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), "configAuthToml") {
		t.Fatalf("detail status=%d body=%s", detail.Code, detail.Body.String())
	}

	missing := httptest.NewRecorder()
	server.Handler().ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/v1/auth-profiles", nil))
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("missing auth status=%d", missing.Code)
	}

	traversal := httptest.NewRecorder()
	traversalReq := httptest.NewRequest(http.MethodGet, "/v1/auth-profiles/..%2Fsecret", nil)
	traversalReq.Header.Set("Authorization", "Bearer "+unsignedJWT(t, map[string]any{"sub": "user-1"}))
	server.Handler().ServeHTTP(traversal, traversalReq)
	if traversal.Code == http.StatusOK {
		t.Fatalf("traversal accepted: %s", traversal.Body.String())
	}
}
