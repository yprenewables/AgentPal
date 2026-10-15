package centralclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
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

func (c Client) StartLogin(base string) (types.LoginStart, error) {
	var result types.LoginStart
	if err := c.postJSON(strings.TrimRight(base, "/")+"/v1/auth/login/start", map[string]any{}, &result); err != nil {
		return result, err
	}
	return result, nil
}

func (c Client) LoginStatus(base, loginID, pollToken string) (types.LoginStatus, error) {
	var result types.LoginStatus
	target := strings.TrimRight(base, "/") + "/v1/auth/login/status/" + url.PathEscape(loginID) + "?pollToken=" + url.QueryEscape(pollToken)
	if err := c.getJSON(target, &result); err != nil {
		return result, err
	}
	return result, nil
}

func (c Client) CentralAuthProfiles(base, token string) ([]types.RemoteAuthProfile, error) {
	var result []types.RemoteAuthProfile
	if err := c.getJSONAuth(strings.TrimRight(base, "/")+"/v1/auth-profiles", token, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c Client) CentralAuthProfile(base, token, name string) (types.RemoteAuthProfileDetail, error) {
	var result types.RemoteAuthProfileDetail
	target := strings.TrimRight(base, "/") + "/v1/auth-profiles/" + url.PathEscape(name)
	if err := c.getJSONAuth(target, token, &result); err != nil {
		return result, err
	}
	return result, nil
}

func (c Client) getJSONAuth(target, token string, value any) error {
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New(resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(value)
}

func (c Client) postJSON(target string, payload any, value any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Post(target, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return errors.New(resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(value)
}
