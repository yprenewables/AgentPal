package central

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type PortalClaims struct {
	Subject       string
	Role          string
	TokenUse      string
	PrincipalType string
	Issuer        string
	ExpiresAt     int64
}

type PortalJWTVerifier struct {
	publicKey *rsa.PublicKey
	issuer    string
}

func NewPortalJWTVerifier(pemBytes []byte, issuer string) (*PortalJWTVerifier, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("portal JWT public key is not PEM")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse portal JWT public key: %w", err)
	}
	publicKey, ok := key.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("portal JWT public key is not RSA")
	}
	if issuer == "" {
		return nil, errors.New("portal JWT issuer is required")
	}
	return &PortalJWTVerifier{publicKey: publicKey, issuer: issuer}, nil
}

func NewPortalJWTVerifierFromFile(path, issuer string) (*PortalJWTVerifier, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return NewPortalJWTVerifier(data, issuer)
}

func (v *PortalJWTVerifier) VerifyRequest(r *http.Request) (PortalClaims, error) {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if token == "" {
		if cookie, err := r.Cookie("access_token"); err == nil {
			token = cookie.Value
		}
	}
	claims, err := v.VerifySession(token)
	if err != nil || claims.Role != "admin" {
		return PortalClaims{}, errors.New("portal JWT is not an administrator session")
	}
	return claims, nil
}

func (v *PortalJWTVerifier) Verify(token string) (PortalClaims, error) {
	return v.VerifyRequest(&http.Request{Header: http.Header{"Authorization": []string{"Bearer " + token}}})
}

func (v *PortalJWTVerifier) VerifySession(token string) (PortalClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return PortalClaims{}, errors.New("invalid portal JWT")
	}
	header, err := decodeJWTPart(parts[0])
	if err != nil || header["alg"] != "RS256" {
		return PortalClaims{}, errors.New("unsupported portal JWT algorithm")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return PortalClaims{}, errors.New("invalid portal JWT signature")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(v.publicKey, crypto.SHA256, digest[:], signature); err != nil {
		return PortalClaims{}, errors.New("invalid portal JWT signature")
	}
	body, err := decodeJWTPart(parts[1])
	if err != nil {
		return PortalClaims{}, errors.New("invalid portal JWT claims")
	}
	claims := PortalClaims{
		Subject: stringClaim(body, "sub"), Role: stringClaim(body, "role"),
		TokenUse: stringClaim(body, "token_use"), PrincipalType: stringClaim(body, "principal_type"),
		Issuer: stringClaim(body, "iss"), ExpiresAt: intClaim(body, "exp"),
	}
	if claims.Subject == "" || claims.TokenUse != "session" || claims.PrincipalType != "user" || claims.Issuer != v.issuer || claims.ExpiresAt <= time.Now().Unix() {
		return PortalClaims{}, errors.New("portal JWT claims are not authorized")
	}
	return claims, nil
}

func (v *PortalJWTVerifier) VerifySessionRequest(r *http.Request) (PortalClaims, error) {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if token == "" {
		if cookie, err := r.Cookie("access_token"); err == nil {
			token = cookie.Value
		}
	}
	return v.VerifySession(token)
}

func decodeJWTPart(value string) (map[string]any, error) {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func stringClaim(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

func intClaim(values map[string]any, key string) int64 {
	value, _ := values[key].(float64)
	return int64(value)
}
