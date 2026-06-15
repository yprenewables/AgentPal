package peer

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"agentpal/internal/constants"
	"agentpal/internal/types"
)

type Client struct {
	HTTPClient *http.Client
}

func NewClient() Client {
	return Client{HTTPClient: &http.Client{Timeout: 60 * time.Second}}
}

func (c Client) Health(input string, port int) (types.PeerStatus, error) {
	baseURL, err := Normalize(input, port)
	if err != nil {
		return types.PeerStatus{}, err
	}
	var health types.HealthResponse
	if err := c.getJSON(baseURL+"/health", &health); err != nil {
		return types.PeerStatus{URL: baseURL}, err
	}
	if health.App != constants.AppName {
		return types.PeerStatus{URL: baseURL}, errors.New("peer is not AgentPal")
	}
	return types.PeerStatus{OK: true, URL: baseURL, Version: health.Version}, nil
}

func (c Client) Manifest(input string, port int) (types.RemoteManifest, error) {
	baseURL, err := Normalize(input, port)
	if err != nil {
		return types.RemoteManifest{}, err
	}
	var manifest types.RemoteManifest
	if err := c.getJSON(baseURL+"/manifest", &manifest); err != nil {
		return manifest, err
	}
	if manifest.Schema != 1 || manifest.App != constants.AppName {
		return manifest, errors.New("remote manifest is not a supported AgentPal manifest")
	}
	return manifest, nil
}

func (c Client) Download(input string, port int, remotePath string) (*http.Response, error) {
	baseURL, err := Normalize(input, port)
	if err != nil {
		return nil, err
	}
	return c.HTTPClient.Get(baseURL + "/files/" + remotePath)
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
	return json.NewDecoder(resp.Body).Decode(target)
}
