package main

import (
	"testing"

	"agentpal/internal/authprofile"
	"agentpal/internal/types"
)

func TestDisconnectedRefreshHasEmptyItems(t *testing.T) {
	got := disconnectedRefresh("connection refused")
	if got.Connected {
		t.Fatal("disconnected refresh reported connected")
	}
	if got.Diff.Items == nil {
		t.Fatal("disconnected refresh items must serialize as [] instead of null")
	}
}

func TestProfileTypeKeepsEachSavedProfileConfiguration(t *testing.T) {
	profile := authprofile.AuthProfile{ID: "profile-1", Name: "Saved", Type: "api_key", Provider: "saved-provider", APIKeyPreview: "sk-••••1234", BaseURL: "https://saved.example/v1"}
	got := profileType(profile)
	if got.Provider != "saved-provider" || got.APIKeyPreview != "sk-••••1234" || got.BaseURL != "https://saved.example/v1" {
		t.Fatalf("profile mapping lost saved config: %#v", got)
	}
}

func TestProfileListDoesNotOverwriteSavedProfilesWithCurrentAuth(t *testing.T) {
	profiles := []authprofile.AuthProfile{
		{ID: "active", Name: "Active", Type: "api_key", Provider: "saved-active", APIKeyPreview: "sk-••••aaaa", BaseURL: "https://active.example/v1"},
		{ID: "other", Name: "Other", Type: "api_key", Provider: "saved-other", APIKeyPreview: "sk-••••bbbb", BaseURL: "https://other.example/v1"},
	}
	current := authprofile.CurrentStatus{CurrentProfileID: "active", APIKeyImported: true, APIKeyPreview: "sk-••••live", BaseURL: "https://live.example/v1"}
	got := profileTypesFromStore(profiles, current)
	if len(got) != 2 {
		t.Fatalf("profiles=%#v", got)
	}
	if !got[0].Active || got[0].APIKeyPreview != "sk-••••aaaa" || got[0].BaseURL != "https://active.example/v1" {
		t.Fatalf("active profile did not retain its saved snapshot: %#v", got[0])
	}
	if got[1].Active || got[1].Provider != "saved-other" || got[1].APIKeyPreview != "sk-••••bbbb" || got[1].BaseURL != "https://other.example/v1" {
		t.Fatalf("other profile was overwritten by current auth: %#v", got[1])
	}
}

func TestFindLocalByReservedName(t *testing.T) {
	profiles := []authprofile.AuthProfile{
		{ID: "a", Name: "Work", Provider: "openai"},
		{ID: "b", Name: "yangpu", ReservedName: "yangpu"},
	}
	found, ok := findLocalReserved(profiles, "yangpu")
	if !ok || found.ID != "b" {
		t.Fatalf("found=%#v ok=%v", found, ok)
	}
	if _, ok := findLocalReserved(profiles, "missing"); ok {
		t.Fatal("unexpected match")
	}
	if _, ok := findLocalReserved(profiles, "openai"); ok {
		t.Fatal("must not match by provider")
	}
}

func TestCentralProfileFromDetail(t *testing.T) {
	detail := types.RemoteAuthProfileDetail{Name: "yangpu", Provider: "yangpu", Type: "api_key", Auth: map[string]any{"OPENAI_API_KEY": "sk-1"}, ConfigAuthToml: "model_provider = \"yangpu\"\n"}
	profile := centralProfileFrom(detail)
	if profile.Name != "yangpu" || profile.Provider != "yangpu" || string(profile.AuthJSON) == "" || string(profile.ConfigTOML) == "" {
		t.Fatalf("profile=%#v", profile)
	}
}
