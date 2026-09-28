package authprofile

import (
	"agentpal/internal/codex"
	"encoding/json"
	"github.com/google/uuid"
	"os"
	"path/filepath"
	"time"
)

func (s *Store) ImportCurrent(codexDir, name string) (AuthProfile, error) {
	auth, err := os.ReadFile(filepath.Join(codexDir, "auth.json"))
	if err != nil {
		return AuthProfile{}, err
	}
	var parsed any
	if err := json.Unmarshal(auth, &parsed); err != nil {
		return AuthProfile{}, err
	}
	fragment, summary, err := codex.ExtractAuthConfig(filepath.Join(codexDir, "config.toml"))
	if err != nil {
		return AuthProfile{}, err
	}
	p := AuthProfile{ID: uuid.NewString(), Name: name, Type: "chatgpt", Provider: summary.Provider, Source: "local_import", ChatGPTUserID: findUserID(parsed), ChatGPTExpiresAt: findExpiresAt(parsed), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	dir := filepath.Join(s.Root, p.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return p, err
	}
	if err := writeProfile(dir, p, auth, fragment); err != nil {
		return p, err
	}
	return p, nil
}
