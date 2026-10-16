package authprofile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func centralMetadata(local AuthProfile, c CentralProfile) AuthProfile {
	local.Name = c.Name
	local.Type = c.Type
	local.Provider = c.Provider
	local.Source = "central_login"
	local.ReservedName = c.Name
	local.UpdatedAt = time.Now().UTC()
	return local
}

func (s *Store) replaceProfile(profile AuthProfile, auth, config []byte) error {
	dir := filepath.Join(s.Root, profile.ID)
	staging, err := os.MkdirTemp(s.Root, ".agentpal-auth-import-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(staging) }()
	if err := writeProfile(staging, profile, auth, config); err != nil {
		return err
	}
	if err := copyOptionalModels(filepath.Join(dir, "models.json"), filepath.Join(staging, "models.json")); err != nil {
		return err
	}
	if _, err := readSavedProfile(staging); err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.Rename(staging, dir); err != nil {
		return err
	}
	if _, err := readSavedProfile(dir); err != nil {
		return err
	}
	return nil
}

func centralOwnedName(central AuthProfile) string {
	if central.ReservedName != "" {
		return central.ReservedName
	}
	return central.Name
}

func sameNameLocal(profiles []AuthProfile, name string) ([]AuthProfile, error) {
	var matches []AuthProfile
	for _, profile := range profiles {
		if profile.Name == name && profile.Source != "central_login" {
			matches = append(matches, profile)
		}
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("multiple local profiles use Central name %q", name)
	}
	return matches, nil
}

func ensureDerivedNameAvailable(profiles []AuthProfile, local AuthProfile, derivedName string) error {
	for _, profile := range profiles {
		if profile.ID != local.ID && profile.Name == derivedName {
			return fmt.Errorf("profile name %q is already in use", derivedName)
		}
	}
	return nil
}

func validateCentralNameOwnership(profiles []AuthProfile, ownedName, centralHash string) error {
	matches, err := sameNameLocal(profiles, ownedName)
	if err != nil || len(matches) == 0 {
		return err
	}
	if matches[0].ContentHash == centralHash {
		return nil
	}
	return ensureDerivedNameAvailable(profiles, matches[0], ownedName+" (Local)")
}

func (s *Store) prepareCentralImport(c CentralProfile) (*AuthProfile, error) {
	profiles, err := s.List()
	if err != nil {
		return nil, err
	}
	matches, err := sameNameLocal(profiles, c.Name)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, nil
	}
	local := matches[0]
	incomingHash, err := ComputeContentHash(c.AuthJSON, c.ConfigTOML)
	if err != nil {
		return nil, err
	}
	auth, config, err := s.GetRawContent(local.ID)
	if err != nil {
		return nil, err
	}
	if local.ContentHash == incomingHash {
		converted := centralMetadata(local, c)
		if err := s.replaceProfile(converted, c.AuthJSON, c.ConfigTOML); err != nil {
			return nil, err
		}
		saved, err := readSavedProfile(filepath.Join(s.Root, local.ID))
		return &saved, err
	}
	derivedName := c.Name + " (Local)"
	if err := ensureDerivedNameAvailable(profiles, local, derivedName); err != nil {
		return nil, err
	}
	local.Name = derivedName
	local.UpdatedAt = time.Now().UTC()
	if err := s.replaceProfile(local, []byte(auth), []byte(config)); err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *Store) reconcileCentralName(central AuthProfile) ([]AuthProfile, error) {
	profiles, err := s.List()
	if err != nil {
		return nil, err
	}
	ownedName := centralOwnedName(central)
	if central.Name != ownedName {
		auth, config, err := s.GetRawContent(central.ID)
		if err != nil {
			return nil, err
		}
		central.Name = ownedName
		central.UpdatedAt = time.Now().UTC()
		if err := s.replaceProfile(central, []byte(auth), []byte(config)); err != nil {
			return nil, err
		}
		profiles, err = s.List()
		if err != nil {
			return nil, err
		}
	}
	matches, err := sameNameLocal(profiles, ownedName)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return profiles, nil
	}
	local := matches[0]
	if local.ContentHash == central.ContentHash {
		active, readErr := os.ReadFile(filepath.Join(s.Root, "active.json"))
		if readErr != nil && !os.IsNotExist(readErr) {
			return nil, readErr
		}
		wasActive := strings.TrimSpace(string(active)) == local.ID
		// When duplicate credentials are consolidated, carry local model settings
		// to the surviving ID if it has none (or the local profile is active).
		_, modelsErr := os.Stat(filepath.Join(s.Root, central.ID, "models.json"))
		if wasActive || os.IsNotExist(modelsErr) {
			if _, err := os.Stat(filepath.Join(s.Root, local.ID, "models.json")); err == nil {
				if err := copyOptionalModels(filepath.Join(s.Root, local.ID, "models.json"), filepath.Join(s.Root, central.ID, "models.json")); err != nil {
					return nil, err
				}
			} else if !os.IsNotExist(err) {
				return nil, err
			}
		}
		if wasActive {
			if err := s.writeActiveProfileID(central.ID); err != nil {
				return nil, err
			}
		}
		if err := os.RemoveAll(filepath.Join(s.Root, local.ID)); err != nil {
			return nil, err
		}
		return s.List()
	}
	derivedName := ownedName + " (Local)"
	if err := ensureDerivedNameAvailable(profiles, local, derivedName); err != nil {
		return nil, err
	}
	auth, config, err := s.GetRawContent(local.ID)
	if err != nil {
		return nil, err
	}
	local.Name = derivedName
	local.UpdatedAt = time.Now().UTC()
	if err := s.replaceProfile(local, []byte(auth), []byte(config)); err != nil {
		return nil, err
	}
	return s.List()
}

func activeProfileID(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "active.json"))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(data))
	if id == "" {
		return "", errors.New("active profile ID is empty")
	}
	return id, nil
}
