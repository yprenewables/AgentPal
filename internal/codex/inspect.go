package codex

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	skillSet := map[string]bool{}
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
			rel, err := filepath.Rel(path, current)
			if err == nil {
				first := firstPathSegment(filepath.ToSlash(rel))
				if first != "" {
					skillSet[first] = true
				}
			}
		}
		return nil
	})
	for skill := range skillSet {
		status.Skills = append(status.Skills, skill)
	}
	sort.Strings(status.Skills)
	return status
}

func firstPathSegment(path string) string {
	path = strings.Trim(path, "/")
	if path == "" {
		return ""
	}
	if idx := strings.Index(path, "/"); idx >= 0 {
		return path[:idx]
	}
	return path
}
