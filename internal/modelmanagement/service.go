package modelmanagement

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"agentpal/internal/modelcatalog"

	"github.com/pelletier/go-toml/v2"
)

const UnavailableMessage = "Model management is only available for API Key profiles. Switch to an API Key profile in Authentication."

type State struct {
	Available       bool              `json:"available"`
	Error           string            `json:"error,omitempty"`
	Provider        string            `json:"provider"`
	BaseURL         string            `json:"baseUrl"`
	DiscoveryError  string            `json:"discoveryError,omitempty"`
	EnrichmentError string            `json:"enrichmentError,omitempty"`
	UnmatchedModels []string          `json:"unmatchedModels"`
	FileValid       bool              `json:"fileValid"`
	Writable        bool              `json:"writable"`
	FileExists      bool              `json:"fileExists"`
	FileHash        string            `json:"fileHash"`
	Models          []json.RawMessage `json:"models"`
	RawModels       []string          `json:"rawModels"`
}
type SaveRequest struct {
	FileHash            string            `json:"fileHash"`
	Models              []json.RawMessage `json:"models"`
	RawModels           []string          `json:"rawModels,omitempty"`
	VisibilityOverrides map[int]string    `json:"visibilityOverrides,omitempty"`
	UpdateParams        bool              `json:"updateParams,omitempty"`
}

// CatalogSource 提供模型目录；默认实现会请求 OpenRouter。
type CatalogSource interface {
	Catalog(ctx context.Context) (*modelcatalog.Catalog, error)
}

// NewCatalogSource 可被测试替换，用于离线注入模型目录。
var NewCatalogSource = func() CatalogSource { return modelcatalog.NewClient() }

type Service struct {
	Dir     string
	Client  *http.Client
	Catalog CatalogSource
}

var saveMu sync.Mutex

func New(dir string) *Service {
	return &Service{Dir: dir, Client: &http.Client{Timeout: 10 * time.Second}}
}

// credentials only reads actual Codex files; keys never enter the public state.
func (s *Service) credentials() (provider, base, key string, err error) {
	data, err := os.ReadFile(filepath.Join(s.Dir, "auth.json"))
	if err != nil {
		return "", "", "", fmt.Errorf("read current authentication: %w", err)
	}
	var auth struct {
		Mode   string          `json:"auth_mode"`
		Key    string          `json:"OPENAI_API_KEY"`
		APIKey string          `json:"api_key"`
		Tokens json.RawMessage `json:"tokens"`
	}
	if err = json.Unmarshal(data, &auth); err != nil {
		return "", "", "", errors.New("current auth.json is invalid")
	}
	if auth.Mode == "chatgpt" {
		return "", "", "", errors.New(UnavailableMessage)
	}
	key = strings.TrimSpace(auth.Key)
	if key == "" {
		key = strings.TrimSpace(auth.APIKey)
	}
	if key == "" {
		if len(auth.Tokens) > 0 && string(auth.Tokens) != "null" {
			return "", "", "", errors.New(UnavailableMessage)
		}
		return "", "", "", errors.New("current API Key was not found in auth.json")
	}
	if auth.Mode != "" && auth.Mode != "apikey" && auth.Mode != "api_key" {
		return "", "", "", errors.New("current authentication type is not recognized")
	}
	data, err = os.ReadFile(filepath.Join(s.Dir, "config.toml"))
	if err != nil {
		return "", "", key, fmt.Errorf("read config.toml: %w", err)
	}
	var cfg struct {
		Provider  string `toml:"model_provider"`
		Providers map[string]struct {
			Name string `toml:"name"`
			Base string `toml:"base_url"`
		} `toml:"model_providers"`
	}
	if err = toml.Unmarshal(data, &cfg); err != nil {
		return "", "", key, errors.New("config.toml is invalid")
	}
	if cfg.Provider == "" {
		return "", "", key, errors.New("model_provider is missing from config.toml")
	}
	p := cfg.Providers[cfg.Provider]
	provider = p.Name
	if provider == "" {
		provider = cfg.Provider
	}
	base = p.Base
	if base == "" {
		return provider, base, key, errors.New("base_url is missing for the current model_provider")
	}
	u, e := url.Parse(base)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return provider, base, key, errors.New("current Provider base_url must be an HTTP(S) URL without credentials, query or fragment")
	}
	return provider, base, key, nil
}

func (s *Service) read() (map[string]json.RawMessage, []json.RawMessage, string, bool, error) {
	data, err := os.ReadFile(filepath.Join(s.Dir, "models.json"))
	if os.IsNotExist(err) {
		return map[string]json.RawMessage{}, []json.RawMessage{}, "missing", false, nil
	}
	if err != nil {
		return nil, nil, "", false, err
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	var top map[string]json.RawMessage
	if json.Unmarshal(data, &top) != nil || top == nil {
		return nil, nil, hash, true, errors.New("models.json must contain a valid JSON object; the original file will not be overwritten")
	}
	raw := bytes.TrimSpace(top["models"])
	var models []json.RawMessage
	if len(raw) == 0 || raw[0] != '[' || json.Unmarshal(raw, &models) != nil {
		return nil, nil, hash, true, errors.New("models.json models must be an array; the original file will not be overwritten")
	}
	return top, models, hash, true, nil
}

func (s *Service) State() State {
	state := State{Models: []json.RawMessage{}, RawModels: []string{}, UnmatchedModels: []string{}}
	provider, base, key, err := s.credentials()
	if key == "" {
		if err != nil {
			state.Error = err.Error()
		}
		return state
	}
	// Authentication is known even if discovery configuration is incomplete.
	state.Available = true
	state.Provider = provider
	state.BaseURL = base
	if err != nil {
		state.DiscoveryError = err.Error()
	}
	_, models, hash, exists, err := s.read()
	state.FileHash = hash
	state.FileExists = exists
	if err != nil {
		state.Error = err.Error()
		return state
	}
	state.FileValid = true
	state.Models = models
	for _, model := range models {
		state.RawModels = append(state.RawModels, string(model))
	}
	// Probe the directory's actual ability to create the same-directory temporary file.
	f, err := os.CreateTemp(s.Dir, ".models-check-")
	if err != nil {
		state.Error = "models.json is not writable: " + err.Error()
		return state
	}
	name := f.Name()
	closeErr := f.Close()
	removeErr := os.Remove(name)
	if closeErr != nil || removeErr != nil {
		state.Error = "could not check models.json write access"
		return state
	}
	state.Writable = true
	return state
}

func (s *Service) Discover() ([]string, error) {
	_, base, key, err := s.credentials()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(base, "/")+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, errors.New("model discovery request failed or timed out")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("model discovery returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, errors.New("could not read model discovery response")
	}
	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal(data, &result) != nil || result.Data == nil {
		return nil, errors.New("invalid model discovery response: expected data array")
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, m := range result.Data {
		if strings.TrimSpace(m.ID) == "" {
			return nil, errors.New("invalid model discovery response: missing model id")
		}
		if !seen[m.ID] {
			seen[m.ID] = true
			ids = append(ids, m.ID)
		}
	}
	return ids, nil
}

//go:embed template.json
var templateJSON []byte

func Template(id string) json.RawMessage {
	var model map[string]json.RawMessage
	_ = json.Unmarshal(templateJSON, &model)
	model["slug"], _ = json.Marshal(id)
	model["display_name"] = model["slug"]
	data, _ := json.Marshal(model)
	return data
}
func Merge(models []json.RawMessage, ids []string) ([]json.RawMessage, int) {
	result := append([]json.RawMessage{}, models...)
	seen := map[string]bool{}
	for _, raw := range models {
		var m struct {
			Slug string `json:"slug"`
		}
		_ = json.Unmarshal(raw, &m)
		seen[m.Slug] = true
	}
	added := 0
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			result = append(result, Template(id))
			added++
		}
	}
	return result, added
}

func (s *Service) Save(req SaveRequest) (State, error) {
	saveMu.Lock()
	defer saveMu.Unlock()
	if req.RawModels != nil {
		req.Models = make([]json.RawMessage, len(req.RawModels))
		for i, raw := range req.RawModels {
			if !json.Valid([]byte(raw)) {
				return State{}, errors.New("invalid model draft JSON")
			}
			req.Models[i] = json.RawMessage(raw)
		}
	}
	for index, visibility := range req.VisibilityOverrides {
		if index < 0 || index >= len(req.Models) || (visibility != "list" && visibility != "hide") {
			return State{}, errors.New("invalid visibility override")
		}
		var model map[string]json.RawMessage
		if json.Unmarshal(req.Models[index], &model) != nil || model == nil {
			return State{}, errors.New("visibility can only be changed on model objects")
		}
		model["visibility"], _ = json.Marshal(visibility)
		req.Models[index], _ = json.Marshal(model)
	}
	state := s.State()
	if !state.Available || !state.FileValid || !state.Writable {
		return State{}, fmt.Errorf("cannot save models: %s", state.Error)
	}
	top, _, hash, _, err := s.read()
	if err != nil {
		return State{}, err
	}
	if hash != req.FileHash {
		return State{}, errors.New("models.json changed since loading. Reload Models before saving")
	}
	if req.Models == nil {
		req.Models = []json.RawMessage{}
	}
	// Upgrade catalogs produced by older AgentPal versions without replacing custom metadata.
	var defaults map[string]json.RawMessage
	_ = json.Unmarshal(templateJSON, &defaults)
	for i, raw := range req.Models {
		var model map[string]json.RawMessage
		if json.Unmarshal(raw, &model) != nil || model == nil {
			return State{}, errors.New("model catalog entries must be objects")
		}
		for key, value := range defaults {
			if _, exists := model[key]; !exists {
				model[key] = value
			}
		}
		req.Models[i], _ = json.Marshal(model)
	}
	report := enrichModels(req.Models, req.UpdateParams, s.catalogSource())
	raw, err := json.Marshal(req.Models)
	if err != nil {
		return State{}, err
	}
	top["models"] = raw
	data, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return State{}, err
	}
	data = append(data, '\n')
	configPath := filepath.Join(s.Dir, "config.toml")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return State{}, err
	}
	catalogPath, err := filepath.Abs(filepath.Join(s.Dir, "models.json"))
	if err != nil {
		return State{}, err
	}
	updatedConfig, err := patchConfigValues(configData, map[string]string{"model_catalog_json": catalogPath})
	if err != nil {
		return State{}, fmt.Errorf("cannot configure model catalog: %w", err)
	}
	var configTemp string
	if !bytes.Equal(configData, updatedConfig) {
		cf, err := os.CreateTemp(s.Dir, ".model-catalog-")
		if err != nil {
			return State{}, err
		}
		configTemp = cf.Name()
		defer os.Remove(configTemp)
		if err = cf.Chmod(0600); err == nil {
			_, err = cf.Write(updatedConfig)
		}
		if err == nil {
			err = cf.Sync()
		}
		closeErr := cf.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return State{}, err
		}
	}
	f, err := os.CreateTemp(s.Dir, ".models-")
	if err != nil {
		return State{}, err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return State{}, err
	}
	// Detect external edits during preparation, immediately before the atomic replacement.
	_, _, current, _, err := s.read()
	if err != nil {
		return State{}, err
	}
	if current != req.FileHash {
		return State{}, errors.New("models.json changed since loading. Reload Models before saving")
	}
	currentConfig, err := os.ReadFile(configPath)
	if err != nil {
		return State{}, err
	}
	if !bytes.Equal(currentConfig, configData) {
		return State{}, errors.New("config.toml changed during save. Reload Models before saving")
	}
	if err = os.Rename(f.Name(), filepath.Join(s.Dir, "models.json")); err != nil {
		return State{}, err
	}
	if configTemp != "" {
		if err = os.Rename(configTemp, configPath); err != nil {
			return State{}, fmt.Errorf("models saved, but model catalog configuration failed; reload and save again: %w", err)
		}
	}
	result := s.State()
	result.EnrichmentError = report.err
	result.UnmatchedModels = report.unmatched
	return result, nil
}

type enrichmentReport struct {
	err       string
	unmatched []string
}

func (s *Service) catalogSource() CatalogSource {
	if s.Catalog != nil {
		return s.Catalog
	}
	return NewCatalogSource()
}

// enrichModels 按需从模型目录补全参数；失败或未匹配时保留本地值。
func enrichModels(models []json.RawMessage, enabled bool, source CatalogSource) enrichmentReport {
	report := enrichmentReport{unmatched: []string{}}
	if !enabled {
		return report
	}
	catalog, err := source.Catalog(context.Background())
	if err != nil {
		report.err = err.Error()
		return report
	}
	for i, raw := range models {
		var model map[string]json.RawMessage
		if json.Unmarshal(raw, &model) != nil || model == nil {
			continue
		}
		var slug string
		_ = json.Unmarshal(model["slug"], &slug)
		info, ok := catalog.Lookup(slug)
		if !ok {
			report.unmatched = append(report.unmatched, slug)
			continue
		}
		modelcatalog.Apply(model, info)
		models[i], _ = json.Marshal(model)
	}
	return report
}
