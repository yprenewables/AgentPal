package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"agentpal/internal/codex"
	appconfig "agentpal/internal/config"
	"agentpal/internal/peer"
	"agentpal/internal/security"
	"agentpal/internal/types"
)

func FromPeer(req types.SyncRequest) (types.SyncResult, error) {
	client := peer.NewClient()
	manifest, err := client.Manifest(req.PeerIP, req.Port)
	if err != nil {
		return types.SyncResult{}, err
	}
	targetDir, err := codex.ExpandPath(req.TargetDir)
	if err != nil {
		return types.SyncResult{}, err
	}
	if targetDir == "" {
		targetDir, err = codex.ExpandPath(codex.DefaultDir())
		if err != nil {
			return types.SyncResult{}, err
		}
	}
	items, err := selectedItems(req, manifest)
	if err != nil {
		return types.SyncResult{}, err
	}
	req = withDefaultSelections(req, manifest)
	tempDir, err := os.MkdirTemp("", "agentpal-sync-*")
	if err != nil {
		return types.SyncResult{}, err
	}
	defer os.RemoveAll(tempDir)

	if err := downloadSelected(client, req, manifest, tempDir); err != nil {
		return types.SyncResult{}, err
	}
	backupPath, err := backupSelected(req, targetDir)
	if err != nil {
		return types.SyncResult{}, err
	}
	result := types.SyncResult{OK: true, Items: items, BackupPath: backupPath}
	if err := applySelected(req, targetDir, tempDir); err != nil {
		result.OK = false
		return result, errors.New("target resources were backed up but replacement failed; backup is at " + backupPath + ": " + err.Error())
	}
	baseURL, _ := peer.Normalize(req.PeerIP, req.Port)
	_ = appconfig.AppendHistory(appconfig.HistoryEntry{Time: time.Now().UTC(), Peer: baseURL, Target: targetDir, Items: items, Backup: backupPath})
	return result, nil
}

func selectedItems(req types.SyncRequest, manifest types.RemoteManifest) ([]string, error) {
	var items []string
	if req.SyncConfig {
		if !manifest.Shared.Config.Enabled {
			return nil, errors.New("remote manifest does not include config.toml")
		}
		items = append(items, "config.toml")
	}
	if req.SyncAuth {
		if !manifest.Shared.Auth.Enabled {
			return nil, errors.New("remote manifest does not include auth.json")
		}
		items = append(items, "auth.json")
	}
	if req.SyncSkills {
		if !manifest.Shared.Skills.Enabled {
			return nil, errors.New("remote manifest does not include skills")
		}
		items = append(items, "skills")
	}
	if len(items) == 0 {
		return nil, errors.New("select at least one resource to sync")
	}
	return items, nil
}

func withDefaultSelections(req types.SyncRequest, manifest types.RemoteManifest) types.SyncRequest {
	if req.SyncConfig && len(req.ConfigRootKeys) == 0 && len(req.ConfigTables) == 0 {
		req.ConfigRootKeys = manifest.Shared.Config.RootKeys
		req.ConfigTables = manifest.Shared.Config.Tables
	}
	if req.SyncSkills && len(req.Skills) == 0 {
		req.Skills = manifest.Shared.Skills.Skills
	}
	return req
}

func downloadSelected(client peer.Client, req types.SyncRequest, manifest types.RemoteManifest, tempDir string) error {
	if req.SyncConfig {
		if err := downloadFile(client, req, "config.toml", filepath.Join(tempDir, "config.toml"), manifest.Shared.Config.SHA256); err != nil {
			return err
		}
	}
	if req.SyncAuth {
		if err := downloadFile(client, req, "auth.json", filepath.Join(tempDir, "auth.json"), manifest.Shared.Auth.SHA256); err != nil {
			return err
		}
	}
	if req.SyncSkills {
		selectedSkills := stringSet(req.Skills)
		for _, file := range manifest.Shared.Skills.Files {
			if err := security.ValidateSkillRelPath(file.Path); err != nil {
				return err
			}
			if len(selectedSkills) > 0 && !selectedSkills[firstPathSegment(file.Path)] {
				continue
			}
			target, err := security.SafeJoin(filepath.Join(tempDir, "skills"), file.Path)
			if err != nil {
				return err
			}
			if err := downloadFile(client, req, "skills/"+file.Path, target, file.SHA256); err != nil {
				return err
			}
		}
	}
	return nil
}

func downloadFile(client peer.Client, req types.SyncRequest, remotePath, localPath, wantHash string) error {
	resp, err := client.Download(req.PeerIP, req.Port, remotePath)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("download failed for " + remotePath + ": " + resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(localPath), 0o700); err != nil {
		return err
	}
	file, err := os.Create(localPath)
	if err != nil {
		return err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(file, hash), resp.Body)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	gotHash := hex.EncodeToString(hash.Sum(nil))
	if gotHash != wantHash {
		return errors.New("downloaded " + remotePath + " failed sha256 verification")
	}
	return nil
}

func backupSelected(req types.SyncRequest, targetDir string) (string, error) {
	backupRoot, err := appconfig.BackupRoot()
	if err != nil {
		return "", err
	}
	backupPath := filepath.Join(backupRoot, time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(backupPath, 0o700); err != nil {
		return "", err
	}
	if req.SyncConfig {
		if err := copyIfExists(filepath.Join(targetDir, "config.toml"), filepath.Join(backupPath, "config.toml")); err != nil {
			return "", err
		}
	}
	if req.SyncAuth {
		if err := copyIfExists(filepath.Join(targetDir, "auth.json"), filepath.Join(backupPath, "auth.json")); err != nil {
			return "", err
		}
	}
	if req.SyncSkills {
		if err := copyIfExists(filepath.Join(targetDir, "skills"), filepath.Join(backupPath, "skills")); err != nil {
			return "", err
		}
	}
	return backupPath, nil
}

func applySelected(req types.SyncRequest, targetDir, tempDir string) error {
	if err := os.MkdirAll(targetDir, 0o700); err != nil {
		return err
	}
	if req.SyncConfig {
		if err := applyConfig(req, filepath.Join(tempDir, "config.toml"), filepath.Join(targetDir, "config.toml")); err != nil {
			return err
		}
	}
	if req.SyncAuth {
		if err := atomicCopy(filepath.Join(tempDir, "auth.json"), filepath.Join(targetDir, "auth.json")); err != nil {
			return err
		}
	}
	if req.SyncSkills {
		if err := applySkills(req, filepath.Join(tempDir, "skills"), filepath.Join(targetDir, "skills")); err != nil {
			return err
		}
	}
	return nil
}

func applySkills(req types.SyncRequest, sourceDir, targetDir string) error {
	if len(req.Skills) == 0 {
		return nil
	}
	if err := os.MkdirAll(targetDir, 0o700); err != nil {
		return err
	}
	for _, skill := range req.Skills {
		if err := security.ValidateSkillRelPath(skill); err != nil {
			return err
		}
		sourcePath, err := security.SafeJoin(sourceDir, skill)
		if err != nil {
			return err
		}
		if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		targetPath, err := security.SafeJoin(targetDir, skill)
		if err != nil {
			return err
		}
		if err := os.RemoveAll(targetPath); err != nil {
			return err
		}
		if err := copyIfExists(sourcePath, targetPath); err != nil {
			return err
		}
	}
	return nil
}

func applyConfig(req types.SyncRequest, sourcePath, targetPath string) error {
	tmp := targetPath + ".agentpal.tmp"
	if err := copyIfExists(targetPath, tmp); err != nil {
		return err
	}
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		return codex.MergeConfigSections(sourcePath, targetPath, req.ConfigRootKeys, req.ConfigTables)
	}
	if err := codex.MergeConfigSections(sourcePath, targetPath, req.ConfigRootKeys, req.ConfigTables); err != nil {
		if _, restoreErr := os.Stat(tmp); restoreErr == nil {
			_ = os.Rename(tmp, targetPath)
		}
		return err
	}
	_ = os.Remove(tmp)
	return nil
}

func copyIfExists(src, dst string) error {
	info, err := os.Stat(src)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyDir(src, dst)
	}
	return copyFile(src, dst, info.Mode())
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		if entry.Type().IsRegular() {
			return copyFile(path, target, info.Mode())
		}
		return nil
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
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

func atomicCopy(src, dst string) error {
	tmp := dst + ".agentpal.tmp"
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := copyFile(src, tmp, info.Mode()); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func stringSet(values []string) map[string]bool {
	set := map[string]bool{}
	for _, value := range values {
		if value != "" {
			set[value] = true
		}
	}
	return set
}

func firstPathSegment(path string) string {
	for idx, char := range path {
		if char == '/' {
			return path[:idx]
		}
	}
	return path
}
