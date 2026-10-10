// Package centralscan 提供中心节点与客户端共用的技能目录扫描与哈希逻辑。
package centralscan

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sort"

	"agentpal/internal/security"
	"agentpal/internal/types"
)

// ScanSkills 扫描 skills 根目录，按一级子目录聚合出技能清单并计算内容哈希。
func ScanSkills(root string, scanErrors []types.ScanError) ([]types.CenterSkill, []types.ScanError) {
	groups := map[string][]types.FileEntry{}
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			scanErrors = append(scanErrors, types.ScanError{Path: path, Error: walkErr.Error()})
			return nil
		}
		if path != root && security.IsIgnoredName(entry.Name()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			scanErrors = append(scanErrors, types.ScanError{Path: path, Error: err.Error()})
			return nil
		}
		rel = filepath.ToSlash(rel)
		if err := security.ValidateSkillRelPath(rel); err != nil {
			scanErrors = append(scanErrors, types.ScanError{Path: rel, Error: err.Error()})
			return nil
		}
		hash, size, err := hashFile(path)
		if err != nil {
			scanErrors = append(scanErrors, types.ScanError{Path: rel, Error: err.Error()})
			return nil
		}
		name := firstSegment(rel)
		groups[name] = append(groups[name], types.FileEntry{Path: rel, SHA256: hash, Size: size})
		return nil
	})
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]types.CenterSkill, 0, len(names))
	for _, name := range names {
		files := groups[name]
		sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
		h := sha256.New()
		var size int64
		for _, file := range files {
			_, _ = io.WriteString(h, file.Path+":"+file.SHA256+"\n")
			size += file.Size
		}
		result = append(result, types.CenterSkill{Name: name, SHA256: hex.EncodeToString(h.Sum(nil)), Files: files, Size: size})
	}
	return result, scanErrors
}

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func firstSegment(path string) string {
	for i, c := range path {
		if c == '/' {
			return path[:i]
		}
	}
	return path
}
