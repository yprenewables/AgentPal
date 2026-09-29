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
	writeJSON(w, profileDetail(profile))
}

func profileDetail(p reservedProfile) types.RemoteAuthProfileDetail {
	var auth map[string]any
	if err := json.Unmarshal(p.AuthJSON, &auth); err != nil {
		auth = map[string]any{}
	}
	return types.RemoteAuthProfileDetail{
		Name: p.Meta.Name, Provider: p.Meta.Provider, Type: p.Meta.Type,
		ContentHash: p.ContentHash, UpdatedAt: p.UpdatedAt,
		Auth: auth, ConfigAuthToml: string(p.ConfigTOML),
	}
}

func (s *userProfileStore) listUsers() ([]string, error) {
	entries, err := os.ReadDir(s.root)
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := security.SafeJoin(s.root, entry.Name(), "auth-profiles"); err != nil {
			continue
		}
		items, err := s.list(entry.Name())
		if err != nil || len(items) == 0 {
			continue
		}
		result = append(result, entry.Name())
	}
	sort.Strings(result)
	return result, nil
}

func (s *userProfileStore) write(sub, name string, meta reservedMeta, auth, config []byte) error {
	if err := validateSubject(sub); err != nil {
		return err
	}
	if err := authprofile.ValidateReservedName(name); err != nil {
		return err
	}
	if meta.Name != name {
		return errors.New("profile name does not match its path")
	}
	base, err := security.SafeJoin(s.root, sub, "auth-profiles")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(base, 0o700); err != nil {
		return err
	}
	temp, err := os.MkdirTemp(base, ".agentpal-write-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(temp) }()
	metaData, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	for file, data := range map[string][]byte{"profile.json": metaData, "auth.json": auth, "config-auth.toml": config} {
		if err := os.WriteFile(filepath.Join(temp, file), data, 0o600); err != nil {
			return err
		}
	}
	target := filepath.Join(base, name)
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return os.Rename(temp, target)
}

func (s *userProfileStore) delete(sub, name string) error {
	if err := validateSubject(sub); err != nil {
		return err
	}
	if err := authprofile.ValidateReservedName(name); err != nil {
		return err
	}
	dir, err := security.SafeJoin(s.root, sub, "auth-profiles", name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

// listAll 返回所有用户的预留 profile，并带上所属用户 sub。
func (s *userProfileStore) listAll() ([]types.AdminAuthProfile, error) {
	entries, err := os.ReadDir(s.root)
	if os.IsNotExist(err) {
		return []types.AdminAuthProfile{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := []types.AdminAuthProfile{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sub := entry.Name()
		if err := validateSubject(sub); err != nil {
			continue
		}
		items, err := s.list(sub)
		if err != nil {
			continue
		}
		for _, item := range items {
			result = append(result, types.AdminAuthProfile{
				Sub: sub, Name: item.Name, Provider: item.Provider, Type: item.Type,
				ContentHash: item.ContentHash, UpdatedAt: item.UpdatedAt,
			})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Sub != result[j].Sub {
			return result[i].Sub < result[j].Sub
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}
