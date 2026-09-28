package authprofile

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

func Pack(profileDir string, writer io.Writer) error {
	z := zip.NewWriter(writer)
	for _, name := range []string{"profile.json", "auth.json", "config-auth.toml"} {
		data, err := os.ReadFile(filepath.Join(profileDir, name))
		if err != nil {
			_ = z.Close()
			return err
		}
		entry, err := z.Create(name)
		if err != nil {
			_ = z.Close()
			return err
		}
		if _, err := entry.Write(data); err != nil {
			_ = z.Close()
			return err
		}
	}
	return z.Close()
}

func Unpack(reader io.ReaderAt, size int64, targetRoot string) (AuthProfile, error) {
	if size > 5<<20 {
		return AuthProfile{}, errors.New("profile archive is too large")
	}
	z, err := zip.NewReader(reader, size)
	if err != nil {
		return AuthProfile{}, err
	}
	allowed := map[string]bool{"profile.json": true, "auth.json": true, "config-auth.toml": true}
	files := map[string][]byte{}
	for _, entry := range z.File {
		if !allowed[entry.Name] || strings.Contains(entry.Name, "..") || strings.HasPrefix(entry.Name, "/") {
			return AuthProfile{}, errors.New("invalid profile archive entry")
		}
		r, err := entry.Open()
		if err != nil {
			return AuthProfile{}, err
		}
		data, err := io.ReadAll(io.LimitReader(r, 2<<20+1))
		_ = r.Close()
		if err != nil || len(data) > 2<<20 {
			return AuthProfile{}, errors.New("profile archive entry is too large")
		}
		files[entry.Name] = data
	}
	for name := range allowed {
		if _, ok := files[name]; !ok {
			return AuthProfile{}, errors.New("profile archive is missing " + name)
		}
	}
	var profile AuthProfile
	if err := json.Unmarshal(files["profile.json"], &profile); err != nil {
		return AuthProfile{}, err
	}
	var auth any
	if err := json.Unmarshal(files["auth.json"], &auth); err != nil {
		return AuthProfile{}, err
	}
	var config map[string]any
	if err := toml.Unmarshal(files["config-auth.toml"], &config); err != nil {
		return AuthProfile{}, err
	}
	if profile.ID == "" || profile.Name == "" {
		return AuthProfile{}, errors.New("profile metadata is incomplete")
	}
	dir := filepath.Join(targetRoot, profile.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return AuthProfile{}, err
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			return AuthProfile{}, err
		}
	}
	return profile, nil
}
