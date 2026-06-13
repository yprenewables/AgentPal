package security

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
)

func IsIgnoredName(name string) bool {
	switch name {
	case ".git", ".DS_Store", "__pycache__":
		return true
	default:
		return false
	}
}

func ValidateSkillRelPath(path string) error {
	if path == "" {
		return errors.New("skill path is empty")
	}
	if strings.Contains(path, "\\") {
		return errors.New("skill path must use forward slashes")
	}
	if filepath.IsAbs(path) || strings.HasPrefix(path, "/") {
		return errors.New("skill path must be relative")
	}
	if len(path) >= 2 && path[1] == ':' {
		return errors.New("skill path must not include a drive letter")
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return errors.New("skill path contains an invalid segment")
		}
	}
	return nil
}

func SafeJoin(base string, parts ...string) (string, error) {
	cleanBase, err := filepath.Abs(filepath.Clean(base))
	if err != nil {
		return "", err
	}
	joinedParts := append([]string{cleanBase}, parts...)
	cleanTarget, err := filepath.Abs(filepath.Clean(filepath.Join(joinedParts...)))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(cleanBase, cleanTarget)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes base directory")
	}
	if runtime.GOOS == "windows" && strings.Contains(rel, `\..\`) {
		return "", errors.New("path escapes base directory")
	}
	return cleanTarget, nil
}
