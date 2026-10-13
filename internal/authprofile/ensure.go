package authprofile

import (
	"agentpal/internal/codex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pelletier/go-toml/v2"
)

var reconcileMu sync.Mutex

type currentSnapshot struct {
	name        string
	profileType string
	provider    string
	authJSON    []byte
	configTOML  []byte
	contentHash string
	status      CurrentStatus
}

func readCurrentSnapshot(codexDir string) (currentSnapshot, bool, error) {
	authData, err := os.ReadFile(filepath.Join(codexDir, "auth.json"))
	if err != nil && !os.IsNotExist(err) {
		return currentSnapshot{}, false, err
	}
	var auth any
	if len(authData) > 0 {
		if err := json.Unmarshal(authData, &auth); err != nil {
			return currentSnapshot{}, false, err
		}
	}

	configData, err := os.ReadFile(filepath.Join(codexDir, "config.toml"))
	if err != nil && !os.IsNotExist(err) {
		return currentSnapshot{}, false, err
	}
	var config map[string]any
	if len(configData) > 0 {
		if err := toml.Unmarshal(configData, &config); err != nil {
			return currentSnapshot{}, false, err
		}
	}

	provider := providerFromConfig(config)
	key := findAPIKey(auth)
	if key == "" {
		key = findAPIKey(providerTable(config, provider))
	}
	userID := findUserID(auth)
	if key == "" && userID == "" {
		return currentSnapshot{}, false, nil
	}

	fragment, summary, err := codex.ExtractAuthConfigFromBytes(configData)
	if err != nil {
		return currentSnapshot{}, false, err
	}
	status := CurrentStatus{
		Provider:         summary.Provider,
		APIKeyPreview:    maskAPIKey(key),
		BaseURL:          baseURLFromConfig(config),
		ChatGPTUserID:    userID,
		ChatGPTExpiresAt: findExpiresAt(auth),
	}
	snapshot := currentSnapshot{provider: summary.Provider, configTOML: fragment, status: status}
	if key != "" {
		snapshot.name = summary.Provider
		snapshot.profileType = "api_key"
		snapshot.authJSON, err = json.Marshal(map[string]any{"OPENAI_API_KEY": key})
	} else {
		snapshot.name = userID
		snapshot.profileType = "chatgpt"
		snapshot.authJSON = authData
	}
	if err != nil {
		return currentSnapshot{}, false, err
	}
	snapshot.contentHash, err = ComputeContentHash(snapshot.authJSON, snapshot.configTOML)
	if err != nil {
		return currentSnapshot{}, false, err
	}
	return snapshot, true, nil
}

func exactCurrentStatus(snapshot currentSnapshot, profiles []AuthProfile) CurrentStatus {
	status := snapshot.status
	for _, profile := range profiles {
		if profile.ContentHash != snapshot.contentHash {
			continue
		}
		status.CurrentProfileID = profile.ID
		status.APIKeyImported = snapshot.profileType == "api_key"
		status.ChatGPTImported = snapshot.profileType == "chatgpt"
		break
	}
	return status
}

func (s *Store) ReconcileCurrent(codexDir string) ([]AuthProfile, CurrentStatus, error) {
	return s.reconcileCurrent(codexDir, nil)
}

func (s *Store) reconcileCurrent(codexDir string, afterWrite func()) ([]AuthProfile, CurrentStatus, error) {
	reconcileMu.Lock()
	defer reconcileMu.Unlock()

	for attempt := 0; attempt < 2; attempt++ {
		profiles, err := s.List()
		if err != nil {
			return nil, CurrentStatus{}, err
		}
		for _, profile := range profiles {
			if profile.Source == "central_login" {
				if err := validateCentralNameOwnership(profiles, centralOwnedName(profile), profile.ContentHash); err != nil {
					return nil, CurrentStatus{}, err
				}
			}
		}
		for _, profile := range profiles {
			if profile.Source != "central_login" {
				continue
			}
			profiles, err = s.reconcileCentralName(profile)
			if err != nil {
				return nil, CurrentStatus{}, err
			}
		}
		snapshot, ok, err := readCurrentSnapshot(codexDir)
		if err != nil {
			return nil, CurrentStatus{}, err
		}
		if !ok {
			if err := s.writeActiveProfileID(""); err != nil {
				return nil, CurrentStatus{}, err
			}
			status, err := InspectCurrent(codexDir, profiles)
			return profiles, status, err
		}
		if status := exactCurrentStatus(snapshot, profiles); status.CurrentProfileID != "" {
			if err := s.writeActiveProfileID(status.CurrentProfileID); err != nil {
				return nil, CurrentStatus{}, err
			}
			return profiles, status, nil
		}
		centralStatus, err := s.activeCentralStatus(snapshot, profiles)
		if err != nil {
			return nil, CurrentStatus{}, err
		}
		if centralStatus.CurrentProfileID != "" {
			return profiles, centralStatus, nil
		}
		for _, profile := range profiles {
			if profile.Name == snapshot.name && profile.Source == "central_login" {
				snapshot.name += " (Local)"
				break
			}
		}

		var localMatches []AuthProfile
		for _, profile := range profiles {
			if profile.Name != snapshot.name {
				continue
			}
			if profile.Source == "central_login" {
				return nil, CurrentStatus{}, fmt.Errorf("cannot automatically replace Central profile %q", snapshot.name)
			}
			localMatches = append(localMatches, profile)
		}
		if len(localMatches) > 1 {
			return nil, CurrentStatus{}, fmt.Errorf("multiple local profiles use generated name %q", snapshot.name)
		}
		var existing *AuthProfile
		if len(localMatches) == 1 {
			existing = &localMatches[0]
		}
		rollback, err := s.currentSnapshotRollback(existing)
		if err != nil {
			return nil, CurrentStatus{}, err
		}
		written, err := s.writeCurrentSnapshot(snapshot, existing)
		if err != nil {
			return nil, CurrentStatus{}, err
		}
		if afterWrite != nil {
			afterWrite()
		}

		profiles, err = s.List()
		if err != nil {
			if rollbackErr := rollback(written.ID); rollbackErr != nil {
				return nil, CurrentStatus{}, fmt.Errorf("reload profiles: %w; rollback failed: %v", err, rollbackErr)
			}
			return nil, CurrentStatus{}, err
		}
		verified, verifiedOK, err := readCurrentSnapshot(codexDir)
		if err != nil {
			if rollbackErr := rollback(written.ID); rollbackErr != nil {
				return nil, CurrentStatus{}, fmt.Errorf("verify current authentication: %w; rollback failed: %v", err, rollbackErr)
			}
			return nil, CurrentStatus{}, err
		}
		if !verifiedOK {
			if err := rollback(written.ID); err != nil {
				return nil, CurrentStatus{}, err
			}
			return nil, CurrentStatus{}, errors.New("current Codex authentication changed during reconciliation")
		}
		if verified.contentHash != snapshot.contentHash {
			if err := rollback(written.ID); err != nil {
				return nil, CurrentStatus{}, err
			}
			continue
		}
		status := exactCurrentStatus(verified, profiles)
		if status.CurrentProfileID == "" {
			if err := rollback(written.ID); err != nil {
				return nil, CurrentStatus{}, err
			}
			return nil, CurrentStatus{}, errors.New("automatically imported authentication profile does not match current Codex authentication")
		}
		if err := s.writeActiveProfileID(status.CurrentProfileID); err != nil {
			if rollbackErr := rollback(written.ID); rollbackErr != nil {
				return nil, CurrentStatus{}, fmt.Errorf("write active profile: %w; rollback failed: %v", err, rollbackErr)
			}
			return nil, CurrentStatus{}, err
		}
		return profiles, status, nil
	}
	return nil, CurrentStatus{}, errors.New("current Codex authentication changed during reconciliation")
}

func (s *Store) activeCentralStatus(snapshot currentSnapshot, profiles []AuthProfile) (CurrentStatus, error) {
	activeID, err := activeProfileID(s.Root)
	if err != nil || activeID == "" {
		return CurrentStatus{}, err
	}
	for _, profile := range profiles {
		if profile.ID != activeID || profile.Source != "central_login" {
			continue
		}
		auth, config, err := s.GetRawContent(profile.ID)
		if err != nil {
			return CurrentStatus{}, err
		}
		normalized, err := codex.EnsureProviderBearerTokenInFragment([]byte(config), []byte(auth))
		if err != nil {
			return CurrentStatus{}, err
		}
		normalized, _, err = codex.ExtractAuthConfigFromBytes(normalized)
		if err != nil {
			return CurrentStatus{}, err
		}
		hash, err := ComputeContentHash([]byte(auth), normalized)
		if err != nil {
			return CurrentStatus{}, err
		}
		if hash == snapshot.contentHash {
			status := snapshot.status
			status.CurrentProfileID = profile.ID
			status.APIKeyImported = snapshot.profileType == "api_key"
			status.ChatGPTImported = snapshot.profileType == "chatgpt"
			return status, nil
		}
	}
	return CurrentStatus{}, nil
}

func (s *Store) currentSnapshotRollback(existing *AuthProfile) (func(string) error, error) {
	if existing == nil {
		return func(profileID string) error {
			return os.RemoveAll(filepath.Join(s.Root, profileID))
		}, nil
	}
	authJSON, configTOML, err := s.GetRawContent(existing.ID)
	if err != nil {
		return nil, err
	}
	original := *existing
	return func(string) error {
		dir := filepath.Join(s.Root, original.ID)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		if err := writeProfile(dir, original, []byte(authJSON), []byte(configTOML)); err != nil {
			return err
		}
		_, err := readSavedProfile(dir)
		return err
	}, nil
}

func (s *Store) writeActiveProfileID(profileID string) error {
	path := filepath.Join(s.Root, "active.json")
	if profileID == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return os.WriteFile(path, []byte(profileID), 0o600)
}

func (s *Store) writeCurrentSnapshot(snapshot currentSnapshot, existing *AuthProfile) (AuthProfile, error) {
	if err := os.MkdirAll(s.Root, 0o700); err != nil {
		return AuthProfile{}, err
	}
	now := time.Now().UTC()
	profile := AuthProfile{
		ID:        uuid.NewString(),
		Name:      snapshot.name,
		Type:      snapshot.profileType,
		Provider:  snapshot.provider,
		Source:    "manual",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if snapshot.profileType == "chatgpt" {
		profile.Source = "local_import"
	}
	if existing != nil {
		if existing.Source == "central_login" {
			return AuthProfile{}, fmt.Errorf("cannot automatically replace Central profile %q", snapshot.name)
		}
		profile = *existing
		profile.Name = snapshot.name
		profile.Type = snapshot.profileType
		profile.Provider = snapshot.provider
		profile.ReservedName = ""
		profile.UpdatedAt = now
	}

	staging, err := os.MkdirTemp(s.Root, ".agentpal-auth-import-")
	if err != nil {
		return AuthProfile{}, err
	}
	defer func() { _ = os.RemoveAll(staging) }()
	if err := writeProfile(staging, profile, snapshot.authJSON, snapshot.configTOML); err != nil {
		return AuthProfile{}, err
	}
	_, err = readSavedProfile(staging)
	if err != nil {
		return AuthProfile{}, err
	}

	finalDir := filepath.Join(s.Root, profile.ID)
	if existing == nil {
		if err := os.Rename(staging, finalDir); err != nil {
			return AuthProfile{}, err
		}
		return readSavedProfile(finalDir)
	}

	backupDir := filepath.Join(s.Root, ".agentpal-auth-backup-"+uuid.NewString())
	if err := os.Rename(finalDir, backupDir); err != nil {
		return AuthProfile{}, err
	}
	rollback := func(cause error) error {
		removeErr := os.RemoveAll(finalDir)
		restoreErr := os.Rename(backupDir, finalDir)
		if restoreErr == nil {
			_, restoreErr = readSavedProfile(finalDir)
		}
		if removeErr != nil || restoreErr != nil {
			return fmt.Errorf("%w; rollback failed: remove=%v restore=%v", cause, removeErr, restoreErr)
		}
		return cause
	}
	if err := os.Rename(staging, finalDir); err != nil {
		return AuthProfile{}, rollback(err)
	}
	installed, err := readSavedProfile(finalDir)
	if err != nil {
		return AuthProfile{}, rollback(err)
	}
	_ = os.RemoveAll(backupDir)
	return installed, nil
}
