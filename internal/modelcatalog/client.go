package modelcatalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// DefaultURL 是模型目录接口地址。
const DefaultURL = "https://openrouter.ai/api/v1/models"

// Client 拉取并按 TTL 缓存模型目录。
type Client struct {
	HTTP *http.Client
	URL  string
	TTL  time.Duration

	mu      sync.Mutex
	cached  *Catalog
	expires time.Time
}

type apiResponse struct {
	Data []apiModel `json:"data"`
}

type apiModel struct {
	ID            string `json:"id"`
	CanonicalSlug string `json:"canonical_slug"`
	ContextLength int64  `json:"context_length"`
	Architecture  struct {
		InputModalities []string `json:"input_modalities"`
	} `json:"architecture"`
	Reasoning *struct {
		SupportedEfforts []string `json:"supported_efforts"`
		DefaultEffort    string   `json:"default_effort"`
	} `json:"reasoning"`
}

// NewClient 返回使用默认地址与 10 分钟缓存的客户端。
func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 10 * time.Second}, URL: DefaultURL, TTL: 10 * time.Minute}
}

// Catalog 返回模型目录，命中缓存时不发起请求。
func (c *Client) Catalog(ctx context.Context) (*Catalog, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cached != nil && time.Now().Before(c.expires) {
		return c.cached, nil
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	target := c.URL
	if target == "" {
		target = DefaultURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, errors.New("model catalog request failed or timed out")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("model catalog returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, errors.New("could not read model catalog response")
	}
	var payload apiResponse
	if json.Unmarshal(data, &payload) != nil || payload.Data == nil {
		return nil, errors.New("invalid model catalog response: expected data array")
	}
	entries := make([]Entry, 0, len(payload.Data))
	for _, m := range payload.Data {
		if strings.TrimSpace(m.ID) == "" {
			continue
		}
		var efforts []string
		var defaultEffort string
		if m.Reasoning != nil {
			efforts = m.Reasoning.SupportedEfforts
			defaultEffort = m.Reasoning.DefaultEffort
		}
		entry, ok := BuildInfo(m.ID, m.CanonicalSlug, m.ContextLength, m.Architecture.InputModalities, efforts, defaultEffort)
		if ok {
			entries = append(entries, entry)
		}
	}
	catalog := NewCatalog(entries)
	c.cached = catalog
	ttl := c.TTL
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	c.expires = time.Now().Add(ttl)
	return catalog, nil
}
