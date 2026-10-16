package authprofile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// LoginChatGPT authorizes in an isolated home before touching the active files.
func (s *Store) LoginChatGPT(ctx context.Context, codexDir string, run func(context.Context, string) error) (AuthProfile, error) {
	home, err := os.MkdirTemp("", "agentpal-chatgpt-")
	if err != nil {
		return AuthProfile{}, err
	}
	defer os.RemoveAll(home)
	if err := run(ctx, home); err != nil {
		return AuthProfile{}, err
	}
	if err := ctx.Err(); err != nil {
		return AuthProfile{}, err
	}
	data, err := os.ReadFile(filepath.Join(home, "auth.json"))
	if err != nil {
		return AuthProfile{}, fmt.Errorf("read ChatGPT login result: %w", err)
	}
	var auth struct {
		Tokens struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(data, &auth); err != nil {
		return AuthProfile{}, errors.New("Codex returned invalid ChatGPT credentials")
	}
	if auth.Tokens.AccessToken == "" || auth.Tokens.RefreshToken == "" {
		return AuthProfile{}, errors.New("Codex did not return a ChatGPT login")
	}
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("model_provider = \"openai\"\n"), 0600); err != nil {
		return AuthProfile{}, err
	}

	reconcileMu.Lock()
	defer reconcileMu.Unlock()
	// Save the current profile before replacing its files, including credentials
	// changed externally since the last UI refresh.
	snapshot, exists, err := readCurrentSnapshot(codexDir)
	if err != nil {
		return AuthProfile{}, err
	}
	if exists {
		profiles, err := s.List()
		if err != nil {
			return AuthProfile{}, err
		}
		if exactCurrentStatus(snapshot, profiles).CurrentProfileID == "" {
			if _, err := s.writeCurrentSnapshot(snapshot, nil); err != nil {
				return AuthProfile{}, err
			}
		}
	}
	profile, err := s.ImportCurrent(home, "Current ChatGPT login")
	if err != nil {
		return AuthProfile{}, err
	}
	// ImportCurrent is shared with manual imports. Mark the new profile by its
	// actual content so reconciliation recognizes it immediately.
	profile.Type = "chatgpt"
	profile.ContentHash, err = ComputeContentHash(data, []byte("model_provider = \"openai\"\n"))
	if err != nil {
		return AuthProfile{}, err
	}
	if err := writeProfile(filepath.Join(s.Root, profile.ID), profile, data, []byte("model_provider = \"openai\"\n")); err != nil {
		return AuthProfile{}, err
	}
	paths := []string{filepath.Join(codexDir, "auth.json"), filepath.Join(codexDir, "config.toml"), filepath.Join(s.Root, "active.json")}
	type backup struct {
		data   []byte
		exists bool
		mode   os.FileMode
	}
	backups := make([]backup, len(paths))
	for i, path := range paths {
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return AuthProfile{}, err
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return AuthProfile{}, err
		}
		backups[i] = backup{contents, true, info.Mode().Perm()}
	}
	if err := ctx.Err(); err != nil {
		return AuthProfile{}, err
	}
	result, err := s.activateLocked(codexDir, profile.ID, true)
	if err == nil && result.OK {
		profile.Active = true
		return profile, nil
	}
	if err == nil {
		err = errors.New("ChatGPT profile could not be activated")
	}
	for i, path := range paths {
		var restoreErr error
		if backups[i].exists {
			restoreErr = os.WriteFile(path, backups[i].data, backups[i].mode)
		} else {
			restoreErr = os.Remove(path)
			if os.IsNotExist(restoreErr) {
				restoreErr = nil
			}
		}
		if restoreErr != nil {
			err = errors.Join(err, fmt.Errorf("restore %s: %w", filepath.Base(path), restoreErr))
		}
	}
	return AuthProfile{}, err
}
