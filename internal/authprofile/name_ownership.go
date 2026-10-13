package authprofile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
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
	if _, err := readSavedProfile(staging); err != nil {
		return err
	}
	backup := filepath.Join(s.Root, ".agentpal-auth-backup-"+uuid.NewString())
	if err := os.Rename(dir, backup); err != nil {
		return err
	}
	if err := os.Rename(staging, dir); err != nil {
		if restoreErr := os.Rename(backup, dir); restoreErr != nil {
			return fmt.Errorf("install profile: %w; rollback failed: %v", err, restoreErr)
		}
		return err
	}
	if _, err := readSavedProfile(dir); err != nil {
		removeErr := os.RemoveAll(dir)
		restoreErr := os.Rename(backup, dir)
		if removeErr != nil || restoreErr != nil {
			return fmt.Errorf("validate installed profile: %w; rollback failed: remove=%v restore=%v", err, removeErr, restoreErr)
		}
		return err
	}
	// The replacement is committed. A stale hidden backup is safer than
	// reporting failure after callers have already observed the new profile.
	_ = os.RemoveAll(backup)
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

func (s *Store) prepareCentralImport(c CentralProfile) (*AuthProfile, func() error, error) {
	profiles, err := s.List()
	if err != nil {
		return nil, nil, err
	}
	matches, err := sameNameLocal(profiles, c.Name)
	if err != nil {
		return nil, nil, err
	}
	if len(matches) == 0 {
		return nil, func() error { return nil }, nil
	}
	local := matches[0]
	incomingHash, err := ComputeContentHash(c.AuthJSON, c.ConfigTOML)
	if err != nil {
		return nil, nil, err
	}
	auth, config, err := s.GetRawContent(local.ID)
	if err != nil {
		return nil, nil, err
	}
	if local.ContentHash == incomingHash {
		converted := centralMetadata(local, c)
		if err := s.replaceProfile(converted, c.AuthJSON, c.ConfigTOML); err != nil {
			return nil, nil, err
		}
		saved, err := readSavedProfile(filepath.Join(s.Root, local.ID))
		return &saved, func() error { return nil }, err
	}
	derivedName := c.Name + " (Local)"
	if err := ensureDerivedNameAvailable(profiles, local, derivedName); err != nil {
		return nil, nil, err
	}
	original := local
	local.Name = derivedName
	local.UpdatedAt = time.Now().UTC()
	if err := s.replaceProfile(local, []byte(auth), []byte(config)); err != nil {
		return nil, nil, err
	}
	rollback := func() error {
		return s.replaceProfile(original, []byte(auth), []byte(config))
	}
	return nil, rollback, nil
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
		backup := filepath.Join(s.Root, ".agentpal-auth-backup-"+uuid.NewString())
		if err := os.Rename(filepath.Join(s.Root, local.ID), backup); err != nil {
			return nil, err
		}
		if wasActive {
			if err := s.writeActiveProfileID(central.ID); err != nil {
				_ = os.Rename(backup, filepath.Join(s.Root, local.ID))
				return nil, err
			}
		}
		if err := os.RemoveAll(backup); err != nil {
			if wasActive {
				_ = s.writeActiveProfileID(local.ID)
			}
			if restoreErr := os.Rename(backup, filepath.Join(s.Root, local.ID)); restoreErr != nil {
				return nil, fmt.Errorf("remove duplicate local profile: %w; rollback failed: %v", err, restoreErr)
			}
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
