package central

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"agentpal/internal/constants"
	"agentpal/internal/types"
)

type Client struct{ HTTPClient *http.Client }

func NewClient(httpClient *http.Client, timeout time.Duration) Client {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	clone := *httpClient
	clone.Timeout = timeout
	return Client{HTTPClient: &clone}
}
func (c Client) Manifest(base string) (types.CenterManifest, error) {
	var result types.CenterManifest
	if err := c.getJSON(strings.TrimRight(base, "/")+"/v1/config/manifest", &result); err != nil {
		return result, err
	}
	if result.Schema != 1 || result.App != constants.AppName {
		return result, errors.New("remote manifest is not a supported AgentPal manifest")
	}
	return result, nil
}

func (c Client) UploadBackup(base, device, profile string, archive io.Reader, overwrite bool) error {
	url := strings.TrimRight(base, "/") + "/v1/auth-backups/" + device + "/" + profile
	if overwrite {
		url += "?overwrite=true"
	}
	req, err := http.NewRequest(http.MethodPost, url, archive)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return errors.New(resp.Status)
	}
	return nil
}

func (c Client) ListBackups(base string) ([]types.BackupInfo, error) {
	var result []types.BackupInfo
	if err := c.getJSON(strings.TrimRight(base, "/")+"/v1/auth-backups", &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c Client) DownloadBackup(base, device, profile string) ([]byte, error) {
	resp, err := c.HTTPClient.Get(strings.TrimRight(base, "/") + "/v1/auth-backups/" + device + "/" + profile)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 5<<20+1))
}

func (c Client) DeleteBackup(base, device, profile string) error {
	req, err := http.NewRequest(http.MethodDelete, strings.TrimRight(base, "/")+"/v1/auth-backups/"+device+"/"+profile, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return errors.New(resp.Status)
	}
	return nil
}
func (c Client) Health(base string) error {
	var value map[string]any
	return c.getJSON(strings.TrimRight(base, "/")+"/health", &value)
}
func (c Client) DownloadSkillFile(base, rel string) (*http.Response, error) {
	return c.HTTPClient.Get(strings.TrimRight(base, "/") + "/v1/config/files/skills/" + rel)
}
func (c Client) DownloadMCP(base, name string) (*http.Response, error) {
	return c.HTTPClient.Get(strings.TrimRight(base, "/") + "/v1/config/mcp/" + name)
}
func (c Client) getJSON(url string, target any) error {
	resp, err := c.HTTPClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New(resp.Status)
	}
	limited := io.LimitReader(resp.Body, 4<<20)
	return json.NewDecoder(limited).Decode(target)
}
