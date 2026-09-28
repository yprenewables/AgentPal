package main

import (
	"testing"

	"agentpal/internal/authprofile"
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
