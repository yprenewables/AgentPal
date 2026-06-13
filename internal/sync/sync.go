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
		for _, file := range manifest.Shared.Skills.Files {
			if err := security.ValidateSkillRelPath(file.Path); err != nil {
				return err
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
		if err := atomicCopy(filepath.Join(tempDir, "config.toml"), filepath.Join(targetDir, "config.toml")); err != nil {
			return err
		}
	}
	if req.SyncAuth {
		if err := atomicCopy(filepath.Join(tempDir, "auth.json"), filepath.Join(targetDir, "auth.json")); err != nil {
			return err
		}
	}
	if req.SyncSkills {
		if err := os.RemoveAll(filepath.Join(targetDir, "skills")); err != nil {
			return err
		}
		if err := copyIfExists(filepath.Join(tempDir, "skills"), filepath.Join(targetDir, "skills")); err != nil {
			return err
		}
	}
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
