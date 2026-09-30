package central

import (
	"strings"
	"testing"

	"agentpal/internal/types"
)

func templateSettings() types.ReservedSettings {
	return types.ReservedSettings{
		ProfileTemplate: "model_provider = \"${provider}\"\nbase_url = \"${base_url}\"\n",
		AuthTemplate:    map[string]any{"${env_key}": "${api_key}"},
		Variables: []types.TemplateVariable{
			{Name: "provider", Required: true},
			{Name: "base_url", Default: "https://example.test/v1", Required: true},
			{Name: "env_key", Default: "OPENAI_API_KEY", Required: true},
			{Name: "api_key", Required: true, Secret: true},
		},
	}
}

func TestRenderReservedProfileUsesDefaultsAndVariables(t *testing.T) {
	profile, err := renderReservedProfile(templateSettings(), map[string]string{"provider": "corp", "api_key": "sk-test"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Name != "corp" || !strings.Contains(string(profile.ConfigTOML), "https://example.test/v1") {
		t.Fatalf("unexpected rendered profile: %#v", profile)
	}
	if !strings.Contains(string(profile.AuthJSON), "OPENAI_API_KEY") || !strings.Contains(string(profile.AuthJSON), "sk-test") {
		t.Fatalf("unexpected rendered auth: %s", profile.AuthJSON)
	}
}

func TestRenderReservedProfilePreservesSecretOnEdit(t *testing.T) {
	settings := templateSettings()
	profile, err := renderReservedProfile(settings, map[string]string{"provider": "corp"}, map[string]any{"OPENAI_API_KEY": "sk-existing"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(profile.AuthJSON), "sk-existing") {
		t.Fatalf("secret was not preserved: %s", profile.AuthJSON)
	}
}

func TestRenderReservedProfileRejectsMissingRequiredVariable(t *testing.T) {
	_, err := renderReservedProfile(templateSettings(), map[string]string{"api_key": "sk-test"}, nil)
	if err == nil || !strings.Contains(err.Error(), "provider") {
		t.Fatalf("expected provider validation error, got %v", err)
	}
}
