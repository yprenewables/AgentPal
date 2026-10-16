package authprofile

import (
	"encoding/json"
	"os"
	"path/filepath"

	"agentpal/internal/codex"
	"github.com/pelletier/go-toml/v2"
)

const defaultsFile = "model-defaults.json"

type profileDefaults struct {
	Model  string `json:"model" toml:"model"`
	Effort string `json:"effort" toml:"model_reasoning_effort"`
}

func readCurrentDefaults(codexDir string) (profileDefaults, error) {
	var defaults profileDefaults
	data, err := os.ReadFile(filepath.Join(codexDir, "config.toml"))
	if os.IsNotExist(err) {
		return defaults, nil
	}
	if err != nil {
		return defaults, err
	}
	err = toml.Unmarshal(data, &defaults)
	return defaults, err
}

func (s *Store) saveCurrentDefaults(codexDir, profileID string) error {
	defaults, err := readCurrentDefaults(codexDir)
	if err != nil {
		return err
	}
	data, err := json.Marshal(defaults)
	if err != nil {
		return err
	}
	return writeModelFile(filepath.Join(s.Root, profileID, defaultsFile), data)
}

func (s *Store) applyProfileDefaults(codexDir, profileID string) error {
	var defaults profileDefaults
	data, err := os.ReadFile(filepath.Join(s.Root, profileID, defaultsFile))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		if err := json.Unmarshal(data, &defaults); err != nil {
			return err
		}
	}
	return codex.SetModelDefaults(filepath.Join(codexDir, "config.toml"), defaults.Model, defaults.Effort)
}

func (s *Store) saveCurrentModelSettings(codexDir, profileID string) error {
	if err := copyOptionalModels(filepath.Join(codexDir, "models.json"), filepath.Join(s.Root, profileID, "models.json")); err != nil {
		return err
	}
	return s.saveCurrentDefaults(codexDir, profileID)
}
