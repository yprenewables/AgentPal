package authprofile

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/pelletier/go-toml/v2"
)

// CentralProfile 是中心预留 profile 的传输形态。
type CentralProfile struct {
	Name       string
	Provider   string
	Type       string
	AuthJSON   []byte
	ConfigTOML []byte
}

func validateCentralProfile(c CentralProfile) error {
	if err := ValidateReservedName(c.Name); err != nil {
		return err
	}
	if c.Type == "" || c.Provider == "" {
		return errors.New("central profile metadata is incomplete")
	}
	var config map[string]any
	if err := toml.Unmarshal(c.ConfigTOML, &config); err != nil {
		return err
	}
	if provider, _ := config["model_provider"].(string); provider != c.Name {
		return errors.New("central profile provider does not match its name")
	}
	return nil
}

func (s *Store) FindByReservedName(name string) (AuthProfile, bool, error) {
	profiles, err := s.List()
	if err != nil {
		return AuthProfile{}, false, err
	}
	for _, profile := range profiles {
		if profile.ReservedName == name {
			return profile, true, nil
		}
	}
	return AuthProfile{}, false, nil
}

func (s *Store) AddCentral(c CentralProfile) (AuthProfile, error) {
	if err := validateCentralProfile(c); err != nil {
		return AuthProfile{}, err
	}
	now := time.Now().UTC()
	p := AuthProfile{ID: uuid.NewString(), Name: c.Name, Type: c.Type, Provider: c.Provider, Source: "central_login", ReservedName: c.Name, CreatedAt: now, UpdatedAt: now}
	dir := filepath.Join(s.Root, p.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return p, err
	}
	if err := writeProfile(dir, p, c.AuthJSON, c.ConfigTOML); err != nil {
		return p, err
	}
	return readSavedProfile(dir)
}

func (s *Store) OverwriteCentral(profileID string, c CentralProfile) (AuthProfile, error) {
	if profileID == "" || filepath.Base(profileID) != profileID || profileID == "." || profileID == ".." {
		return AuthProfile{}, errors.New("invalid profile ID")
	}
	if err := validateCentralProfile(c); err != nil {
		return AuthProfile{}, err
	}
	dir := filepath.Join(s.Root, profileID)
	existing, err := readSavedProfile(dir)
	if err != nil {
		return AuthProfile{}, err
	}
	existing.Name = c.Name
	existing.Type = c.Type
	existing.Provider = c.Provider
	existing.ReservedName = c.Name
	existing.Source = "central_login"
	existing.UpdatedAt = time.Now().UTC()
	if err := writeProfile(dir, existing, c.AuthJSON, c.ConfigTOML); err != nil {
		return existing, err
	}
	return readSavedProfile(dir)
}

func (s *Store) ContentHashFor(profileID string) (string, error) {
	if profileID == "" || filepath.Base(profileID) != profileID {
		return "", errors.New("invalid profile ID")
	}
	dir := filepath.Join(s.Root, profileID)
	authData, err := os.ReadFile(filepath.Join(dir, "auth.json"))
	if err != nil {
		return "", err
	}
	configData, err := os.ReadFile(filepath.Join(dir, "config-auth.toml"))
	if err != nil {
		return "", err
	}
	return ComputeContentHash(authData, configData)
}
