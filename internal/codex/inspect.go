package codex

import (
	"os"
	"path/filepath"

	"agentpal/internal/security"
	"agentpal/internal/types"
)

func InspectDir(path string) (types.CodexInspection, error) {
	expanded, err := ExpandPath(path)
	if err != nil {
		return types.CodexInspection{}, err
	}
	if expanded == "" {
		expanded, err = ExpandPath(DefaultDir())
		if err != nil {
			return types.CodexInspection{}, err
		}
	}

	inspection := types.CodexInspection{Path: expanded}
	inspection.Config = inspectFile(filepath.Join(expanded, "config.toml"), false)
	inspection.Auth = inspectFile(filepath.Join(expanded, "auth.json"), true)
	inspection.Skills = inspectSkills(filepath.Join(expanded, "skills"))
	return inspection, nil
}

func inspectFile(path string, sensitive bool) types.ResourceStatus {
	status := types.ResourceStatus{Path: path, Sensitive: sensitive}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return status
	}
	status.Exists = true
	status.Size = info.Size()
	if filepath.Base(path) == "config.toml" {
		sections, err := InspectConfigSections(path)
		if err == nil {
			status.RootKeys = sections.RootKeys
			status.Tables = sections.Tables
		}
	}
	return status
}

func inspectSkills(path string) types.ResourceStatus {
	status := types.ResourceStatus{Path: path}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return status
	}
	status.Exists = true
	_ = filepath.WalkDir(path, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if current != path && security.IsIgnoredName(entry.Name()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type().IsRegular() {
			status.Count++
		}
		return nil
	})
	return status
}
