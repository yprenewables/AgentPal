package share

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"agentpal/internal/codex"
	"agentpal/internal/constants"
	"agentpal/internal/security"
	"agentpal/internal/types"
)

func BuildManifest(req types.ShareRequest) (types.RemoteManifest, string, error) {
	base, err := codex.ExpandPath(req.CodexDir)
	if err != nil {
		return types.RemoteManifest{}, "", err
	}
	manifest := types.RemoteManifest{Schema: 1, App: constants.AppName, Version: constants.AppVersion}
	if req.ShareConfig {
		entry, err := fileResource(base, "config.toml", false)
		if err != nil {
			return manifest, base, err
		}
		entry.RootKeys = req.ConfigRootKeys
		entry.Tables = req.ConfigTables
		manifest.Shared.Config = entry
	}
	if req.ShareAuth {
		entry, err := fileResource(base, "auth.json", true)
		if err != nil {
			return manifest, base, err
		}
		manifest.Shared.Auth = entry
	} else {
		manifest.Shared.Auth.Sensitive = true
	}
	if req.ShareSkills {
		entry, err := skillsResource(base, req.Skills)
		if err != nil {
			return manifest, base, err
		}
		manifest.Shared.Skills = entry
	}
	if !manifest.Shared.Config.Enabled && !manifest.Shared.Auth.Enabled && !manifest.Shared.Skills.Enabled {
		return manifest, base, errors.New("select at least one existing resource to share")
	}
	return manifest, base, nil
}

func fileResource(base, relPath string, sensitive bool) (types.ManifestResource, error) {
	path, err := security.SafeJoin(base, relPath)
	if err != nil {
		return types.ManifestResource{}, err
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return types.ManifestResource{}, errors.New(relPath + " was selected but was not found in the sharing directory")
	}
	hash, err := hashFile(path)
	if err != nil {
		return types.ManifestResource{}, err
	}
	return types.ManifestResource{Enabled: true, Path: relPath, SHA256: hash, Size: info.Size(), Sensitive: sensitive}, nil
}

func skillsResource(base string, selectedSkills []string) (types.ManifestResource, error) {
	skillsDir, err := security.SafeJoin(base, "skills")
	if err != nil {
		return types.ManifestResource{}, err
	}
	info, err := os.Stat(skillsDir)
	if err != nil || !info.IsDir() {
		return types.ManifestResource{}, errors.New("skills was selected but was not found in the sharing directory")
	}
	selectedSet := selectedSkillSet(selectedSkills)
	skillSet := map[string]bool{}
	var files []types.FileEntry
	err = filepath.WalkDir(skillsDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path != skillsDir && security.IsIgnoredName(entry.Name()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(skillsDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if err := security.ValidateSkillRelPath(rel); err != nil {
			return err
		}
		skill := firstPathSegment(rel)
		if len(selectedSet) > 0 && !selectedSet[skill] {
			return nil
		}
		hash, err := hashFile(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		files = append(files, types.FileEntry{Path: rel, SHA256: hash, Size: info.Size()})
		skillSet[skill] = true
		return nil
	})
	if err != nil {
		return types.ManifestResource{}, err
	}
	sort.Slice(files, func(i, j int) bool { return strings.Compare(files[i].Path, files[j].Path) < 0 })
	skills := make([]string, 0, len(skillSet))
	for skill := range skillSet {
		skills = append(skills, skill)
	}
	sort.Strings(skills)
	return types.ManifestResource{Enabled: true, Count: len(files), Files: files, Skills: skills}, nil
}

func selectedSkillSet(skills []string) map[string]bool {
	set := map[string]bool{}
	for _, skill := range skills {
		skill = strings.TrimSpace(skill)
		if skill != "" {
			set[skill] = true
		}
	}
	return set
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

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
