package central

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"agentpal/internal/authprofile"
	"agentpal/internal/types"

	"github.com/pelletier/go-toml/v2"
)

type adminProfileRequest struct {
	Type           string         `json:"type"`
	Provider       string         `json:"provider"`
	Auth           map[string]any `json:"auth"`
	ConfigAuthToml string         `json:"configAuthToml"`
}

// adminSubject 返回发起请求的管理员 subject。
// 状态码为 0 表示通过；否则为应返回的 HTTP 状态码与错误信息。
func (s *Server) adminSubject(r *http.Request) (string, int, string) {
	if s.adminAuth == nil && strings.TrimSpace(s.options.CasdoorURL) == "" {
		return "", http.StatusServiceUnavailable, "administrator authentication is not configured"
	}
	if s.adminAuth != nil {
		if claims, err := s.adminAuth.VerifySessionRequest(r); err == nil {
			if claims.Role == "admin" {
				return claims.Subject, 0, ""
			}
			return "", http.StatusForbidden, "this account is not authorized for administration"
		}
	}
	if strings.TrimSpace(s.options.CasdoorURL) == "" {
		return "", http.StatusUnauthorized, "administrator login required"
	}
	token := requestBearer(r)
	if token == "" {
		return "", http.StatusUnauthorized, "administrator login required"
	}
	payload, err := ParseJWTClaims(token)
	if err != nil {
		return "", http.StatusUnauthorized, "administrator login required"
	}
	issuer, _ := payload["iss"].(string)
	if strings.TrimRight(issuer, "/") != strings.TrimRight(s.options.CasdoorURL, "/") {
		return "", http.StatusUnauthorized, "administrator login required"
	}
	subject, _ := payload["sub"].(string)
	if err := validateSubject(subject); err != nil {
		return "", http.StatusUnauthorized, "administrator login required"
	}
	if !s.isAdmin(subject, payload) {
		return "", http.StatusForbidden, "this account is not authorized for administration"
	}
	return subject, 0, ""
}

func (s *Server) isAdmin(subject string, payload map[string]any) bool {
	claim := strings.TrimSpace(s.options.CasdoorAdminClaim)
	if claim == "" {
		claim = "isAdmin"
	}
	if truthy(payload[claim]) {
		return true
	}
	for _, candidate := range strings.Split(s.options.AdminSubs, ",") {
		if strings.TrimSpace(candidate) == subject {
			return true
		}
	}
	return false
}

func truthy(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "admin", "1":
			return true
		}
	case []any:
		for _, item := range typed {
			if truthy(item) {
				return true
			}
		}
	}
	return false
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if _, status, message := s.adminSubject(r); status != 0 {
		http.Error(w, message, status)
		return false
	}
	return true
}

func (s *Server) adminMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	subject, status, message := s.adminSubject(r)
	if status != 0 {
		http.Error(w, message, status)
		return
	}
	writeJSON(w, map[string]any{"subject": subject, "admin": true})
}

func (s *Server) adminUsers(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	subs, err := s.userProfiles.listUsers()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, subs)
}

func (s *Server) adminProfiles(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/admin/user-profiles"), "/")
	if rest == "" {
		http.Error(w, "user sub is required", http.StatusBadRequest)
		return
	}
	parts := strings.Split(rest, "/")
	if len(parts) > 2 {
		http.NotFound(w, r)
		return
	}
	sub := parts[0]
	if err := validateSubject(sub); err != nil {
		http.Error(w, "invalid user sub", http.StatusBadRequest)
		return
	}
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			items, err := s.userProfiles.list(sub)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, items)
		case http.MethodPost:
			s.adminApplyReserved(w, r, sub)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}
	name := parts[1]
	switch r.Method {
	case http.MethodGet:
		profile, err := s.userProfiles.read(sub, name)
		if err != nil {
			http.Error(w, "profile not found", http.StatusNotFound)
			return
		}
		writeJSON(w, profileDetail(profile))
	case http.MethodDelete:
		if err := s.userProfiles.delete(sub, name); err != nil {
			http.Error(w, "profile not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPost:
		s.adminProfileSave(w, r, sub, name)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) adminProfileSave(w http.ResponseWriter, r *http.Request, sub, name string) {
	if err := authprofile.ValidateReservedName(name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 5<<20))
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	var req adminProfileRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Type == "" || req.Provider == "" {
		http.Error(w, "type and provider are required", http.StatusBadRequest)
		return
	}
	if req.Auth == nil {
		http.Error(w, "auth must be a JSON object", http.StatusBadRequest)
		return
	}
	var config map[string]any
	if err := toml.Unmarshal([]byte(req.ConfigAuthToml), &config); err != nil {
		http.Error(w, "configAuthToml is invalid: "+err.Error(), http.StatusBadRequest)
		return
	}
	if provider, _ := config["model_provider"].(string); provider != name {
		http.Error(w, "configAuthToml model_provider must equal the profile name", http.StatusBadRequest)
		return
	}
	authJSON, err := json.Marshal(req.Auth)
	if err != nil {
		http.Error(w, "auth is invalid", http.StatusBadRequest)
		return
	}
	meta := reservedMeta{Name: name, Type: req.Type, Provider: req.Provider}
	if err := s.userProfiles.write(sub, name, meta, authJSON, []byte(req.ConfigAuthToml)); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	profile, err := s.userProfiles.read(sub, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, profileDetail(profile))
}

func buildReservedProfile(settings types.ReservedSettings, apiKey string) authprofile.CentralProfile {
	quote := strconv.Quote
	config := "model_provider = " + quote(settings.Provider) + "\n\n" +
		"[model_providers." + settings.Provider + "]\n" +
		"name = " + quote(settings.Provider) + "\n" +
		"base_url = " + quote(settings.BaseURL) + "\n" +
		"env_key = " + quote(settings.EnvKey) + "\n" +
		"wire_api = " + quote(settings.WireAPI) + "\n"
	if strings.HasPrefix(apiKey, "sk-mem-") {
		config += "experimental_bearer_token = " + quote(apiKey) + "\n"
	}
	authJSON, _ := json.Marshal(map[string]string{settings.EnvKey: apiKey})
	return authprofile.CentralProfile{
		Name: settings.Provider, Provider: settings.Provider, Type: "api_key",
		AuthJSON: authJSON, ConfigTOML: []byte(config),
	}
}

// adminApplyReserved 用系统配置里的统一参数为用户生成预留 profile，管理员只需提供 api_key。
func (s *Server) adminApplyReserved(w http.ResponseWriter, r *http.Request, sub string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	var request struct {
		APIKey string `json:"apiKey"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	request.APIKey = strings.TrimSpace(request.APIKey)
	if request.APIKey == "" {
		http.Error(w, "apiKey is required", http.StatusBadRequest)
		return
	}
	settings, err := s.settings.get()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	profile := buildReservedProfile(settings, request.APIKey)
	meta := reservedMeta{Name: profile.Name, Type: profile.Type, Provider: profile.Provider}
	if err := s.userProfiles.write(sub, profile.Name, meta, profile.AuthJSON, profile.ConfigTOML); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	stored, err := s.userProfiles.read(sub, profile.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, profileDetail(stored))
}

func (s *Server) adminSettings(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		settings, err := s.settings.get()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, settings)
	case http.MethodPut:
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		if err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		var settings types.ReservedSettings
		if err := json.Unmarshal(body, &settings); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := s.settings.put(settings); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		saved, err := s.settings.get()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, saved)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) adminAllProfiles(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	items, err := s.userProfiles.listAll()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, items)
}

// adminCasdoorUsers 用管理员自己的 Casdoor 令牌代理 Casdoor 的用户列表。
func (s *Server) adminCasdoorUsers(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if strings.TrimSpace(s.options.CasdoorURL) == "" {
		http.Error(w, "casdoor is not configured", http.StatusServiceUnavailable)
		return
	}
	token := requestBearer(r)
	if token == "" {
		http.Error(w, "casdoor token is required", http.StatusUnauthorized)
		return
	}
	organization := strings.TrimSpace(s.options.CasdoorOrganization)
	if organization == "" {
		organization = "built-in"
	}
	target := strings.TrimRight(s.options.CasdoorURL, "/") + "/api/get-users?owner=" + url.QueryEscape(organization)
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := s.oauthClient.Do(req)
	if err != nil {
		http.Error(w, "casdoor is unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		http.Error(w, "casdoor user list failed: "+resp.Status, http.StatusBadGateway)
		return
	}
	var payload struct {
		Status string `json:"status"`
		Msg    string `json:"msg"`
		Data   []struct {
			Id          string `json:"id"`
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
			Email       string `json:"email"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "casdoor response is invalid", http.StatusBadGateway)
		return
	}
	if payload.Status != "ok" {
		message := payload.Msg
		if message == "" {
			message = "casdoor rejected the request"
		}
		http.Error(w, message, http.StatusBadGateway)
		return
	}
	users := []types.CasdoorUser{}
	for _, item := range payload.Data {
		if item.Id == "" {
			continue
		}
		users = append(users, types.CasdoorUser{Sub: item.Id, Name: item.Name, DisplayName: item.DisplayName, Email: item.Email})
	}
	sort.Slice(users, func(i, j int) bool { return users[i].Name < users[j].Name })
	writeJSON(w, users)
}
