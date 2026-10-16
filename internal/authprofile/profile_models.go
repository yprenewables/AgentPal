package authprofile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"agentpal/internal/codex"
)

func copyOptionalModels(source, target string) error {
	data, err := os.ReadFile(source)
	if os.IsNotExist(err) {
		err = os.Remove(target)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err != nil {
		return err
	}
	return writeModelFile(target, data)
}

func writeModelFile(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".profile-models-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func (s *Store) saveOutgoingModels(codexDir string) error {
	_, fileErr := os.Stat(filepath.Join(codexDir, "models.json"))
	defaults, err := readCurrentDefaults(codexDir)
	if err != nil {
		return err
	}
	if fileErr != nil && !os.IsNotExist(fileErr) {
		return fileErr
	}
	if os.IsNotExist(fileErr) && defaults.Model == "" && defaults.Effort == "" {
		// No legacy file to migrate. Update only an already identifiable profile.
		snapshot, ok, err := readCurrentSnapshot(codexDir)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		profiles, err := s.List()
		if err != nil {
			return err
		}
		current := exactCurrentStatus(snapshot, profiles)
		if current.CurrentProfileID == "" {
			return nil
		}
		return s.saveCurrentModelSettings(codexDir, current.CurrentProfileID)
	}
	_, current, err := s.reconcileCurrentLocked(codexDir, nil)
	if err != nil {
		return err
	}
	if current.CurrentProfileID == "" {
		if os.IsNotExist(fileErr) {
			return nil
		}
		return errors.New("cannot associate existing model settings with the current authentication")
	}
	return s.saveCurrentModelSettings(codexDir, current.CurrentProfileID)
}

// WithCurrentModels serializes model saves with profile switches and reconciliation.
func (s *Store) WithCurrentModels(codexDir string, save func() error) error {
	reconcileMu.Lock()
	defer reconcileMu.Unlock()
	_, current, err := s.reconcileCurrentLocked(codexDir, nil)
	if err != nil {
		return err
	}
	if current.CurrentProfileID == "" {
		return errors.New("current authentication profile could not be identified")
	}
	profileModels := filepath.Join(s.Root, current.CurrentProfileID, "models.json")
	backups, err := backupFiles(filepath.Join(codexDir, "models.json"), filepath.Join(codexDir, "config.toml"), profileModels, filepath.Join(s.Root, current.CurrentProfileID, defaultsFile))
	if err != nil {
		return err
	}
	if err := save(); err != nil {
		return restoreFiles(backups, err)
	}
	if err := s.saveCurrentModelSettings(codexDir, current.CurrentProfileID); err != nil {
		return restoreFiles(backups, err)
	}
	return nil
}

type fileBackup struct {
	path   string
	data   []byte
	exists bool
	mode   os.FileMode
}

func backupFiles(paths ...string) ([]fileBackup, error) {
	backups := make([]fileBackup, 0, len(paths))
	for _, path := range paths {
		b := fileBackup{path: path}
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			backups = append(backups, b)
			continue
		}
		if err != nil {
			return nil, err
		}
		b.data, err = os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		b.exists = true
		b.mode = info.Mode().Perm()
		backups = append(backups, b)
	}
	return backups, nil
}

func restoreFiles(backups []fileBackup, cause error) error {
	for _, b := range backups {
		var err error
		if b.exists {
			err = os.WriteFile(b.path, b.data, b.mode)
		} else {
			err = os.Remove(b.path)
			if os.IsNotExist(err) {
				err = nil
			}
		}
		if err != nil {
			cause = errors.Join(cause, fmt.Errorf("restore %s: %w", filepath.Base(b.path), err))
		}
	}
	return cause
}

func (s *Store) applyProfileModels(codexDir, profileID string) error {
	target := filepath.Join(codexDir, "models.json")
	if err := copyOptionalModels(filepath.Join(s.Root, profileID, "models.json"), target); err != nil {
		return err
	}
	catalog := ""
	if _, err := os.Stat(target); err == nil {
		catalog, err = filepath.Abs(target)
		if err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := codex.SetModelCatalog(filepath.Join(codexDir, "config.toml"), catalog); err != nil {
		return err
	}
	return s.applyProfileDefaults(codexDir, profileID)
}
