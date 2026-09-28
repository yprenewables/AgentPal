package central

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"agentpal/internal/types"
)

const loginTTL = 5 * time.Minute

type pendingLogin struct {
	ID           string
	PollToken    string
	State        string
	CodeVerifier string
	Challenge    string
	RedirectURL  string
	CreatedAt    time.Time
	ExpiresAt    time.Time
	Approved     bool
	Failed       bool
	FailReason   string
	Subject      string
	AccessToken  string
	ExpiresIn    int
	Consumed     bool
}

type pendingLogins struct {
	mu    sync.Mutex
	now   func() time.Time
	items map[string]*pendingLogin
}

func newPendingLogins() *pendingLogins {
	return &pendingLogins{now: time.Now, items: make(map[string]*pendingLogin)}
}

func codeChallenge(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func (p *pendingLogins) start(redirectURL string) (*pendingLogin, error) {
	id, err := randomToken(24)
	if err != nil {
		return nil, err
	}
	pollToken, err := randomToken(24)
	if err != nil {
		return nil, err
	}
	state, err := randomToken(24)
	if err != nil {
		return nil, err
	}
	verifier, err := randomToken(32)
	if err != nil {
		return nil, err
	}
	now := p.now()
	item := &pendingLogin{
		ID: id, PollToken: pollToken, State: state,
		CodeVerifier: verifier, Challenge: codeChallenge(verifier),
		RedirectURL: redirectURL, CreatedAt: now, ExpiresAt: now.Add(loginTTL),
	}
	p.mu.Lock()
	p.items[id] = item
	p.mu.Unlock()
	return item, nil
}

func (p *pendingLogins) findByStateLocked(state string) *pendingLogin {
	for _, item := range p.items {
		if item.State == state {
			return item
		}
	}
	return nil
}

func (p *pendingLogins) peekByState(state string) *pendingLogin {
	p.mu.Lock()
	defer p.mu.Unlock()
	item := p.findByStateLocked(state)
	if item == nil || p.now().After(item.ExpiresAt) || item.Consumed {
		return nil
	}
	return item
}

func (p *pendingLogins) approve(state, subject, token string, expiresIn int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	item := p.findByStateLocked(state)
	if item == nil || p.now().After(item.ExpiresAt) || item.Consumed {
		return errors.New("login request expired")
	}
	item.Approved = true
	item.Subject = subject
	item.AccessToken = token
	item.ExpiresIn = expiresIn
	return nil
}

func (p *pendingLogins) fail(state, reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if item := p.findByStateLocked(state); item != nil {
		item.Failed = true
		item.FailReason = reason
	}
}

func (p *pendingLogins) take(id, pollToken string) (types.LoginStatus, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	item := p.items[id]
	if item == nil || p.now().After(item.ExpiresAt) {
		return types.LoginStatus{}, errors.New("login request expired")
	}
	if item.PollToken != pollToken {
		return types.LoginStatus{}, errors.New("login request is not authorized")
	}
	if item.Consumed {
		return types.LoginStatus{}, errors.New("login request already consumed")
	}
	if item.Failed {
		return types.LoginStatus{Status: "failed", Error: item.FailReason}, nil
	}
	if !item.Approved {
		return types.LoginStatus{Status: "pending"}, nil
	}
	item.Consumed = true
	return types.LoginStatus{Status: "approved", Subject: item.Subject, AccessToken: item.AccessToken, ExpiresIn: item.ExpiresIn}, nil
}

func (s *Server) oauthStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if strings.TrimSpace(s.options.CasdoorURL) == "" || strings.TrimSpace(s.options.CasdoorClientID) == "" {
		http.Error(w, "casdoor is not configured", http.StatusServiceUnavailable)
		return
	}
	redirect := strings.TrimSpace(s.options.OAuthRedirectURL)
	if redirect == "" {
		redirect = "http://" + r.Host + "/callback"
	}
	item, err := s.logins.start(redirect)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	scopes := strings.TrimSpace(s.options.CasdoorScopes)
	if scopes == "" {
		scopes = "openid profile"
	}
	values := url.Values{}
	values.Set("client_id", s.options.CasdoorClientID)
	values.Set("response_type", "code")
	values.Set("redirect_uri", redirect)
	values.Set("scope", scopes)
	values.Set("state", item.State)
	values.Set("code_challenge", item.Challenge)
	values.Set("code_challenge_method", "S256")
	authorizeURL := strings.TrimRight(s.options.CasdoorURL, "/") + "/login/oauth/authorize?" + values.Encode()
	writeJSON(w, types.LoginStart{LoginID: item.ID, PollToken: item.PollToken, AuthorizeURL: authorizeURL, ExpiresAt: item.ExpiresAt})
}

func (s *Server) oauthStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/auth/login/status/")
	status, err := s.logins.take(id, r.URL.Query().Get("pollToken"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusGone)
		return
	}
	writeJSON(w, status)
}

func (s *Server) oauthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		http.Error(w, "authorization code is missing", http.StatusBadRequest)
		return
	}
	item := s.logins.peekByState(state)
	if item == nil {
		http.Error(w, "authorization state is invalid", http.StatusGone)
		return
	}
	token, expiresIn, err := s.exchangeCode(item.RedirectURL, code, item.CodeVerifier)
	if err != nil {
		s.logins.fail(state, err.Error())
		http.Error(w, "login failed", http.StatusBadGateway)
		return
	}
	subject, err := ParseJWTSubject(token)
	if err != nil {
		s.logins.fail(state, err.Error())
		http.Error(w, "login failed", http.StatusBadGateway)
		return
	}
	if err := s.logins.approve(state, subject, token, expiresIn); err != nil {
		http.Error(w, err.Error(), http.StatusGone)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, "<!doctype html><meta name=viewport content=width=device-width><title>AgentPal login</title><style>body{font:16px system-ui;max-width:600px;margin:60px auto;padding:20px;color:#172033}</style><h1>Login complete</h1><p>You can return to the AgentPal application.</p>")
}

func (s *Server) exchangeCode(redirectURL, code, verifier string) (string, int, error) {
	if s.oauthClient == nil {
		return "", 0, errors.New("oauth client is not configured")
	}
	payload, err := json.Marshal(map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     s.options.CasdoorClientID,
		"code":          code,
		"redirect_uri":  redirectURL,
		"code_verifier": verifier,
	})
	if err != nil {
		return "", 0, err
	}
	resp, err := s.oauthClient.Post(strings.TrimRight(s.options.CasdoorURL, "/")+"/api/login/oauth/access_token", "application/json", bytes.NewReader(payload))
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", 0, errors.New("casdoor token exchange failed: " + resp.Status)
	}
	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", 0, err
	}
	if result.AccessToken == "" {
		if result.Error != "" {
			return "", 0, errors.New(result.Error)
		}
		return "", 0, errors.New("casdoor did not return an access token")
	}
	return result.AccessToken, result.ExpiresIn, nil
}
