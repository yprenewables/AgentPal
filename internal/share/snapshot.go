package share

import (
	"io"
	"os"
	"path/filepath"

	"agentpal/internal/codex"
	"agentpal/internal/security"
	"agentpal/internal/types"
)

func CreateSnapshot(req types.ShareRequest) (string, error) {
	base, err := codex.ExpandPath(req.CodexDir)
	if err != nil {
		return "", err
	}
	snapshotDir, err := os.MkdirTemp("", "agentpal-share-*")
	if err != nil {
		return "", err
	}
	if err := copySelectedResources(base, snapshotDir, req); err != nil {
		_ = os.RemoveAll(snapshotDir)
		return "", err
	}
	return snapshotDir, nil
}

func copySelectedResources(sourceDir, snapshotDir string, req types.ShareRequest) error {
	if req.ShareConfig {
		if err := copyFileResource(sourceDir, snapshotDir, "config.toml"); err != nil {
			return err
		}
	}
	if req.ShareAuth {
		if err := copyFileResource(sourceDir, snapshotDir, "auth.json"); err != nil {
			return err
		}
	}
	if req.ShareSkills {
		if err := copySkillsResource(sourceDir, snapshotDir, req.Skills); err != nil {
			return err
		}
	}
	return nil
}

func copyFileResource(sourceDir, snapshotDir, rel string) error {
	source, err := security.SafeJoin(sourceDir, rel)
	if err != nil {
		return err
	}
	target, err := security.SafeJoin(snapshotDir, rel)
	if err != nil {
		return err
	}
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	return copyFile(source, target, info.Mode())
}

func copySkillsResource(sourceDir, snapshotDir string, selectedSkills []string) error {
	sourceSkills, err := security.SafeJoin(sourceDir, "skills")
	if err != nil {
		return err
	}
	selectedSet := selectedSkillSet(selectedSkills)
	return filepath.WalkDir(sourceSkills, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path != sourceSkills && security.IsIgnoredName(entry.Name()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(sourceSkills, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if err := security.ValidateSkillRelPath(rel); err != nil {
			return err
		}
		if len(selectedSet) > 0 && !selectedSet[firstPathSegment(rel)] {
			return nil
		}
		target, err := security.SafeJoin(snapshotDir, "skills", rel)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(source, target string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
