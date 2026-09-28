package authprofile

import (
	"agentpal/internal/codex"
	appconfig "agentpal/internal/config"
	"agentpal/internal/types"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

func (s *Store) Preview(codexDir, profileID string) (types.ProfileSwitchPreview, error) {
	p, fragment, err := s.loadProfile(profileID)
	if err != nil {
		return types.ProfileSwitchPreview{}, err
	}
	conflict, description, err := codex.CompareProviderConfig(filepath.Join(codexDir, "config.toml"), fragment)
	if err != nil {
		return types.ProfileSwitchPreview{}, err
	}
	return types.ProfileSwitchPreview{Profile: types.AuthProfile{ID: p.ID, Name: p.Name, Type: p.Type, Provider: p.Provider, Source: p.Source}, Provider: p.Provider, ProviderConflict: conflict, ConflictDescription: description}, nil
}

func (s *Store) Activate(codexDir, profileID string, allowProviderConflict ...bool) (types.ProfileSwitchResult, error) {
	dir := filepath.Join(s.Root, profileID)
	p, fragment, err := s.loadProfile(profileID)
	if err != nil {
		return types.ProfileSwitchResult{}, err
	}
	conflict, description, err := codex.CompareProviderConfig(filepath.Join(codexDir, "config.toml"), fragment)
	if err != nil {
		return types.ProfileSwitchResult{}, err
	}
	if conflict && (len(allowProviderConflict) == 0 || !allowProviderConflict[0]) {
		return types.ProfileSwitchResult{Profile: types.AuthProfile{ID: p.ID, Name: p.Name, Type: p.Type, Provider: p.Provider, Source: p.Source}, ProviderConflict: true, ConflictDescription: description}, nil
	}
	auth, err := os.ReadFile(filepath.Join(dir, "auth.json"))
	if err != nil {
		return types.ProfileSwitchResult{}, err
	}
	fragment, err = codex.EnsureProviderBearerTokenInFragment(fragment, auth)
	if err != nil {
		return types.ProfileSwitchResult{}, err
	}
	var parsed any
	if err := json.Unmarshal(auth, &parsed); err != nil {
		return types.ProfileSwitchResult{}, err
	}
	root, err := appconfig.BackupRoot()
	if err != nil {
		return types.ProfileSwitchResult{}, err
	}
	backup := filepath.Join(root, time.Now().Format("20060102-150405.000-auth"))
	if err := os.MkdirAll(backup, 0o700); err != nil {
		return types.ProfileSwitchResult{}, err
	}
	if err := copyIfExists(filepath.Join(codexDir, "auth.json"), filepath.Join(backup, "auth.json")); err != nil {
		return types.ProfileSwitchResult{}, err
	}
	if err := copyIfExists(filepath.Join(codexDir, "config.toml"), filepath.Join(backup, "config.toml")); err != nil {
		return types.ProfileSwitchResult{}, err
	}
	if err := os.WriteFile(filepath.Join(codexDir, "auth.json.agentpal.tmp"), auth, 0o600); err != nil {
		return types.ProfileSwitchResult{}, err
	}
	if err := os.Rename(filepath.Join(codexDir, "auth.json.agentpal.tmp"), filepath.Join(codexDir, "auth.json")); err != nil {
		return types.ProfileSwitchResult{}, err
	}
	if err := codex.MergeAuthConfig(filepath.Join(codexDir, "config.toml"), fragment); err != nil {
		_ = restoreFile(filepath.Join(backup, "auth.json"), filepath.Join(codexDir, "auth.json"))
		_ = restoreFile(filepath.Join(backup, "config.toml"), filepath.Join(codexDir, "config.toml"))
		return types.ProfileSwitchResult{}, err
	}
	if err := codex.EnsureProviderBearerToken(filepath.Join(codexDir, "config.toml"), auth); err != nil {
		_ = restoreFile(filepath.Join(backup, "auth.json"), filepath.Join(codexDir, "auth.json"))
		_ = restoreFile(filepath.Join(backup, "config.toml"), filepath.Join(codexDir, "config.toml"))
		return types.ProfileSwitchResult{}, err
	}
	if err := os.WriteFile(filepath.Join(s.Root, "active.json"), []byte(p.ID), 0o600); err != nil {
		return types.ProfileSwitchResult{}, err
	}
	return types.ProfileSwitchResult{OK: true, Profile: types.AuthProfile{ID: p.ID, Name: p.Name, Type: p.Type, Provider: p.Provider, Source: p.Source, Active: true}, BackupPath: backup}, nil
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
func copyIfExists(src, dst string) error {
	data, err := os.ReadFile(src)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o600)
}

func restoreFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if os.IsNotExist(err) {
		return os.Remove(dst)
	}
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o600)
}
