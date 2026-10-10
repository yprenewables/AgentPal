package centralnode

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPortalJWTVerifierAcceptsAdminSession(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewPortalJWTVerifier(publicKeyPEM(t, &key.PublicKey), "internal-sso-portal")
	if err != nil {
		t.Fatal(err)
	}
	token := signTestJWT(t, key, map[string]any{
		"sub": "user-1", "role": "admin", "token_use": "session", "principal_type": "user",
		"iss": "internal-sso-portal", "exp": time.Now().Add(time.Minute).Unix(),
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	claims, err := verifier.VerifyRequest(req)
	if err != nil || claims.Subject != "user-1" {
		t.Fatalf("claims=%#v err=%v", claims, err)
	}
}

func TestPortalJWTVerifierRejectsMemberAndWrongIssuer(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewPortalJWTVerifier(publicKeyPEM(t, &key.PublicKey), "internal-sso-portal")
	if err != nil {
		t.Fatal(err)
	}
	for _, claims := range []map[string]any{
		{"sub": "user-1", "role": "member", "token_use": "session", "principal_type": "user", "iss": "internal-sso-portal", "exp": time.Now().Add(time.Minute).Unix()},
		{"sub": "user-1", "role": "admin", "token_use": "session", "principal_type": "user", "iss": "wrong", "exp": time.Now().Add(time.Minute).Unix()},
	} {
		token := signTestJWT(t, key, claims)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		if _, err := verifier.VerifyRequest(req); err == nil {
			t.Fatalf("expected rejection for %#v", claims)
		}
	}
}

func publicKeyPEM(t *testing.T, key *rsa.PublicKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}

func signTestJWT(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	header := b64JSON(t, map[string]string{"alg": "RS256", "typ": "JWT"})
	body := b64JSON(t, claims)
	signed := header + "." + body
	digest := sha256.Sum256([]byte(signed))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return signed + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func b64JSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(data)
}
