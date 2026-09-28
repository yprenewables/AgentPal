package central

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"agentpal/internal/types"
)

type capturedExchange struct {
	Code     string
	Verifier string
}

func startLogin(t *testing.T, server *Server) types.LoginStart {
	t.Helper()
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/auth/login/start", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("start status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var start types.LoginStart
	if err := json.Unmarshal(recorder.Body.Bytes(), &start); err != nil {
		t.Fatal(err)
	}
	return start
}

func TestLoginFlowApprovesOnceWithPKCE(t *testing.T) {
	captured := &capturedExchange{}
	accessToken := unsignedJWT(t, map[string]any{"sub": "user-1"})
	var challenge string
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]string
		_ = json.Unmarshal(body, &payload)
		captured.Code = payload["code"]
		captured.Verifier = payload["code_verifier"]
		digest := sha256.Sum256([]byte(payload["code_verifier"]))
		if base64.RawURLEncoding.EncodeToString(digest[:]) != challenge {
			http.Error(w, "challenge mismatch", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": accessToken, "expires_in": 3600})
	}))
	defer stub.Close()

	server := NewServer(types.ServerOptions{CodexDir: t.TempDir(), Port: 28888, CasdoorURL: stub.URL, CasdoorClientID: "cid", OAuthRedirectURL: "http://central.test/callback"})
	start := startLogin(t, server)
	parsed, err := url.Parse(start.AuthorizeURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("authorize url=%s", start.AuthorizeURL)
	}
	challenge = parsed.Query().Get("code_challenge")
	state := parsed.Query().Get("state")
	if state == "" || challenge == "" {
		t.Fatalf("authorize url=%s", start.AuthorizeURL)
	}

	callback := httptest.NewRecorder()
	server.Handler().ServeHTTP(callback, httptest.NewRequest(http.MethodGet, "/callback?code=abc&state="+url.QueryEscape(state), nil))
	if callback.Code != http.StatusOK {
		t.Fatalf("callback status=%d body=%s", callback.Code, callback.Body.String())
	}
	if captured.Code != "abc" || captured.Verifier == "" {
		t.Fatalf("exchange captured=%#v", captured)
	}

	status := httptest.NewRecorder()
	server.Handler().ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/v1/auth/login/status/"+start.LoginID+"?pollToken="+url.QueryEscape(start.PollToken), nil))
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), accessToken) {
		t.Fatalf("status=%d body=%s", status.Code, status.Body.String())
	}

	again := httptest.NewRecorder()
	server.Handler().ServeHTTP(again, httptest.NewRequest(http.MethodGet, "/v1/auth/login/status/"+start.LoginID+"?pollToken="+url.QueryEscape(start.PollToken), nil))
	if again.Code != http.StatusGone {
		t.Fatalf("second status=%d body=%s", again.Code, again.Body.String())
	}
}

func TestLoginRejectsWrongStateAndPollToken(t *testing.T) {
	server := NewServer(types.ServerOptions{CodexDir: t.TempDir(), Port: 28888, CasdoorURL: "http://casdoor.test", CasdoorClientID: "cid", OAuthRedirectURL: "http://central.test/callback"})
	start := startLogin(t, server)

	badState := httptest.NewRecorder()
	server.Handler().ServeHTTP(badState, httptest.NewRequest(http.MethodGet, "/callback?code=abc&state=nope", nil))
	if badState.Code == http.StatusOK {
		t.Fatalf("bad state accepted: %d", badState.Code)
	}

	badPoll := httptest.NewRecorder()
	server.Handler().ServeHTTP(badPoll, httptest.NewRequest(http.MethodGet, "/v1/auth/login/status/"+start.LoginID+"?pollToken=wrong", nil))
	if badPoll.Code != http.StatusGone {
		t.Fatalf("bad poll token status=%d", badPoll.Code)
	}
}

func TestLoginStartRequiresCasdoorConfiguration(t *testing.T) {
	server := NewServer(types.ServerOptions{CodexDir: t.TempDir(), Port: 28888})
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/auth/login/start", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
