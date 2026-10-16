package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"agentpal/internal/authprofile"
	"agentpal/internal/codex"
	appconfig "agentpal/internal/config"
	"agentpal/internal/types"
)

func (a *App) LoginChatGPT() (types.AuthProfile, error) {
	a.chatGPTMu.Lock()
	if a.chatGPTCancel != nil {
		a.chatGPTMu.Unlock()
		return types.AuthProfile{}, errors.New("ChatGPT login is already in progress")
	}
	parent := a.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	a.chatGPTCancel = cancel
	a.chatGPTMu.Unlock()
	defer func() { cancel(); a.chatGPTMu.Lock(); a.chatGPTCancel = nil; a.chatGPTMu.Unlock() }()
	state, err := appconfig.ReadState()
	if err != nil {
		return types.AuthProfile{}, err
	}
	dir, err := codex.ExpandPath(state.CodexDir)
	if err != nil {
		return types.AuthProfile{}, err
	}
	root, err := codex.ExpandPath("~/.agentpal/auth-profiles")
	if err != nil {
		return types.AuthProfile{}, err
	}
	executable, err := findCodexCLI()
	if err != nil {
		return types.AuthProfile{}, err
	}
	profile, err := authprofile.NewStore(root).LoginChatGPT(ctx, dir, func(ctx context.Context, home string) error {
		cmd := exec.CommandContext(ctx, executable, "login")
		cmd.Dir = home
		for _, entry := range os.Environ() {
			key, _, _ := strings.Cut(entry, "=")
			if key != "CODEX_HOME" && key != "OPENAI_API_KEY" && key != "OPENAI_BASE_URL" && key != "CODEX_API_KEY" && key != "CODEX_ACCESS_TOKEN" && key != "PATH" {
				cmd.Env = append(cmd.Env, entry)
			}
		}
		cmd.Env = append(cmd.Env, "CODEX_HOME="+home, "PATH="+filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
		// CLI output can contain authorization URLs or tokens; do not expose it in notices.
		if err := cmd.Run(); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errors.New("ChatGPT login failed. Check that Codex CLI can open your browser and reach OpenAI, then try again")
		}
		return nil
	})
	if errors.Is(err, context.Canceled) {
		return types.AuthProfile{}, errors.New("ChatGPT login cancelled")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return types.AuthProfile{}, errors.New("ChatGPT login timed out. Please try again")
	}
	return profileType(profile), err
}

func (a *App) CancelChatGPTLogin() {
	a.chatGPTMu.Lock()
	defer a.chatGPTMu.Unlock()
	if a.chatGPTCancel != nil {
		a.chatGPTCancel()
	}
}

func (a *App) shutdown(ctx context.Context) { a.CancelChatGPTLogin() }

// Finder-launched macOS apps do not inherit the user's shell PATH.
func findCodexCLI() (string, error) {
	if path, err := exec.LookPath("codex"); err == nil {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	candidates := []string{filepath.Join(home, ".local/bin/codex"), "/opt/homebrew/bin/codex", "/usr/local/bin/codex"}
	nvm, _ := filepath.Glob(filepath.Join(home, ".nvm/versions/node/*/bin/codex"))
	candidates = append(candidates, nvm...)
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return path, nil
		}
	}
	return "", errors.New("Codex CLI was not found. Install Codex CLI and restart AgentPal to log in with ChatGPT")
}
