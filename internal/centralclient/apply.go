package centralclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"agentpal/internal/types"
)

func Apply(ctx context.Context, client Client, base, target string, manifest types.CenterManifest, changes []types.ApplyChange) (types.ApplyResult, error) {
	if len(changes) == 0 {
		return types.ApplyResult{}, errors.New("no configuration changes selected")
	}
	available := make(map[string]bool, len(manifest.Skills))
	for _, skill := range manifest.Skills {
		available[skill.Name] = true
	}
	for _, change := range changes {
		if change.Kind != "skill" || change.Action != "apply" {
			return types.ApplyResult{}, errors.New("configuration only supports downloading and updating skills from Central")
		}
		if !available[change.Name] {
			return types.ApplyResult{}, fmt.Errorf("skill %q is not available from Central", change.Name)
		}
	}
	temp, err := os.MkdirTemp("", "agentpal-config-")
	if err != nil {
		return types.ApplyResult{}, err
	}
	defer os.RemoveAll(temp)
	selected := map[string]types.ApplyChange{}
	for _, change := range changes {
		selected[change.Kind+":"+change.Name] = change
	}
	for _, skill := range manifest.Skills {
		if change, ok := selected["skill:"+skill.Name]; ok && change.Action == "apply" {
			for _, file := range skill.Files {
				if err := downloadVerified(ctx, client, base, "skills/"+file.Path, filepath.Join(temp, "skills", file.Path), file.SHA256); err != nil {
					return types.ApplyResult{}, err
				}
			}
		}
	}
	result := types.ApplyResult{OK: true}
	for _, change := range changes {
		key := change.Kind + ":" + change.Name
		src := filepath.Join(temp, "skills", change.Name)
		dst := filepath.Join(target, "skills", change.Name)
		if err := os.RemoveAll(dst); err != nil {
			return types.ApplyResult{}, err
		}
		if err := copyIfExists(src, dst); err != nil {
			return types.ApplyResult{}, err
		}
		result.Applied = append(result.Applied, key)
	}
	return result, nil
}
func downloadVerified(ctx context.Context, client Client, base, remote, local, want string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/config/files/"+remote, nil)
	if err != nil {
		return err
	}
	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return errors.New(resp.Status)
	}
	return saveHash(resp.Body, local, want)
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
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if entry.Type().IsRegular() {
			return copyFile(path, target, 0o600)
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
func saveHash(reader io.Reader, path, want string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(f, h), reader)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != want {
		return fmt.Errorf("downloaded %s failed sha256 verification", path)
	}
	return nil
}
