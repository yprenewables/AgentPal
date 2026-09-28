package central

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"agentpal/internal/authprofile"
	"agentpal/internal/types"
)

func TestServerAuthBackupRoundTrip(t *testing.T) {
	profileRoot := t.TempDir()
	store := authprofile.NewStore(profileRoot)
	profile, err := store.CreateAPIKey(authprofile.APIKeyRequest{Name: "Work", Provider: "openai", BaseURL: "https://api.openai.com/v1", APIKey: "sk-test"})
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	if err := authprofile.Pack(filepath.Join(profileRoot, profile.ID), &archive); err != nil {
		t.Fatal(err)
	}
	server := NewServer(types.ServerOptions{CodexDir: makeCodexDir(t), Port: 28888})
	server.BackupDir = filepath.Join(t.TempDir(), "backups")
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()
	client := authprofileHTTPClient{base: ts.URL, client: ts.Client()}
	if err := client.upload(profile.ID, archive.Bytes()); err != nil {
		t.Fatal(err)
	}
	resp, err := ts.Client().Get(ts.URL + "/v1/auth-backups")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil || !bytes.Contains(data, []byte(`"Work"`)) {
		t.Fatalf("list = %s, err=%v", data, err)
	}
	resp, err = ts.Client().Get(ts.URL + "/v1/auth-backups/device/" + profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("download status = %d", resp.StatusCode)
	}
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/v1/auth-backups/device/"+profile.ID, nil)
	resp, err = ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d", resp.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(server.BackupDir, "device", profile.ID)); !os.IsNotExist(err) {
		t.Fatalf("backup still exists: %v", err)
	}
}

type authprofileHTTPClient struct {
	base   string
	client *http.Client
}

func (c authprofileHTTPClient) upload(id string, data []byte) error {
	req, err := http.NewRequest(http.MethodPost, c.base+"/v1/auth-backups/device/"+id, bytes.NewReader(data))
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return os.ErrInvalid
	}
	return nil
}
