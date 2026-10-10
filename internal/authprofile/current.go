package authprofile

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

type CurrentStatus struct {
	Provider         string `json:"provider,omitempty"`
	APIKeyPreview    string `json:"apiKeyPreview,omitempty"`
	BaseURL          string `json:"baseUrl,omitempty"`
	ChatGPTUserID    string `json:"chatgptUserId,omitempty"`
	ChatGPTExpiresAt string `json:"chatgptExpiresAt,omitempty"`
	APIKeyImported   bool   `json:"apiKeyImported"`
	ChatGPTImported  bool   `json:"chatgptImported"`
	CurrentProfileID string `json:"currentProfileId,omitempty"`
}

func InspectCurrent(codexDir string, profiles []AuthProfile) (CurrentStatus, error) {
	authData, err := os.ReadFile(filepath.Join(codexDir, "auth.json"))
	if err != nil && !os.IsNotExist(err) {
		return CurrentStatus{}, err
	}
	var auth any
	if len(authData) > 0 {
		if err := json.Unmarshal(authData, &auth); err != nil {
			return CurrentStatus{}, err
		}
	}
	configData, err := os.ReadFile(filepath.Join(codexDir, "config.toml"))
	if err != nil && !os.IsNotExist(err) {
		return CurrentStatus{}, err
	}
	var config map[string]any
	if len(configData) > 0 {
		if err := toml.Unmarshal(configData, &config); err != nil {
			return CurrentStatus{}, err
		}
	}
	key := findAPIKey(auth)
	if key == "" {
		key = findAPIKey(config)
	}
	userID := findUserID(auth)
	status := CurrentStatus{Provider: providerFromConfig(config), APIKeyPreview: maskAPIKey(key), ChatGPTUserID: userID, ChatGPTExpiresAt: findExpiresAt(auth), BaseURL: baseURLFromConfig(config)}
	if key != "" {
		for _, profile := range profiles {
			if profile.Type == "api_key" && profile.APIKeyPreview == maskAPIKey(key) && strings.EqualFold(profile.Provider, providerFromConfig(config)) && (profile.BaseURL == "" || profile.BaseURL == status.BaseURL) {
				status.APIKeyImported = true
				status.CurrentProfileID = profile.ID
				break
			}
		}
	} else if userID != "" {
		for _, profile := range profiles {
			if profile.Type == "chatgpt" && profile.ChatGPTUserID == userID {
				status.ChatGPTImported = true
				status.CurrentProfileID = profile.ID
				break
			}
		}
	}
	return status, nil
}

func (s *Store) ImportCurrentAPIKey(codexDir, name string) (AuthProfile, error) {
	data, err := os.ReadFile(filepath.Join(codexDir, "auth.json"))
	if err != nil && !os.IsNotExist(err) {
		return AuthProfile{}, err
	}
	var auth any
	if len(data) > 0 {
		if err := json.Unmarshal(data, &auth); err != nil {
			return AuthProfile{}, err
		}
	}
	configData, err := os.ReadFile(filepath.Join(codexDir, "config.toml"))
	if err != nil {
		return AuthProfile{}, err
	}
	var config map[string]any
	if err := toml.Unmarshal(configData, &config); err != nil {
		return AuthProfile{}, err
	}
	key := findAPIKey(auth)
	if key == "" {
		key = findAPIKey(config)
	}
	if key == "" {
		return AuthProfile{}, errors.New("current API key was not found")
	}
	provider := providerFromConfig(config)
	providerConfig := providerTable(config, provider)
	baseURL, _ := providerConfig["base_url"].(string)
	return s.CreateAPIKey(APIKeyRequest{Name: name, Provider: provider, BaseURL: baseURL, APIKey: key})
}

func findAPIKey(value any) string {
	var found string
	walkJSON(value, func(key string, value any) bool {
		if text, ok := value.(string); ok && (strings.EqualFold(key, "api_key") || strings.EqualFold(key, "openai_api_key") || strings.HasPrefix(text, "sk-")) {
			found = text
			return true
		}
		return false
	})
	return found
}
func findUserID(value any) string {
	var found string
	walkJSON(value, func(key string, value any) bool {
		if text, ok := value.(string); ok && (key == "user_id" || key == "userId") {
			found = text
			return true
		}
		if text, ok := value.(string); ok && strings.Contains(text, ".") {
			if subject := jwtSubject(text); subject != "" {
				found = subject
				return true
			}
		}
		return false
	})
	return found
}
func findExpiresAt(value any) string {
	var found string
	walkJSON(value, func(_ string, value any) bool {
		text, ok := value.(string)
		if !ok || !strings.Contains(text, ".") {
			return false
		}
		_, expires := jwtClaims(text)
		if expires > 0 {
			found = time.Unix(expires, 0).UTC().Format(time.RFC3339)
			return true
		}
		return false
	})
	return found
}
func walkJSON(value any, visit func(string, any) bool) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if visit(key, item) || walkJSON(item, visit) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if walkJSON(item, visit) {
				return true
			}
		}
	}
	return false
}
func jwtSubject(token string) string {
	subject, _ := jwtClaims(token)
	return subject
}
func jwtClaims(token string) (string, int64) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", 0
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", 0
	}
	var payload map[string]any
	if json.Unmarshal(data, &payload) != nil {
		return "", 0
	}
	subject, _ := payload["sub"].(string)
	var expires int64
	switch value := payload["exp"].(type) {
	case float64:
		expires = int64(value)
	case string:
		expires, _ = strconv.ParseInt(value, 10, 64)
	}
	return subject, expires
}
func maskAPIKey(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "••••"
	}
	return value[:3] + "••••••" + value[len(value)-4:]
}
func providerFromConfig(config map[string]any) string {
	provider, _ := config["model_provider"].(string)
	return provider
}

func baseURLFromConfig(config map[string]any) string {
	provider := providerFromConfig(config)
	value, _ := providerTable(config, provider)["base_url"].(string)
	return value
}
func providerTable(config map[string]any, provider string) map[string]any {
	models, _ := config["model_providers"].(map[string]any)
	table, _ := models[provider].(map[string]any)
	return table
}
