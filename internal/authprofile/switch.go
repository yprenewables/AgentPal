package authprofile

import (
	"agentpal/internal/codex"
	"agentpal/internal/types"
	"encoding/json"
	"os"
	"path/filepath"
)

func (s *Store) Preview(codexDir, profileID string) (types.ProfileSwitchPreview, error) {
	p, _, fragment, err := s.loadProfileForActivation(profileID)
	if err != nil {
		return types.ProfileSwitchPreview{}, err
	}
	conflict, description, profileProvider, localProvider, err := codex.CompareProviderConfig(filepath.Join(codexDir, "config.toml"), fragment)
	if err != nil {
		return types.ProfileSwitchPreview{}, err
	}
	return types.ProfileSwitchPreview{Profile: types.AuthProfile{ID: p.ID, Name: p.Name, Type: p.Type, Provider: p.Provider, Source: p.Source}, Provider: p.Provider, ProviderConflict: conflict, ConflictDescription: description, ProfileProvider: profileProvider, LocalProvider: localProvider}, nil
}

func (s *Store) Activate(codexDir, profileID string, allowProviderConflict ...bool) (types.ProfileSwitchResult, error) {
	reconcileMu.Lock()
	defer reconcileMu.Unlock()
	return s.activateLocked(codexDir, profileID, allowProviderConflict...)
}

func (s *Store) activateLocked(codexDir, profileID string, allowProviderConflict ...bool) (result types.ProfileSwitchResult, err error) {
	p, auth, fragment, err := s.loadProfileForActivation(profileID)
	if err != nil {
		return types.ProfileSwitchResult{}, err
	}
	conflict, description, profileProvider, localProvider, err := codex.CompareProviderConfig(filepath.Join(codexDir, "config.toml"), fragment)
	if err != nil {
		return types.ProfileSwitchResult{}, err
	}
	if conflict && (len(allowProviderConflict) == 0 || !allowProviderConflict[0]) {
		return types.ProfileSwitchResult{Profile: types.AuthProfile{ID: p.ID, Name: p.Name, Type: p.Type, Provider: p.Provider, Source: p.Source}, ProviderConflict: true, ConflictDescription: description, ProfileProvider: profileProvider, LocalProvider: localProvider}, nil
	}
	if err := s.saveOutgoingModels(codexDir); err != nil {
		return types.ProfileSwitchResult{}, err
	}
	backups, err := backupFiles(filepath.Join(codexDir, "auth.json"), filepath.Join(codexDir, "config.toml"), filepath.Join(codexDir, "models.json"), filepath.Join(s.Root, "active.json"))
	if err != nil {
		return types.ProfileSwitchResult{}, err
	}
	defer func() {
		if err != nil {
			err = restoreFiles(backups, err)
		}
	}()
	if err := os.WriteFile(filepath.Join(codexDir, "auth.json.agentpal.tmp"), auth, 0o600); err != nil {
		return types.ProfileSwitchResult{}, err
	}
	if err := os.Rename(filepath.Join(codexDir, "auth.json.agentpal.tmp"), filepath.Join(codexDir, "auth.json")); err != nil {
		return types.ProfileSwitchResult{}, err
	}
	if err := codex.MergeAuthConfig(filepath.Join(codexDir, "config.toml"), fragment); err != nil {
		return types.ProfileSwitchResult{}, err
	}
	if err := codex.EnsureProviderBearerToken(filepath.Join(codexDir, "config.toml"), auth); err != nil {
		return types.ProfileSwitchResult{}, err
	}
	if err := s.applyProfileModels(codexDir, p.ID); err != nil {
		return types.ProfileSwitchResult{}, err
	}
	if err := os.WriteFile(filepath.Join(s.Root, "active.json"), []byte(p.ID), 0o600); err != nil {
		return types.ProfileSwitchResult{}, err
	}
	return types.ProfileSwitchResult{OK: true, Profile: types.AuthProfile{ID: p.ID, Name: p.Name, Type: p.Type, Provider: p.Provider, Source: p.Source, Active: true}}, nil
}

// ActivateRemote applies a profile fetched directly from the central server without persisting it in the local profile store.
func ActivateRemote(codexDir string, p AuthProfile, auth, fragment []byte, allowProviderConflict bool) (types.ProfileSwitchResult, error) {
	var parsed any
	if err := json.Unmarshal(auth, &parsed); err != nil {
		return types.ProfileSwitchResult{}, err
	}
	fragment, err := codex.EnsureProviderBearerTokenInFragment(fragment, auth)
	if err != nil {
		return types.ProfileSwitchResult{}, err
	}
	conflict, description, profileProvider, localProvider, err := codex.CompareProviderConfig(filepath.Join(codexDir, "config.toml"), fragment)
	if err != nil {
		return types.ProfileSwitchResult{}, err
	}
	if conflict && !allowProviderConflict {
		return types.ProfileSwitchResult{Profile: types.AuthProfile{Name: p.Name, Type: p.Type, Provider: p.Provider, Source: p.Source}, ProviderConflict: true, ConflictDescription: description, ProfileProvider: profileProvider, LocalProvider: localProvider}, nil
	}
	if err := os.WriteFile(filepath.Join(codexDir, "auth.json.agentpal.tmp"), auth, 0o600); err != nil {
		return types.ProfileSwitchResult{}, err
	}
	if err := os.Rename(filepath.Join(codexDir, "auth.json.agentpal.tmp"), filepath.Join(codexDir, "auth.json")); err != nil {
		return types.ProfileSwitchResult{}, err
	}
	configPath := filepath.Join(codexDir, "config.toml")
	if err := codex.MergeAuthConfig(configPath, fragment); err != nil {
		return types.ProfileSwitchResult{}, err
	}
	if err := codex.EnsureProviderBearerToken(configPath, auth); err != nil {
		return types.ProfileSwitchResult{}, err
	}
	return types.ProfileSwitchResult{OK: true, Profile: types.AuthProfile{Name: p.Name, Type: p.Type, Provider: p.Provider, Source: p.Source, Active: true}}, nil
}

func (s *Store) loadProfile(profileID string) (AuthProfile, []byte, error) {
	dir := filepath.Join(s.Root, profileID)
	var p AuthProfile
	data, err := os.ReadFile(filepath.Join(dir, "profile.json"))
	if err != nil {
		return AuthProfile{}, nil, err
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return AuthProfile{}, nil, err
	}
	fragment, err := os.ReadFile(filepath.Join(dir, "config-auth.toml"))
	if err != nil {
		return AuthProfile{}, nil, err
	}
	return p, fragment, nil
}

func (s *Store) loadProfileForActivation(profileID string) (AuthProfile, []byte, []byte, error) {
	p, fragment, err := s.loadProfile(profileID)
	if err != nil {
		return AuthProfile{}, nil, nil, err
	}
	auth, err := os.ReadFile(filepath.Join(s.Root, profileID, "auth.json"))
	if err != nil {
		return AuthProfile{}, nil, nil, err
	}
	var parsed any
	if err := json.Unmarshal(auth, &parsed); err != nil {
		return AuthProfile{}, nil, nil, err
	}
	fragment, err = codex.EnsureProviderBearerTokenInFragment(fragment, auth)
	if err != nil {
		return AuthProfile{}, nil, nil, err
	}
	return p, auth, fragment, nil
}
