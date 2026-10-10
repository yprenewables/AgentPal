package centralnode

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"agentpal/internal/centralscan"
	"agentpal/internal/codex"
	"agentpal/internal/constants"
	"agentpal/internal/types"
)

func BuildManifest(codexDir string, now time.Time) (types.CenterManifest, error) {
	info, err := os.Stat(codexDir)
	if err != nil {
		return types.CenterManifest{}, err
	}
	if !info.IsDir() {
		return types.CenterManifest{}, errors.New("Codex directory is not a directory")
	}
	m := types.CenterManifest{Schema: 1, App: constants.AppName, Version: constants.AppVersion, GeneratedAt: now.UTC()}
	skillsDir := filepath.Join(codexDir, "skills")
	if info, err := os.Stat(skillsDir); err == nil && info.IsDir() {
		m.Skills, m.Errors = centralscan.ScanSkills(skillsDir, m.Errors)
	}
	entries, err := codex.ReadMCP(filepath.Join(codexDir, "config.toml"))
	if err != nil {
		m.Errors = append(m.Errors, types.ScanError{Path: "config.toml", Error: err.Error()})
	} else {
		for _, entry := range entries {
			m.MCP = append(m.MCP, types.CenterMCP{Name: entry.Name, SHA256: entry.SHA256, Size: int64(len(entry.TOML))})
		}
	}
	return m, nil
}
