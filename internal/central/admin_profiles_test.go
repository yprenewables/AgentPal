package central

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
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
	return adminServerWithStore(t, t.TempDir())
}

func adminServerWithStore(t *testing.T, root string) (*Server, *rsa.PrivateKey) {
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
	server := NewServer(types.ServerOptions{CodexDir: t.TempDir(), Port: 28888, UserStoreDir: root, AdminJWTIssuer: "internal-sso-portal", AdminJWTPublic: keyPath})
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

func TestAdminSettingsRoundTripAndValidation(t *testing.T) {
	server, key := adminServerWithKey(t)
	token := adminToken(t, key, "admin")

	defaults := adminDo(server, http.MethodGet, "/v1/admin/settings", token, nil)
	if defaults.Code != http.StatusOK || !strings.Contains(defaults.Body.String(), "\"provider\":\"memory\"") {
		t.Fatalf("defaults status=%d body=%s", defaults.Code, defaults.Body.String())
	}

	custom := `{"provider":"gateway","baseURL":"https://gw.example/v1","wireAPI":"responses","envKey":"OPENAI_API_KEY"}`
	saved := adminDo(server, http.MethodPut, "/v1/admin/settings", token, strings.NewReader(custom))
	if saved.Code != http.StatusOK || !strings.Contains(saved.Body.String(), "\"provider\":\"gateway\"") {
		t.Fatalf("save status=%d body=%s", saved.Code, saved.Body.String())
	}
	again := adminDo(server, http.MethodGet, "/v1/admin/settings", token, nil)
	if !strings.Contains(again.Body.String(), "https://gw.example/v1") {
		t.Fatalf("get after save body=%s", again.Body.String())
	}

	for _, bad := range []string{
		`{"provider":"a.b","baseURL":"https://gw.example/v1","wireAPI":"responses","envKey":"OPENAI_API_KEY"}`,
		`{"provider":"gw","baseURL":"not-a-url","wireAPI":"responses","envKey":"OPENAI_API_KEY"}`,
		`{"provider":"gw","baseURL":"https://gw.example/v1","wireAPI":"responses","envKey":"1bad"}`,
	} {
		if got := adminDo(server, http.MethodPut, "/v1/admin/settings", token, strings.NewReader(bad)); got.Code != http.StatusBadRequest {
			t.Fatalf("invalid settings accepted: %s -> %d", bad, got.Code)
		}
	}
}

func TestAdminApplyReservedUsesSettings(t *testing.T) {
	server, key := adminServerWithKey(t)
	token := adminToken(t, key, "admin")

	settings := `{"provider":"memory","baseURL":"https://memory.example/codex/default","wireAPI":"responses","envKey":"OPENAI_API_KEY"}`
	if got := adminDo(server, http.MethodPut, "/v1/admin/settings", token, strings.NewReader(settings)); got.Code != http.StatusOK {
		t.Fatalf("settings save status=%d body=%s", got.Code, got.Body.String())
	}

	applied := adminDo(server, http.MethodPost, "/v1/admin/user-profiles/user-1", token, strings.NewReader(`{"apiKey":"sk-mem-abc"}`))
	if applied.Code != http.StatusOK || !strings.Contains(applied.Body.String(), "\"provider\":\"memory\"") {
		t.Fatalf("apply status=%d body=%s", applied.Code, applied.Body.String())
	}

	detail := adminDo(server, http.MethodGet, "/v1/admin/user-profiles/user-1/memory", token, nil)
	body := detail.Body.String()
	if detail.Code != http.StatusOK || !strings.Contains(body, "sk-mem-abc") || !strings.Contains(body, "https://memory.example/codex/default") || !strings.Contains(body, "experimental_bearer_token") {
		t.Fatalf("detail status=%d body=%s", detail.Code, body)
	}

	empty := adminDo(server, http.MethodPost, "/v1/admin/user-profiles/user-1", token, strings.NewReader(`{"apiKey":""}`))
	if empty.Code != http.StatusBadRequest {
		t.Fatalf("empty api key status=%d", empty.Code)
	}
}

func TestAdminAllProfilesListsEveryUser(t *testing.T) {
	root := t.TempDir()
	writeReservedProfile(t, root, "user-1", "memory", "memory")
	writeReservedProfile(t, root, "user-2", "memory", "memory")
	server, key := adminServerWithStore(t, root)
	token := adminToken(t, key, "admin")

	got := adminDo(server, http.MethodGet, "/v1/admin/profiles", token, nil)
	if got.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", got.Code, got.Body.String())
	}
	body := got.Body.String()
	if !strings.Contains(body, "\"sub\":\"user-1\"") || !strings.Contains(body, "\"sub\":\"user-2\"") || !strings.Contains(body, "\"name\":\"memory\"") {
		t.Fatalf("body=%s", body)
	}
	if got := adminDo(server, http.MethodGet, "/v1/admin/profiles", "", nil); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d", got.Code)
	}
}

func TestAdminCasdoorUsersProxiesUserList(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/get-users" || r.URL.Query().Get("owner") != "built-in" {
			http.Error(w, "unexpected path", http.StatusBadRequest)
			return
		}
		if r.Header.Get("Authorization") == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"data": []map[string]any{
				{"id": "uuid-alice", "name": "alice", "displayName": "Alice", "email": "alice@example.com"},
				{"id": "uuid-bob", "name": "bob", "displayName": "Bob"},
			},
		})
	}))
	defer stub.Close()

	server := NewServer(types.ServerOptions{CodexDir: t.TempDir(), Port: 28888, UserStoreDir: t.TempDir(), CasdoorURL: stub.URL, CasdoorOrganization: "built-in"})
	token := unsignedJWT(t, map[string]any{"iss": stub.URL, "sub": "admin", "isAdmin": true})
	got := adminDo(server, http.MethodGet, "/v1/admin/casdoor-users", token, nil)
	if got.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", got.Code, got.Body.String())
	}
	body := got.Body.String()
	if !strings.Contains(body, "\"sub\":\"uuid-alice\"") || !strings.Contains(body, "\"displayName\":\"Alice\"") || !strings.Contains(body, "uuid-bob") {
		t.Fatalf("body=%s", body)
	}
}
