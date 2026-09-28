package central

import (
	"strings"
	"testing"
)

func unsignedJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	return b64JSON(t, map[string]string{"alg": "RS256", "typ": "JWT"}) + "." + b64JSON(t, claims) + ".signature"
}

func TestParseJWTSubjectReadsSub(t *testing.T) {
	token := unsignedJWT(t, map[string]any{"sub": "user-1", "iss": "https://id.yprenew.com"})
	subject, err := ParseJWTSubject(token)
	if err != nil || subject != "user-1" {
		t.Fatalf("subject=%q err=%v", subject, err)
	}
}

func TestParseJWTSubjectRejectsInvalidToken(t *testing.T) {
	for _, token := range []string{"", "a.b", "a.b.c.d", "a.!!!.x", "a." + b64JSON(t, "not-an-object") + ".x"} {
		if _, err := ParseJWTSubject(token); err == nil {
			t.Fatalf("token accepted: %q", token)
		}
	}
}

func TestParseJWTSubjectRejectsUnsafeSub(t *testing.T) {
	for _, sub := range []string{"", ".", "..", "a/b", `a\b`, "a..b", "a:b", "a\x00b", strings.Repeat("x", 257)} {
		token := unsignedJWT(t, map[string]any{"sub": sub})
		if _, err := ParseJWTSubject(token); err == nil {
			t.Fatalf("sub accepted: %q", sub)
		}
	}
}
