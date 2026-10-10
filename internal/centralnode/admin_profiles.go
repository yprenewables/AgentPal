package centralnode

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"agentpal/internal/authprofile"
	"agentpal/internal/types"
	"github.com/pelletier/go-toml/v2"
)

type adminProfileRequest struct {
	Variables      map[string]string `json:"variables"`
	AuthOverride   json.RawMessage   `json:"authOverride,omitempty"`
	ConfigOverride string            `json:"configOverride,omitempty"`
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
	token := requestBearer(r)
	if token != "" {
		sub, name, displayName, email, err := ParseJWTUserInfo(token)
		if err == nil && sub != "" {
			subject, status, message := s.adminSubject(r)
			if status == 0 && subject == sub {
				writeJSON(w, map[string]any{"subject": sub, "name": name, "displayName": displayName, "email": email, "admin": true})
				return
			}
			if status != 0 {
				http.Error(w, message, status)
				return
			}
		}
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
		detail := profileDetail(profile)
		if settings, settingsErr := s.settings.get(); settingsErr == nil {
			detail.Variables = resolveStoredVariables(settings, profile.Meta.Variables, detail.Auth)
		}
		writeJSON(w, detail)
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
	settings, err := s.settings.get()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
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
	var existing map[string]any
	var existingVariables map[string]string
	if current, readErr := s.userProfiles.read(sub, name); readErr == nil {
		_ = json.Unmarshal(current.AuthJSON, &existing)
		existingVariables = current.Meta.Variables
	}
	if existingVariables != nil {
		if req.Variables == nil {
			req.Variables = map[string]string{}
		}
		for _, variable := range settings.Variables {
			if !variable.Secret {
				continue
			}
			if value, ok := existingVariables[variable.Name]; ok {
				if _, submitted := req.Variables[variable.Name]; !submitted || strings.TrimSpace(req.Variables[variable.Name]) == "" {
					req.Variables[variable.Name] = value
				}
			}
		}
	}
	if req.Variables == nil {
		req.Variables = map[string]string{}
	}
	rendered, err := renderReservedProfile(settings, req.Variables, existing)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if rendered.Name != name {
		http.Error(w, "profile_name cannot change an existing profile", http.StatusBadRequest)
		return
	}
	authJSON := rendered.AuthJSON
	configTOML := rendered.ConfigTOML
	if len(req.AuthOverride) > 0 {
		var testAuth map[string]any
		if err := json.Unmarshal(req.AuthOverride, &testAuth); err != nil {
			http.Error(w, "authOverride is not valid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		authJSON = req.AuthOverride
	}
	if strings.TrimSpace(req.ConfigOverride) != "" {
		var testConfig map[string]any
		if err := toml.Unmarshal([]byte(req.ConfigOverride), &testConfig); err != nil {
			http.Error(w, "configOverride is not valid TOML: "+err.Error(), http.StatusBadRequest)
			return
		}
		configTOML = []byte(req.ConfigOverride)
	}
	meta := reservedMeta{Name: rendered.Name, Type: rendered.Type, Provider: rendered.Provider, Variables: nonSecretVariables(settings, req.Variables)}
	if err := s.userProfiles.write(sub, name, meta, authJSON, configTOML); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	saved, err := s.userProfiles.read(sub, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, profileDetail(saved))
}

func nonSecretVariables(settings types.ReservedSettings, values map[string]string) map[string]string {
	result := map[string]string{}
	for _, variable := range settings.Variables {
		if !variable.Secret {
			if value, ok := values[variable.Name]; ok {
				result[variable.Name] = value
			}
		}
	}
	return result
}

func (s *Server) adminApplyReserved(w http.ResponseWriter, r *http.Request, sub string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	var request struct {
		Variables      map[string]string `json:"variables"`
		AuthOverride   json.RawMessage   `json:"authOverride,omitempty"`
		ConfigOverride string            `json:"configOverride,omitempty"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(request.Variables) == 0 {
		http.Error(w, "variables are required", http.StatusBadRequest)
		return
	}
	settings, err := s.settings.get()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	profile, err := renderReservedProfile(settings, request.Variables, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if profile.Name == "" {
		http.Error(w, "profile template could not be rendered", http.StatusBadRequest)
		return
	}
	authJSON := profile.AuthJSON
	configTOML := profile.ConfigTOML
	if len(request.AuthOverride) > 0 {
		var testAuth map[string]any
		if err := json.Unmarshal(request.AuthOverride, &testAuth); err != nil {
			http.Error(w, "authOverride is not valid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		authJSON = request.AuthOverride
	}
	if strings.TrimSpace(request.ConfigOverride) != "" {
		var testConfig map[string]any
		if err := toml.Unmarshal([]byte(request.ConfigOverride), &testConfig); err != nil {
			http.Error(w, "configOverride is not valid TOML: "+err.Error(), http.StatusBadRequest)
			return
		}
		configTOML = []byte(request.ConfigOverride)
	}
	meta := reservedMeta{Name: profile.Name, Type: profile.Type, Provider: profile.Provider, Variables: nonSecretVariables(settings, request.Variables)}
	if err := s.userProfiles.write(sub, profile.Name, meta, authJSON, configTOML); err != nil {
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
