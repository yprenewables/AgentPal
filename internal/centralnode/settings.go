package centralnode

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"agentpal/internal/types"
	"github.com/pelletier/go-toml/v2"
)

var variablePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var placeholderPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

type settingsStore struct{ path string }

func newSettingsStore(root string) *settingsStore {
	return &settingsStore{path: filepath.Join(root, "settings.json")}
}

func defaultReservedSettings() types.ReservedSettings {
	return types.ReservedSettings{
		ProfileTemplate: "model_provider = \"${provider}\"\n\n[model_providers.${provider}]\nname = \"${provider}\"\nbase_url = \"${base_url}\"\nenv_key = \"${env_key}\"\nwire_api = \"${wire_api}\"\n",
		AuthTemplate:    map[string]any{"${env_key}": "${api_key}"},
		Variables: []types.TemplateVariable{
			{Name: "provider", Label: "Provider 名称", Default: "memory", Required: true},
			{Name: "base_url", Label: "Base URL", Default: "https://memory.yprenewables.com/codex/default", Required: true},
			{Name: "env_key", Label: "环境变量名", Default: "OPENAI_API_KEY", Required: true},
			{Name: "wire_api", Label: "Wire API", Default: "responses", Required: true},
			{Name: "api_key", Label: "API Key", Required: true, Secret: true},
		},
	}
}

func normalizeReservedSettings(settings types.ReservedSettings) types.ReservedSettings {
	settings.ProfileTemplate = strings.TrimSpace(settings.ProfileTemplate)
	return settings
}

func validateReservedSettings(settings types.ReservedSettings) error {
	if settings.ProfileTemplate == "" {
		return errors.New("profileTemplate is required")
	}
	if len(settings.AuthTemplate) == 0 {
		return errors.New("authTemplate is required")
	}
	testValues := map[string]string{}
	for _, variable := range settings.Variables {
		value := variable.Default
		if value == "" {
			value = "template_value"
		}
		testValues[variable.Name] = value
	}
	var config map[string]any
	if err := toml.Unmarshal([]byte(substituteTemplate(settings.ProfileTemplate, testValues)), &config); err != nil {
		return errors.New("profileTemplate is invalid TOML: " + err.Error())
	}
	seen := map[string]bool{}
	for _, variable := range settings.Variables {
		if !variablePattern.MatchString(variable.Name) || seen[variable.Name] {
			return errors.New("template variable names must be unique and match [A-Za-z_][A-Za-z0-9_]*")
		}
		seen[variable.Name] = true
	}
	for _, template := range []string{settings.ProfileTemplate, authTemplateText(settings.AuthTemplate)} {
		for _, match := range placeholderPattern.FindAllStringSubmatch(template, -1) {
			if !seen[match[1]] {
				return errors.New("template references undeclared variable: " + match[1])
			}
		}
	}
	return nil
}

func authTemplateText(template map[string]any) string {
	data, _ := json.Marshal(template)
	return string(data)
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
