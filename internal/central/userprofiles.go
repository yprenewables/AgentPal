package central

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"agentpal/internal/authprofile"
	"agentpal/internal/security"
	"agentpal/internal/types"

	"github.com/pelletier/go-toml/v2"
)

type userProfileStore struct{ root string }

func newUserProfileStore(root string) *userProfileStore { return &userProfileStore{root: root} }

type reservedMeta struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Provider string `json:"provider"`
}

type reservedProfile struct {
	Meta        reservedMeta
	AuthJSON    []byte
	ConfigTOML  []byte
	ContentHash string
	UpdatedAt   time.Time
}

func (p reservedProfile) remote() types.RemoteAuthProfile {
	return types.RemoteAuthProfile{Name: p.Meta.Name, Provider: p.Meta.Provider, Type: p.Meta.Type, ContentHash: p.ContentHash, UpdatedAt: p.UpdatedAt}
}

func (s *userProfileStore) list(sub string) ([]types.RemoteAuthProfile, error) {
	dir, err := security.SafeJoin(s.root, sub, "auth-profiles")
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []types.RemoteAuthProfile{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := []types.RemoteAuthProfile{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		profile, err := s.read(sub, entry.Name())
		if err != nil {
			continue
		}
		result = append(result, profile.remote())
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (s *userProfileStore) read(sub, name string) (reservedProfile, error) {
	if err := authprofile.ValidateReservedName(name); err != nil {
		return reservedProfile{}, err
	}
	dir, err := security.SafeJoin(s.root, sub, "auth-profiles", name)
	if err != nil {
		return reservedProfile{}, err
	}
	metaData, err := os.ReadFile(filepath.Join(dir, "profile.json"))
	if err != nil {
		return reservedProfile{}, err
	}
	var meta reservedMeta
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return reservedProfile{}, err
	}
	if meta.Name != name || meta.Type == "" || meta.Provider == "" {
		return reservedProfile{}, errors.New("reserved profile metadata is invalid")
	}
	authData, err := os.ReadFile(filepath.Join(dir, "auth.json"))
	if err != nil {
		return reservedProfile{}, err
	}
	configData, err := os.ReadFile(filepath.Join(dir, "config-auth.toml"))
	if err != nil {
		return reservedProfile{}, err
	}
	var config map[string]any
	if err := toml.Unmarshal(configData, &config); err != nil {
		return reservedProfile{}, err
	}
	if provider, _ := config["model_provider"].(string); provider != name {
		return reservedProfile{}, errors.New("reserved profile provider does not match its name")
	}
	hash, err := authprofile.ComputeContentHash(authData, configData)
	if err != nil {
		return reservedProfile{}, err
	}
	updated := time.Time{}
	if info, err := os.Stat(filepath.Join(dir, "profile.json")); err == nil {
		updated = info.ModTime().UTC()
	}
	return reservedProfile{Meta: meta, AuthJSON: authData, ConfigTOML: configData, ContentHash: hash, UpdatedAt: updated}, nil
}

func (s *Server) requireSubject(r *http.Request) (string, error) {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if token == "" {
		return "", errors.New("bearer token is required")
	}
	return ParseJWTSubject(token)
}

func (s *Server) authProfiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	subject, err := s.requireSubject(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/v1/auth-profiles")
	name = strings.TrimPrefix(name, "/")
	if name == "" {
		items, err := s.userProfiles.list(subject)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, items)
		return
	}
	profile, err := s.userProfiles.read(subject, name)
	if err != nil {
		http.Error(w, "profile not found", http.StatusNotFound)
		return
	}
	var auth map[string]any
	if err := json.Unmarshal(profile.AuthJSON, &auth); err != nil {
		auth = map[string]any{}
	}
	writeJSON(w, types.RemoteAuthProfileDetail{
		Name: profile.Meta.Name, Provider: profile.Meta.Provider, Type: profile.Meta.Type,
		ContentHash: profile.ContentHash, UpdatedAt: profile.UpdatedAt,
		Auth: auth, ConfigAuthToml: string(profile.ConfigTOML),
	})
}
