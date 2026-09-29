package central

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"agentpal/internal/types"
)

var envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var providerPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type settingsStore struct{ path string }

func newSettingsStore(root string) *settingsStore {
	return &settingsStore{path: filepath.Join(root, "settings.json")}
}

func defaultReservedSettings() types.ReservedSettings {
	return types.ReservedSettings{
		Provider: "memory",
		BaseURL:  "https://memory.yprenewables.com/codex/default",
		WireAPI:  "responses",
		EnvKey:   "OPENAI_API_KEY",
	}
}

func normalizeReservedSettings(settings types.ReservedSettings) types.ReservedSettings {
	defaults := defaultReservedSettings()
	settings.Provider = strings.TrimSpace(settings.Provider)
	settings.BaseURL = strings.TrimSpace(settings.BaseURL)
	settings.WireAPI = strings.TrimSpace(settings.WireAPI)
	settings.EnvKey = strings.TrimSpace(settings.EnvKey)
	if settings.Provider == "" {
		settings.Provider = defaults.Provider
	}
	if settings.BaseURL == "" {
		settings.BaseURL = defaults.BaseURL
	}
	if settings.WireAPI == "" {
		settings.WireAPI = defaults.WireAPI
	}
	if settings.EnvKey == "" {
		settings.EnvKey = defaults.EnvKey
	}
	return settings
}

func validateReservedSettings(settings types.ReservedSettings) error {
	if !providerPattern.MatchString(settings.Provider) {
		return errors.New("provider must match [A-Za-z0-9_-]")
	}
	parsed, err := url.Parse(settings.BaseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return errors.New("baseURL must be an absolute http(s) URL")
	}
	if strings.TrimSpace(settings.WireAPI) == "" {
		return errors.New("wireAPI is required")
	}
	if !envKeyPattern.MatchString(settings.EnvKey) {
		return errors.New("envKey must be a valid environment variable name")
	}
	return nil
}

func (s *settingsStore) get() (types.ReservedSettings, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return defaultReservedSettings(), nil
	}
	if err != nil {
		return types.ReservedSettings{}, err
	}
	settings := defaultReservedSettings()
	if err := json.Unmarshal(data, &settings); err != nil {
		return types.ReservedSettings{}, err
	}
	return normalizeReservedSettings(settings), nil
}

func (s *settingsStore) put(settings types.ReservedSettings) error {
	settings = normalizeReservedSettings(settings)
	if err := validateReservedSettings(settings); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(s.path, data, 0o600)
}
