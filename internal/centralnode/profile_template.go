package centralnode

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"agentpal/internal/authprofile"
	"agentpal/internal/types"
)

var templatePlaceholder = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func resolveTemplateValues(settings types.ReservedSettings, supplied map[string]string, existing map[string]any) (map[string]string, error) {
	values := make(map[string]string, len(settings.Variables))
	for _, variable := range settings.Variables {
		value, suppliedValue := supplied[variable.Name]
		if suppliedValue {
			value = strings.TrimSpace(value)
		} else {
			value = variable.Default
		}
		if variable.Secret && value == "" && existing != nil {
			value = existingSecretValue(settings, values, variable.Name, existing)
		}
		if value == "" && variable.Required {
			return nil, fmt.Errorf("variable %q is required", variable.Name)
		}
		values[variable.Name] = value
	}
	return values, nil
}

// resolveStoredVariables 还原预留 profile 的全部模板变量（含 secret），供管理后台明文回显。
// 非 secret 变量优先取已存元数据，secret 变量从已存 auth 内容按模板反向还原。
func resolveStoredVariables(settings types.ReservedSettings, stored map[string]string, auth map[string]any) map[string]string {
	values := make(map[string]string, len(settings.Variables))
	for _, variable := range settings.Variables {
		if variable.Secret {
			continue
		}
		if value, ok := stored[variable.Name]; ok && value != "" {
			values[variable.Name] = value
			continue
		}
		values[variable.Name] = variable.Default
	}
	for _, variable := range settings.Variables {
		if !variable.Secret {
			continue
		}
		if value, ok := stored[variable.Name]; ok && value != "" {
			values[variable.Name] = value
			continue
		}
		if auth != nil {
			values[variable.Name] = existingSecretValue(settings, values, variable.Name, auth)
		}
	}
	return values
}

func existingSecretValue(settings types.ReservedSettings, values map[string]string, name string, existing map[string]any) string {
	resolved := make(map[string]string, len(values))
	for key, value := range values {
		resolved[key] = value
	}
	for _, variable := range settings.Variables {
		if _, ok := resolved[variable.Name]; !ok {
			resolved[variable.Name] = variable.Default
		}
	}
	for keyTemplate, valueTemplate := range settings.AuthTemplate {
		key := substituteTemplate(keyTemplate, resolved)
		if key == "" {
			continue
		}
		if raw, ok := existing[key]; ok {
			if text, ok := raw.(string); ok && strings.Contains(fmt.Sprint(valueTemplate), "${"+name+"}") {
				return text
			}
		}
	}
	return ""
}

func substituteTemplate(input string, values map[string]string) string {
	return templatePlaceholder.ReplaceAllStringFunc(input, func(match string) string {
		name := templatePlaceholder.FindStringSubmatch(match)[1]
		return values[name]
	})
}

func renderTemplateValue(value any, values map[string]string) any {
	switch typed := value.(type) {
	case string:
		return substituteTemplate(typed, values)
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			result[substituteTemplate(key, values)] = renderTemplateValue(item, values)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for i, item := range typed {
			result[i] = renderTemplateValue(item, values)
		}
		return result
	default:
		return value
	}
}

func renderReservedProfile(settings types.ReservedSettings, supplied map[string]string, existing map[string]any) (authprofile.CentralProfile, error) {
	values, err := resolveTemplateValues(settings, supplied, existing)
	if err != nil {
		return authprofile.CentralProfile{}, err
	}
	name := values["profile_name"]
	if name == "" {
		name = values["provider"]
	}
	if err := authprofile.ValidateReservedName(name); err != nil {
		return authprofile.CentralProfile{}, errors.New("profile name is invalid: " + err.Error())
	}
	authValue := renderTemplateValue(settings.AuthTemplate, values)
	authJSON, err := json.Marshal(authValue)
	if err != nil {
		return authprofile.CentralProfile{}, err
	}
	config := substituteTemplate(settings.ProfileTemplate, values)
	return authprofile.CentralProfile{Name: name, Provider: values["provider"], Type: "api_key", AuthJSON: authJSON, ConfigTOML: []byte(config)}, nil
}
