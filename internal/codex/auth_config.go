package codex

import (
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

var authRootKeys = []string{"model_provider"}

type AuthSummary struct {
	Provider string `json:"provider"`
}

func ExtractAuthConfig(path string) ([]byte, AuthSummary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, AuthSummary{}, err
	}
	var root map[string]any
	if err := toml.Unmarshal(data, &root); err != nil {
		return nil, AuthSummary{}, err
	}
	summary := AuthSummary{Provider: stringValue(root["model_provider"])}
	if summary.Provider == "" {
		return nil, summary, errors.New("model_provider is required")
	}
	sections := splitConfig(data)
	rootPart := filterRootAssignments(sections.root, stringSet(authRootKeys))
	prefix := "model_providers." + summary.Provider
	var provider []byte
	for _, section := range sections.tables {
		if tableOwnedBy(section.name, prefix) {
			provider = append(provider, section.bytes...)
		}
	}
	if len(provider) == 0 {
		return nil, summary, errors.New("model provider table was not found: " + summary.Provider)
	}
	return joinSections([][]byte{rootPart, ensureProviderName(provider, summary.Provider)}), summary, nil
}

func MergeAuthConfig(path string, fragment []byte) error {
	var root map[string]any
	if err := toml.Unmarshal(fragment, &root); err != nil {
		return err
	}
	provider := stringValue(root["model_provider"])
	if provider == "" {
		return errors.New("model_provider is required")
	}
	target, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		target = nil
	} else if err != nil {
		return err
	}
	fragment = ensureProviderName(fragment, provider)
	source := splitConfig(fragment)
	profileRootKeys := map[string]bool{"model_provider": true}
	mergedRoot := replaceRootAssignments(targetRoot(target), profileRootKeys, filterRootAssignments(source.root, profileRootKeys))
	targetSections := splitConfig(target)
	var base [][]byte
	if len(strings.TrimSpace(string(mergedRoot))) > 0 {
		base = append(base, mergedRoot)
	}
	prefix := "model_providers." + provider
	matchedProvider := false
	for _, section := range targetSections.tables {
		if section.name == prefix {
			base = append(base, ensureProviderName(section.bytes, provider))
			matchedProvider = true
			continue
		}
		base = append(base, section.bytes)
	}
	if !matchedProvider {
		for _, section := range source.tables {
			if tableOwnedBy(section.name, prefix) {
				base = append(base, section.bytes)
			}
		}
	}
	return atomicWrite(path, joinSections(base), 0o600)
}

// EnsureProviderBearerToken adds the Memory Codex gateway credential to the
// provider table when the saved auth key is the gateway's user_key format.
func EnsureProviderBearerToken(configPath string, authData []byte) error {
	var auth map[string]any
	if err := json.Unmarshal(authData, &auth); err != nil {
		return err
	}
	token, _ := auth["OPENAI_API_KEY"].(string)
	if !strings.HasPrefix(token, "sk-mem-") {
		return nil
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	var root map[string]any
	if err := toml.Unmarshal(data, &root); err != nil {
		return err
	}
	provider := stringValue(root["model_provider"])
	if provider == "" {
		return errors.New("model_provider is required")
	}
	sections := splitConfig(data)
	prefix := "model_providers." + provider
	for index, section := range sections.tables {
		if section.name != prefix || strings.Contains(string(section.bytes), "experimental_bearer_token") {
			continue
		}
		section.bytes = append(section.bytes, []byte("experimental_bearer_token = "+strconv.Quote(token)+"\n")...)
		sections.tables[index] = section
		return atomicWrite(configPath, joinParsedSections(sections), 0o600)
	}
	return errors.New("model provider table was not found: " + provider)
}

func EnsureProviderBearerTokenInFragment(fragment, authData []byte) ([]byte, error) {
	var auth map[string]any
	if err := json.Unmarshal(authData, &auth); err != nil {
		return nil, err
	}
	token, _ := auth["OPENAI_API_KEY"].(string)
	if !strings.HasPrefix(token, "sk-mem-") {
		return fragment, nil
	}
	var root map[string]any
	if err := toml.Unmarshal(fragment, &root); err != nil {
		return nil, err
	}
	provider := stringValue(root["model_provider"])
	if provider == "" {
		return nil, errors.New("model_provider is required")
	}
	sections := splitConfig(fragment)
	prefix := "model_providers." + provider
	for index, section := range sections.tables {
		if section.name != prefix {
			continue
		}
		lines := strings.SplitAfter(string(section.bytes), "\n")
		bearerLine := "experimental_bearer_token = " + strconv.Quote(token) + "\n"
		for lineIndex, line := range lines[1:] {
			if strings.HasPrefix(strings.TrimSpace(line), "experimental_bearer_token") {
				lines[lineIndex+1] = bearerLine
				section.bytes = []byte(strings.Join(lines, ""))
				sections.tables[index] = section
				return joinParsedSections(sections), nil
			}
		}
		section.bytes = append(section.bytes, []byte(bearerLine)...)
		sections.tables[index] = section
		return joinParsedSections(sections), nil
	}
	return nil, errors.New("model provider table was not found: " + provider)
}

func CompareProviderConfig(path string, fragment []byte) (bool, string, error) {
	var root map[string]any
	if err := toml.Unmarshal(fragment, &root); err != nil {
		return false, "profile config is invalid", err
	}
	provider := stringValue(root["model_provider"])
	if provider == "" {
		return false, "model_provider is required", errors.New("model_provider is required")
	}
	fragment = ensureProviderName(fragment, provider)
	profileSections := splitConfig(fragment)
	profileProvider := providerSections(profileSections, "model_providers."+provider)
	if len(profileProvider) == 0 {
		return false, "profile Provider table is missing", errors.New("model provider table was not found: " + provider)
	}
	target, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return true, "local config.toml is missing", nil
	}
	if err != nil {
		return false, "cannot read local config.toml", err
	}
	targetProvider := providerSections(splitConfig(target), "model_providers."+provider)
	if len(targetProvider) == 0 {
		return true, "local Provider table is missing: " + provider, nil
	}
	profileCanonical, err := canonicalTOML(profileProvider)
	if err != nil {
		return false, "profile Provider table is invalid", err
	}
	targetCanonical, err := canonicalTOML(ensureProviderName(targetProvider, provider))
	if err != nil {
		return true, "local Provider table is invalid: " + provider, nil
	}
	if string(profileCanonical) != string(targetCanonical) {
		return true, "local Provider table differs from the profile: " + provider, nil
	}
	return false, "", nil
}

func providerSections(config parsedConfig, prefix string) []byte {
	var result []byte
	for _, section := range config.tables {
		if tableOwnedBy(section.name, prefix) {
			result = append(result, section.bytes...)
		}
	}
	return result
}

func canonicalTOML(data []byte) ([]byte, error) {
	var value map[string]any
	if err := toml.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	return toml.Marshal(value)
}

func targetRoot(data []byte) []byte { return splitConfig(data).root }
func stringValue(value any) string  { text, _ := value.(string); return text }

func ensureProviderName(data []byte, provider string) []byte {
	sections := splitConfig(data)
	prefix := "model_providers." + provider
	for index, section := range sections.tables {
		if section.name != prefix {
			continue
		}
		lines := strings.SplitAfter(string(section.bytes), "\n")
		nameLine := "name = " + strconv.Quote(provider) + "\n"
		for lineIndex, line := range lines[1:] {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "name") && strings.Contains(trimmed, "=") {
				lines[lineIndex+1] = nameLine
				section.bytes = []byte(strings.Join(lines, ""))
				sections.tables[index] = section
				return joinParsedSections(sections)
			}
		}
		section.bytes = []byte(string(section.bytes) + nameLine)
		sections.tables[index] = section
		return joinParsedSections(sections)
	}
	return data
}

func joinParsedSections(config parsedConfig) []byte {
	parts := make([][]byte, 0, len(config.tables)+1)
	if len(strings.TrimSpace(string(config.root))) > 0 {
		parts = append(parts, config.root)
	}
	for _, section := range config.tables {
		parts = append(parts, section.bytes)
	}
	return joinSections(parts)
}
