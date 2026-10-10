package centralnode

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"agentpal/internal/types"
)

func TestAdminStatusIgnoresOptionalJWTConfiguration(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(t.TempDir(), "portal.pem")
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	codexDir := t.TempDir()
	server := NewServer(types.ServerOptions{CodexDir: codexDir, Port: 28888, AdminJWTIssuer: "internal-sso-portal", AdminJWTPublic: keyPath})

	token := signTestJWT(t, key, map[string]any{"sub": "admin-1", "role": "admin", "token_use": "session", "principal_type": "user", "iss": "internal-sso-portal", "exp": time.Now().Add(time.Minute).Unix()})
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	authorized := httptest.NewRecorder()
	server.Handler().ServeHTTP(authorized, req)
	if authorized.Code != http.StatusOK {
		t.Fatalf("authorized status=%d body=%s", authorized.Code, authorized.Body.String())
	}
}

func TestAdminStatusWorksWithoutAuthenticationConfiguration(t *testing.T) {
	server := NewServer(types.ServerOptions{CodexDir: t.TempDir(), Port: 28888})
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/admin/status", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
