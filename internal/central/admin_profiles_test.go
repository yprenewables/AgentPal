package central

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agentpal/internal/types"
)

func adminServerWithKey(t *testing.T) (*Server, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(t.TempDir(), "portal.pem")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	server := NewServer(types.ServerOptions{CodexDir: t.TempDir(), Port: 28888, UserStoreDir: t.TempDir(), AdminJWTIssuer: "internal-sso-portal", AdminJWTPublic: keyPath})
	return server, key
}

func adminToken(t *testing.T, key *rsa.PrivateKey, role string) string {
	t.Helper()
	return signTestJWT(t, key, map[string]any{"sub": "admin-1", "role": role, "token_use": "session", "principal_type": "user", "iss": "internal-sso-portal", "exp": time.Now().Add(time.Minute).Unix()})
}

func adminDo(server *Server, method, path, token string, body io.Reader) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, req)
	return recorder
}

const adminProfileBody = `{"type":"api_key","provider":"yangpu","auth":{"OPENAI_API_KEY":"sk-1"},"configAuthToml":"model_provider = \"yangpu\"\n\n[model_providers.yangpu]\nname = \"yangpu\"\nbase_url = \"https://x/v1\"\n"}`

func TestAdminProfilesCrudRoundTrip(t *testing.T) {
	server, key := adminServerWithKey(t)
	token := adminToken(t, key, "admin")

	created := adminDo(server, http.MethodPost, "/v1/admin/user-profiles/user-1/yangpu", token, strings.NewReader(adminProfileBody))
	if created.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	if !strings.Contains(created.Body.String(), "contentHash") {
		t.Fatalf("create body=%s", created.Body.String())
	}

	list := adminDo(server, http.MethodGet, "/v1/admin/user-profiles/user-1", token, nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "yangpu") {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}

	users := adminDo(server, http.MethodGet, "/v1/admin/users", token, nil)
	if users.Code != http.StatusOK || !strings.Contains(users.Body.String(), "user-1") {
		t.Fatalf("users status=%d body=%s", users.Code, users.Body.String())
	}

	detail := adminDo(server, http.MethodGet, "/v1/admin/user-profiles/user-1/yangpu", token, nil)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), "OPENAI_API_KEY") || !strings.Contains(detail.Body.String(), "configAuthToml") {
		t.Fatalf("detail status=%d body=%s", detail.Code, detail.Body.String())
	}

	overwrite := adminDo(server, http.MethodPost, "/v1/admin/user-profiles/user-1/yangpu", token, strings.NewReader(strings.Replace(adminProfileBody, "sk-1", "sk-2", 1)))
	if overwrite.Code != http.StatusOK || !strings.Contains(overwrite.Body.String(), "sk-2") {
		t.Fatalf("overwrite status=%d body=%s", overwrite.Code, overwrite.Body.String())
	}

	removed := adminDo(server, http.MethodDelete, "/v1/admin/user-profiles/user-1/yangpu", token, nil)
	if removed.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", removed.Code, removed.Body.String())
	}
	after := adminDo(server, http.MethodGet, "/v1/admin/user-profiles/user-1", token, nil)
	if after.Code != http.StatusOK || strings.Contains(after.Body.String(), "yangpu") {
		t.Fatalf("after delete status=%d body=%s", after.Code, after.Body.String())
	}
	usersAfter := adminDo(server, http.MethodGet, "/v1/admin/users", token, nil)
	if usersAfter.Code != http.StatusOK || strings.Contains(usersAfter.Body.String(), "user-1") {
		t.Fatalf("users after delete status=%d body=%s", usersAfter.Code, usersAfter.Body.String())
	}
}

func TestAdminProfilesRequireAdminAuth(t *testing.T) {
	unconfigured := NewServer(types.ServerOptions{CodexDir: t.TempDir(), Port: 28888, UserStoreDir: t.TempDir()})
	if got := adminDo(unconfigured, http.MethodGet, "/v1/admin/users", "", nil); got.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured status=%d", got.Code)
	}

	server, key := adminServerWithKey(t)
	if got := adminDo(server, http.MethodGet, "/v1/admin/users", "", nil); got.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status=%d", got.Code)
	}
	member := adminToken(t, key, "member")
	if got := adminDo(server, http.MethodGet, "/v1/admin/users", member, nil); got.Code != http.StatusForbidden {
		t.Fatalf("member token status=%d", got.Code)
	}
	if got := adminDo(server, http.MethodPost, "/v1/admin/user-profiles/user-1/yangpu", "", strings.NewReader(adminProfileBody)); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized write status=%d", got.Code)
	}
}

func TestAdminProfileWriteValidation(t *testing.T) {
	server, key := adminServerWithKey(t)
	token := adminToken(t, key, "admin")

	cases := []struct {
		path string
		body string
	}{
		{"/v1/admin/user-profiles/user-1/yangpu", `{"type":"api_key","provider":"yangpu","auth":null,"configAuthToml":"model_provider = \"yangpu\"\n"}`},
		{"/v1/admin/user-profiles/user-1/yangpu", `{"type":"","provider":"yangpu","auth":{},"configAuthToml":"model_provider = \"yangpu\"\n"}`},
		{"/v1/admin/user-profiles/user-1/yangpu", `{"type":"api_key","provider":"yangpu","auth":{},"configAuthToml":"model_provider = \"other\"\n"}`},
		{"/v1/admin/user-profiles/user-1/yangpu", `{"type":"api_key","provider":"yangpu","auth":{},"configAuthToml":"==="}`},
		{"/v1/admin/user-profiles/user-1/bad:name", adminProfileBody},
		{"/v1/admin/user-profiles/a..b/yangpu", adminProfileBody},
	}
	for _, item := range cases {
		got := adminDo(server, http.MethodPost, item.path, token, strings.NewReader(item.body))
		if got.Code != http.StatusBadRequest {
			t.Fatalf("path=%s status=%d body=%s", item.path, got.Code, got.Body.String())
		}
	}
}

func TestAdminProfilesAcceptCasdoorAdminClaim(t *testing.T) {
	server := NewServer(types.ServerOptions{CodexDir: t.TempDir(), Port: 28888, UserStoreDir: t.TempDir(), CasdoorURL: "https://id.yprenew.com"})
	admin := unsignedJWT(t, map[string]any{"iss": "https://id.yprenew.com", "sub": "user-1", "isAdmin": true})
	if got := adminDo(server, http.MethodGet, "/v1/admin/users", admin, nil); got.Code != http.StatusOK {
		t.Fatalf("admin claim status=%d body=%s", got.Code, got.Body.String())
	}
	plain := unsignedJWT(t, map[string]any{"iss": "https://id.yprenew.com", "sub": "user-2"})
	if got := adminDo(server, http.MethodGet, "/v1/admin/users", plain, nil); got.Code != http.StatusForbidden {
		t.Fatalf("non-admin status=%d", got.Code)
	}
	wrongIssuer := unsignedJWT(t, map[string]any{"iss": "https://evil.example", "sub": "user-1", "isAdmin": true})
	if got := adminDo(server, http.MethodGet, "/v1/admin/users", wrongIssuer, nil); got.Code != http.StatusUnauthorized {
		t.Fatalf("wrong issuer status=%d", got.Code)
	}
}

func TestAdminMeUsesSubsAllowlist(t *testing.T) {
	server := NewServer(types.ServerOptions{CodexDir: t.TempDir(), Port: 28888, UserStoreDir: t.TempDir(), CasdoorURL: "https://id.yprenew.com", AdminSubs: "user-9, user-10"})
	token := unsignedJWT(t, map[string]any{"iss": "https://id.yprenew.com", "sub": "user-9"})
	got := adminDo(server, http.MethodGet, "/v1/admin/me", token, nil)
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "user-9") {
		t.Fatalf("allowlist status=%d body=%s", got.Code, got.Body.String())
	}
}
