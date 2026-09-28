package authprofile

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pelletier/go-toml/v2"
)

type AuthProfile struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Type             string    `json:"type"`
	Provider         string    `json:"provider"`
	Source           string    `json:"source"`
	ReservedName     string    `json:"reservedName,omitempty"`
	ContentHash      string    `json:"contentHash,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
	Active           bool      `json:"active"`
	APIKeyPreview    string    `json:"apiKeyPreview,omitempty"`
	BaseURL          string    `json:"baseUrl,omitempty"`
	ChatGPTUserID    string    `json:"chatgptUserId,omitempty"`
	ChatGPTExpiresAt string    `json:"chatgptExpiresAt,omitempty"`
}
type APIKeyRequest struct {
	Name     string
	Provider string
	BaseURL  string
	APIKey   string
}
type Store struct{ Root string }

func NewStore(root string) *Store { return &Store{Root: root} }
func (s *Store) List() ([]AuthProfile, error) {
	if err := os.MkdirAll(s.Root, 0o700); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		return nil, err
	}
	active, _ := os.ReadFile(filepath.Join(s.Root, "active.json"))
	activeID := strings.TrimSpace(string(active))
	var result []AuthProfile
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		p, err := readSavedProfile(filepath.Join(s.Root, entry.Name()))
		if err != nil {
			continue
		}
		p.Active = p.ID == activeID
		result = append(result, p)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func readSavedProfile(dir string) (AuthProfile, error) {
	metadata, err := os.ReadFile(filepath.Join(dir, "profile.json"))
	if err != nil {
		return AuthProfile{}, err
	}
	var profile AuthProfile
	if err := json.Unmarshal(metadata, &profile); err != nil {
		return AuthProfile{}, err
	}
	if profile.ID == "" || profile.Name == "" || profile.Type == "" {
		return AuthProfile{}, errors.New("profile metadata is incomplete")
	}

	authData, err := os.ReadFile(filepath.Join(dir, "auth.json"))
	if err != nil {
		return AuthProfile{}, err
	}
	var auth map[string]any
	if err := json.Unmarshal(authData, &auth); err != nil || auth == nil {
		if err == nil {
			err = errors.New("auth.json must contain an object")
		}
		return AuthProfile{}, err
	}

	configData, err := os.ReadFile(filepath.Join(dir, "config-auth.toml"))
	if err != nil {
		return AuthProfile{}, err
	}
	var config map[string]any
	if err := toml.Unmarshal(configData, &config); err != nil {
		return AuthProfile{}, err
	}
	provider := stringValue(config["model_provider"])
	if provider == "" {
		return AuthProfile{}, errors.New("saved auth config must contain model_provider")
	}

	profile.Provider = provider
	profile.BaseURL = baseURLFromConfig(config)
	profile.APIKeyPreview = maskAPIKey(findAPIKey(auth))
	profile.ChatGPTUserID = findUserID(auth)
	profile.ChatGPTExpiresAt = findExpiresAt(auth)
	if profile.Type == "api_key" && profile.APIKeyPreview == "" {
		return AuthProfile{}, errors.New("API key profile has no saved API key")
	}
	if hash, err := ComputeContentHash(authData, configData); err == nil {
		profile.ContentHash = hash
	}
	return profile, nil
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
func (s *Store) CreateAPIKey(req APIKeyRequest) (AuthProfile, error) {
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Provider) == "" || strings.TrimSpace(req.APIKey) == "" {
		return AuthProfile{}, errors.New("name, provider, and API key are required")
	}
	id := uuid.NewString()
	now := time.Now().UTC()
	p := AuthProfile{ID: id, Name: req.Name, Type: "api_key", Provider: req.Provider, Source: "manual", APIKeyPreview: maskAPIKey(req.APIKey), BaseURL: req.BaseURL, CreatedAt: now, UpdatedAt: now}
	dir := filepath.Join(s.Root, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return p, err
	}
	auth, _ := json.Marshal(map[string]any{"OPENAI_API_KEY": req.APIKey})
	config := []byte("model_provider = \"" + req.Provider + "\"\n\n[model_providers." + req.Provider + "]\nname = \"" + req.Provider + "\"\nbase_url = \"" + req.BaseURL + "\"\nenv_key = \"OPENAI_API_KEY\"\nwire_api = \"responses\"\n")
	if strings.HasPrefix(req.APIKey, "sk-mem-") {
		config = append(config, []byte("experimental_bearer_token = \""+req.APIKey+"\"\n")...)
	}
	if err := writeProfile(dir, p, auth, config); err != nil {
		return p, err
	}
	return readSavedProfile(dir)
}

func (s *Store) Delete(profileID string, currentProfileID ...string) error {
	if profileID == "" || filepath.Base(profileID) != profileID || profileID == "." || profileID == ".." {
		return errors.New("invalid profile ID")
	}
	active, err := os.ReadFile(filepath.Join(s.Root, "active.json"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	knownCurrentID := strings.TrimSpace(string(active))
	if len(currentProfileID) > 0 {
		knownCurrentID = strings.TrimSpace(currentProfileID[0])
	}
	if knownCurrentID == profileID {
		return errors.New("cannot delete the active profile")
	}
	profileDir := filepath.Join(s.Root, profileID)
	if _, err := os.Stat(profileDir); err != nil {
		return err
	}
	if err := os.RemoveAll(profileDir); err != nil {
		return err
	}
	if strings.TrimSpace(string(active)) == profileID {
		_ = os.Remove(filepath.Join(s.Root, "active.json"))
	}
	return nil
}

func writeProfile(dir string, p AuthProfile, auth, config []byte) error {
	if hash, err := ComputeContentHash(auth, config); err == nil {
		p.ContentHash = hash
	}
	meta, _ := json.MarshalIndent(p, "", "  ")
	meta = append(meta, '\n')
	for name, data := range map[string][]byte{"profile.json": meta, "auth.json": auth, "config-auth.toml": config} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			return err
		}
	}
	return nil
}
