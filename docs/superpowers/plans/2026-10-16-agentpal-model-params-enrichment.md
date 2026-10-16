# 模型参数补全与 models.json 手动编辑 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 AgentPal 的 Models 页支持手动编辑 `models.json`，并在保存时按需从 OpenRouter 补全推理档位、输入模态与上下文长度。

**Architecture:** 新增纯逻辑包 `internal/modelcatalog`（匹配、字段映射、带缓存的 HTTP 拉取），由 `internal/modelmanagement.Service` 在 `Save` 中按 `UpdateParams` 调用；前端 `ModelsTab` 增加 `Update params` 复选框、结果提示与 JSON 编辑视图。

**Tech Stack:** Go 1.x（`net/http`、`encoding/json`、`sync`）、Wails v2 绑定、React + TypeScript。

## Global Constraints

- 不引入任何第三方依赖，只用标准库与既有依赖。
- 只写入 `input_modalities`、`context_window`、`max_context_window`、`supported_reasoning_levels`、`default_reasoning_level` 五个字段；**不写任何输出相关字段**。
- `openai`、`anthropic` provider 的上下文长度固定 `272000`。
- Codex 支持的推理档位白名单：`none`、`minimal`、`low`、`medium`、`high`、`xhigh`、`max`、`ultra`、`persistent`。
- 接口不可用或模型未匹配时，保留本地值并继续保存，不得阻断。
- 不做跨命名体系模糊匹配。
- 用户可见文案沿用现有中英混排风格（界面为英文，规范文档为中文）。

---

### Task 1: modelcatalog 归一化匹配

**Files:**
- Create: `internal/modelcatalog/catalog.go`
- Test: `internal/modelcatalog/catalog_test.go`

**Interfaces:**
- Consumes: 无。
- Produces:
  - `type Info struct { Slug, Provider string; ContextWindow int64; InputModalities, ReasoningEfforts []string; DefaultEffort string }`
  - `type Entry struct { ID, Canonical, Provider string; Info Info }`
  - `type Catalog struct` 与 `func NewCatalog(entries []Entry) *Catalog`
  - `func (c *Catalog) Lookup(slug string) (Info, bool)`

- [ ] **Step 1: Write the failing test**

Create `internal/modelcatalog/catalog_test.go`:

```go
package modelcatalog

import "testing"

func TestLookupMatchesDirectAndNormalizedIDs(t *testing.T) {
	c := NewCatalog([]Entry{
		{ID: "anthropic/claude-opus-5.5", Provider: "anthropic", Info: Info{Slug: "claude-opus-5.5", Provider: "anthropic"}},
		{ID: "anthropic/claude-opus-5.5:batch", Provider: "anthropic", Info: Info{Slug: "claude-opus-5.5", Provider: "batch"}},
		{ID: "~deepseek/deepseek-flash-latest", Provider: "deepseek", Info: Info{Slug: "deepseek-flash", Provider: "deepseek"}},
		{ID: "qwen/qwen3.8-27b", Provider: "qwen", Info: Info{Slug: "qwen3.8-27b", Provider: "qwen"}},
		{ID: "openai/gpt-6-sol", Provider: "openai", Info: Info{Slug: "gpt-6-sol", Provider: "openai"}},
	})
	for _, tc := range []struct{ slug, provider string }{
		{"claude-opus-5.5", "anthropic"},
		{"claude-opus-5-5", "anthropic"},
		{"deepseek-flash", "deepseek"},
		{"qwen3.8:27b", "qwen"},
		{"gpt-6-sol", "openai"},
	} {
		info, ok := c.Lookup(tc.slug)
		if !ok || info.Provider != tc.provider {
			t.Fatalf("Lookup(%q) info=%+v ok=%v", tc.slug, info, ok)
		}
	}
	if _, ok := c.Lookup("gpt-image-2"); ok {
		t.Fatal("matched an unknown model")
	}
}

func TestLookupRejectsAmbiguousMatches(t *testing.T) {
	c := NewCatalog([]Entry{
		{ID: "a/shared-name", Provider: "a", Info: Info{Slug: "shared-name", Provider: "a"}},
		{ID: "b/shared-name", Provider: "b", Info: Info{Slug: "shared-name", Provider: "b"}},
	})
	if _, ok := c.Lookup("shared-name"); ok {
		t.Fatal("ambiguous match was accepted")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/modelcatalog -run TestLookup -v`
Expected: FAIL（`undefined: NewCatalog`）

- [ ] **Step 3: Write minimal implementation**

Create `internal/modelcatalog/catalog.go`:

```go
package modelcatalog

import (
	"regexp"
	"sort"
	"strings"
)

type Info struct {
	Slug             string
	Provider         string
	ContextWindow    int64
	InputModalities  []string
	ReasoningEfforts []string
	DefaultEffort    string
}

type Entry struct {
	ID        string
	Canonical string
	Provider  string
	Info      Info
}

type Catalog struct{ entries []Entry }

func NewCatalog(entries []Entry) *Catalog { return &Catalog{entries: entries} }

var (
	variantSuffix = regexp.MustCompile(`:(batch|free|extended|thinking|online|nitro)$`)
	versionPair   = regexp.MustCompile(`-(\d+)-(\d+)`)
)

func normalized(name string) []string {
	s := strings.ToLower(strings.TrimSpace(name))
	base := strings.TrimPrefix(s, "~")
	base = variantSuffix.ReplaceAllString(base, "")
	base = strings.TrimSuffix(base, "-latest")
	set := map[string]bool{}
	for _, v := range []string{s, base} {
		set[v] = true
		set[versionPair.ReplaceAllString(v, "-$1.$2")] = true
		set[strings.ReplaceAll(v, ":", "-")] = true
	}
	for v := range set {
		set[versionPair.ReplaceAllString(v, "-$1.$2")] = true
	}
	out := make([]string, 0, len(set))
	for v := range set {
		if v != "" {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func intersects(a, b []string) bool {
	seen := map[string]bool{}
	for _, v := range a {
		seen[v] = true
	}
	for _, v := range b {
		if seen[v] {
			return true
		}
	}
	return false
}

func lastSegment(id string) string {
	if i := strings.LastIndex(id, "/"); i >= 0 {
		return id[i+1:]
	}
	return id
}

func (c *Catalog) Lookup(slug string) (Info, bool) {
	lower := strings.ToLower(strings.TrimSpace(slug))
	variants := normalized(slug)
	best, tie := 0, false
	var winner *Entry
	for i := range c.entries {
		e := &c.entries[i]
		score := 0
		for _, cand := range []string{lastSegment(e.ID), lastSegment(e.Canonical)} {
			if cand == "" {
				continue
			}
			if strings.ToLower(cand) == lower {
				score = 3
			} else if score < 2 && intersects(variants, normalized(cand)) {
				score = 2
			}
		}
		if score == 0 {
			continue
		}
		if strings.Contains(lastSegment(e.ID), ":") {
			score--
		}
		if strings.HasPrefix(e.ID, "~") {
			score--
		}
		switch {
		case score > best:
			best, winner, tie = score, e, false
		case score == best && winner != nil && winner.ID != e.ID:
			tie = true
		}
	}
	if winner == nil || tie {
		return Info{}, false
	}
	return winner.Info, true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/modelcatalog -run TestLookup -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/modelcatalog/catalog.go internal/modelcatalog/catalog_test.go
git commit -m "feat: match gateway model slugs to catalog entries"
```

---

### Task 2: modelcatalog 拉取与字段映射

**Files:**
- Create: `internal/modelcatalog/client.go`
- Modify: `internal/modelcatalog/catalog.go`（追加字段映射与常量）
- Test: `internal/modelcatalog/client_test.go`

**Interfaces:**
- Consumes: `Info`、`Entry`、`Catalog`、`NewCatalog`。
- Produces:
  - `const DefaultURL = "https://openrouter.ai/api/v1/models"`
  - `const ContextWindowOverride int64 = 272000`
  - `func CodexEfforts() []string`
  - `func BuildInfo(id, canonical string, contextLength int64, modalities, efforts []string, defaultEffort string) (Entry, bool)`
  - `type Client struct { HTTP *http.Client; URL string; TTL time.Duration }` 与 `func (c *Client) Catalog(ctx context.Context) (*Catalog, error)`

- [ ] **Step 1: Write the failing test**

Create `internal/modelcatalog/client_test.go`:

```go
package modelcatalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const fixture = `{"data":[
 {"id":"openai/gpt-6-sol","canonical_slug":"openai/gpt-6-sol-20260101","context_length":1050000,
  "architecture":{"input_modalities":["file","image","text"]},
  "reasoning":{"supported_efforts":["max","xhigh","high","medium","low","none"],"default_effort":"medium"}},
 {"id":"z-ai/glm-5.3","context_length":1048576,
  "architecture":{"input_modalities":["text"]},
  "reasoning":{"supported_efforts":["max","high","low","bogus"],"default_effort":"bogus"}},
 {"id":"anthropic/claude-opus-5","context_length":1000000,
  "architecture":{"input_modalities":["text","image","file"]},
  "reasoning":{"supported_efforts":["high"],"default_effort":"high"}}
]}`

func TestClientBuildsInfoWithOverridesAndFiltering(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(fixture))
	}))
	defer server.Close()
	c := &Client{HTTP: server.Client(), URL: server.URL, TTL: time.Hour}
	cat, err := c.Catalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	info, ok := cat.Lookup("gpt-6-sol")
	if !ok || info.ContextWindow != ContextWindowOverride || len(info.InputModalities) != 2 || info.InputModalities[0] != "text" || info.InputModalities[1] != "image" || info.DefaultEffort != "medium" {
		t.Fatalf("openai info=%+v ok=%v", info, ok)
	}
	glm, ok := cat.Lookup("glm-5.3")
	if !ok || glm.ContextWindow != 1048576 || glm.DefaultEffort != "" {
		t.Fatalf("glm info=%+v ok=%v", glm, ok)
	}
	if _, ok := cat.Lookup("claude-opus-5"); !ok {
		t.Fatal("anthropic model missing")
	}
	if _, err := c.Catalog(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("expected cached single request, got %d", requests)
	}
}

func TestClientSurfacesRequestFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	c := &Client{HTTP: server.Client(), URL: server.URL, TTL: time.Hour}
	if _, err := c.Catalog(context.Background()); err == nil {
		t.Fatal("expected error for non-2xx")
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{"))
	}))
	defer bad.Close()
	c = &Client{HTTP: bad.Client(), URL: bad.URL, TTL: time.Hour}
	if _, err := c.Catalog(context.Background()); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/modelcatalog -run TestClient -v`
Expected: FAIL（`undefined: Client`）

- [ ] **Step 3: Write minimal implementation**

Append to `internal/modelcatalog/catalog.go`:

```go
const ContextWindowOverride int64 = 272000

var codexEfforts = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra", "persistent"}

func CodexEfforts() []string { return append([]string{}, codexEfforts...) }

func isCodexEffort(value string) bool {
	for _, e := range codexEfforts {
		if e == value {
			return true
		}
	}
	return false
}

func filterModalities(in []string) []string {
	out := []string{}
	for _, m := range []string{"text", "image", "audio"} {
		for _, v := range in {
			if strings.EqualFold(v, m) {
				out = append(out, m)
				break
			}
		}
	}
	if len(out) == 0 {
		return []string{"text"}
	}
	return out
}

func filterEfforts(in []string) []string {
	out := []string{}
	for _, e := range in {
		if isCodexEffort(strings.ToLower(e)) {
			out = append(out, strings.ToLower(e))
		}
	}
	return out
}

func overridesContext(provider string) bool {
	return provider == "openai" || provider == "anthropic"
}

func BuildInfo(id, canonical string, contextLength int64, modalities, efforts []string, defaultEffort string) (Entry, bool) {
	if strings.TrimSpace(id) == "" {
		return Entry{}, false
	}
	provider := strings.ToLower(strings.SplitN(id, "/", 2)[0])
	slug := lastSegment(id)
	if canonical != "" {
		slug = lastSegment(canonical)
	}
	window := contextLength
	if overridesContext(provider) {
		window = ContextWindowOverride
	}
	info := Info{
		Slug:             slug,
		Provider:         provider,
		ContextWindow:    window,
		InputModalities:  filterModalities(modalities),
		ReasoningEfforts: filterEfforts(efforts),
	}
	if isCodexEffort(strings.ToLower(defaultEffort)) {
		info.DefaultEffort = strings.ToLower(defaultEffort)
	}
	return Entry{ID: id, Canonical: canonical, Provider: provider, Info: info}, true
}
```

Create `internal/modelcatalog/client.go`:

```go
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

const DefaultURL = "https://openrouter.ai/api/v1/models"

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

func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 10 * time.Second}, URL: DefaultURL, TTL: 10 * time.Minute}
}

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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/modelcatalog -v`
Expected: PASS（全部用例）

- [ ] **Step 5: Commit**

```bash
git add internal/modelcatalog
git commit -m "feat: fetch and map OpenRouter model metadata"
```

---

### Task 3: 字段写入（enrichment）

**Files:**
- Create: `internal/modelcatalog/enrich.go`
- Test: `internal/modelcatalog/enrich_test.go`

**Interfaces:**
- Consumes: `Info`、`CodexEfforts`。
- Produces: `func Apply(model map[string]json.RawMessage, info Info)`

- [ ] **Step 1: Write the failing test**

Create `internal/modelcatalog/enrich_test.go`:

```go
package modelcatalog

import (
	"encoding/json"
	"testing"
)

func decode(t *testing.T, model map[string]json.RawMessage, key string, into any) {
	t.Helper()
	raw, ok := model[key]
	if !ok {
		t.Fatalf("missing key %s", key)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatal(err)
	}
}

func TestApplyWritesOnlyRequestedFields(t *testing.T) {
	model := map[string]json.RawMessage{
		"slug":             json.RawMessage(`"gpt-6-sol"`),
		"visibility":       json.RawMessage(`"list"`),
		"default_reasoning_level": json.RawMessage(`"low"`),
		"output_modalities": json.RawMessage(`["text"]`),
	}
	Apply(model, Info{Slug: "gpt-6-sol", Provider: "openai", ContextWindow: 272000,
		InputModalities: []string{"text", "image"}, ReasoningEfforts: []string{"low", "high"}, DefaultEffort: "high"})

	var window, maxWindow int64
	decode(t, model, "context_window", &window)
	decode(t, model, "max_context_window", &maxWindow)
	if window != 272000 || maxWindow != 272000 {
		t.Fatalf("window=%d max=%d", window, maxWindow)
	}
	var modalities []string
	decode(t, model, "input_modalities", &modalities)
	if len(modalities) != 2 || modalities[0] != "text" || modalities[1] != "image" {
		t.Fatalf("modalities=%v", modalities)
	}
	var levels []struct {
		Effort      string `json:"effort"`
		Description string `json:"description"`
	}
	decode(t, model, "supported_reasoning_levels", &levels)
	if len(levels) != 2 || levels[1].Effort != "high" || levels[1].Description == "" {
		t.Fatalf("levels=%+v", levels)
	}
	var effort string
	decode(t, model, "default_reasoning_level", &effort)
	if effort != "high" {
		t.Fatalf("default=%q", effort)
	}
	if string(model["visibility"]) != `"list"` || string(model["slug"]) != `"gpt-6-sol"` || string(model["output_modalities"]) != `["text"]` {
		t.Fatalf("unrelated fields changed: %v", model)
	}
}

func TestApplyClearsDefaultEffortWhenUnknown(t *testing.T) {
	model := map[string]json.RawMessage{"default_reasoning_level": json.RawMessage(`"low"`)}
	Apply(model, Info{ContextWindow: 128000, InputModalities: []string{"text"}, ReasoningEfforts: nil})
	if _, ok := model["default_reasoning_level"]; ok {
		t.Fatal("default_reasoning_level was not removed")
	}
	var levels []any
	decode(t, model, "supported_reasoning_levels", &levels)
	if len(levels) != 0 {
		t.Fatalf("levels=%v", levels)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/modelcatalog -run TestApply -v`
Expected: FAIL（`undefined: Apply`）

- [ ] **Step 3: Write minimal implementation**

Create `internal/modelcatalog/enrich.go`:

```go
package modelcatalog

import "encoding/json"

var effortDescriptions = map[string]string{
	"none":       "No reasoning",
	"minimal":    "Minimal reasoning for the fastest responses",
	"low":        "Fast responses with lighter reasoning",
	"medium":     "Balances speed and reasoning depth for everyday tasks",
	"high":       "Greater reasoning depth for complex problems",
	"xhigh":      "Extra high reasoning depth for complex problems",
	"max":        "Maximum reasoning depth for the hardest problems",
	"ultra":      "Highest reasoning depth with extended compute",
	"persistent": "Reasoning that persists across the turn",
}

type reasoningLevel struct {
	Effort      string `json:"effort"`
	Description string `json:"description"`
}

func encode(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}

func Apply(model map[string]json.RawMessage, info Info) {
	model["input_modalities"] = encode(info.InputModalities)
	model["context_window"] = encode(info.ContextWindow)
	model["max_context_window"] = encode(info.ContextWindow)
	levels := make([]reasoningLevel, 0, len(info.ReasoningEfforts))
	for _, effort := range info.ReasoningEfforts {
		levels = append(levels, reasoningLevel{Effort: effort, Description: effortDescriptions[effort]})
	}
	model["supported_reasoning_levels"] = encode(levels)
	if info.DefaultEffort != "" {
		model["default_reasoning_level"] = encode(info.DefaultEffort)
	} else {
		delete(model, "default_reasoning_level")
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/modelcatalog -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/modelcatalog/enrich.go internal/modelcatalog/enrich_test.go
git commit -m "feat: write catalog metadata into model entries"
```

---

### Task 4: Save 集成与容错

**Files:**
- Modify: `internal/modelmanagement/service.go`
- Test: `internal/modelmanagement/service_test.go`

**Interfaces:**
- Consumes: `modelcatalog.Catalog`、`modelcatalog.Apply`、`modelcatalog.Info`。
- Produces:
  - `modelmanagement.SaveRequest` 新增 `UpdateParams bool \`json:"updateParams,omitempty"\``
  - `modelmanagement.State` 新增 `EnrichmentError string \`json:"enrichmentError,omitempty"\`` 与 `UnmatchedModels []string \`json:"unmatchedModels"\``
  - `type CatalogSource interface { Catalog(context.Context) (*modelcatalog.Catalog, error) }`
  - `var NewCatalogSource = func() CatalogSource { return modelcatalog.NewClient() }`
  - `Service` 新增可导出字段 `Catalog CatalogSource`

- [ ] **Step 1: Write the failing test**

追加到 `internal/modelmanagement/service_test.go`：

```go
type fakeCatalog struct {
	catalog *modelcatalog.Catalog
	err     error
	calls   int
}

func (f *fakeCatalog) Catalog(context.Context) (*modelcatalog.Catalog, error) {
	f.calls++
	return f.catalog, f.err
}

func catalogFixture(t *testing.T) *modelcatalog.Catalog {
	t.Helper()
	entry, ok := modelcatalog.BuildInfo("openai/gpt-6-sol", "", 1050000, []string{"file", "image", "text"}, []string{"high", "low"}, "high")
	if !ok {
		t.Fatal("fixture entry rejected")
	}
	return modelcatalog.NewCatalog([]modelcatalog.Entry{entry})
}

func TestSaveAppliesCatalogParameters(t *testing.T) {
	s := fixture(t, keyAuth, config, `{"models":[{"slug":"gpt-6-sol"},{"slug":"unknown-model"}]}`)
	s.Catalog = &fakeCatalog{catalog: catalogFixture(t)}
	state := s.State()
	saved, err := s.Save(SaveRequest{FileHash: state.FileHash, Models: state.Models, UpdateParams: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.UnmatchedModels) != 1 || saved.UnmatchedModels[0] != "unknown-model" {
		t.Fatalf("unmatched=%v", saved.UnmatchedModels)
	}
	var model map[string]json.RawMessage
	json.Unmarshal(saved.Models[0], &model)
	var window int64
	json.Unmarshal(model["context_window"], &window)
	if window != 272000 {
		t.Fatalf("context_window=%d", window)
	}
	if string(model["default_reasoning_level"]) != `"high"` {
		t.Fatalf("default=%s", model["default_reasoning_level"])
	}
	if _, ok := model["output_modalities"]; ok {
		t.Fatal("output field written")
	}
}

func TestSaveKeepsLocalValuesWhenCatalogUnavailable(t *testing.T) {
	s := fixture(t, keyAuth, config, `{"models":[{"slug":"gpt-6-sol","context_window":1234}]}`)
	source := &fakeCatalog{err: errors.New("model catalog request failed or timed out")}
	s.Catalog = source
	state := s.State()
	saved, err := s.Save(SaveRequest{FileHash: state.FileHash, Models: state.Models, UpdateParams: true})
	if err != nil {
		t.Fatal(err)
	}
	if saved.EnrichmentError == "" || len(saved.UnmatchedModels) != 0 {
		t.Fatalf("saved=%+v", saved)
	}
	var model map[string]json.RawMessage
	json.Unmarshal(saved.Models[0], &model)
	if string(model["context_window"]) != "1234" {
		t.Fatalf("local value overwritten: %s", model["context_window"])
	}
}

func TestSaveSkipsCatalogWhenNotRequested(t *testing.T) {
	s := fixture(t, keyAuth, config, `{"models":[{"slug":"gpt-6-sol"}]}`)
	source := &fakeCatalog{catalog: catalogFixture(t)}
	s.Catalog = source
	state := s.State()
	if _, err := s.Save(SaveRequest{FileHash: state.FileHash, Models: state.Models}); err != nil {
		t.Fatal(err)
	}
	if source.calls != 0 {
		t.Fatalf("catalog fetched %d times", source.calls)
	}
}
```

并在文件 import 块加入：

```go
	"context"
	"errors"

	"agentpal/internal/modelcatalog"
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/modelmanagement -run 'TestSave(Applies|Keeps|Skips)' -v`
Expected: FAIL（`saved.UnmatchedModels undefined`）

- [ ] **Step 3: Write minimal implementation**

在 `internal/modelmanagement/service.go` 的 import 块加入 `"context"` 与 `"agentpal/internal/modelcatalog"`，并修改结构体：

```go
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

type CatalogSource interface {
	Catalog(ctx context.Context) (*modelcatalog.Catalog, error)
}

var NewCatalogSource = func() CatalogSource { return modelcatalog.NewClient() }

type Service struct {
	Dir     string
	Client  *http.Client
	Catalog CatalogSource
}
```

在 `State()` 初始化处补上非空切片：

```go
	state := State{Models: []json.RawMessage{}, RawModels: []string{}, UnmatchedModels: []string{}}
```

在 `Save` 中，紧随「补齐模板默认字段」循环之后、`raw, err := json.Marshal(req.Models)` 之前插入：

```go
	report := enrichModels(req.Models, req.UpdateParams, s.catalogSource())
```

然后在 `Save` 结尾把报告合并进返回值：

```go
	result := s.State()
	result.EnrichmentError = report.err
	result.UnmatchedModels = report.unmatched
	return result, nil
```

新增文件底部辅助代码（同一文件内）：

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/modelmanagement -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/modelmanagement
git commit -m "feat: enrich saved models from the catalog"
```

---

### Task 5: 应用层贯通

**Files:**
- Modify: `app_models_test.go`
- Modify: `frontend/src/wails.ts`
- Test: `app_models_test.go`

**Interfaces:**
- Consumes: `modelmanagement.NewCatalogSource`、`modelmanagement.SaveRequest.UpdateParams`、`modelmanagement.State.UnmatchedModels`。
- Produces: 前端 `SaveModels(request)` 增加 `updateParams: boolean`；`ModelManagementState` 增加 `enrichmentError?: string`、`unmatchedModels: string[]`。

- [ ] **Step 1: Write the failing test**

追加到 `app_models_test.go`：

```go
type staticCatalog struct{ catalog *modelcatalog.Catalog }

func (s staticCatalog) Catalog(context.Context) (*modelcatalog.Catalog, error) {
	return s.catalog, nil
}

func TestSaveModelsPassesUpdateParamsThroughApp(t *testing.T) {
	entry, ok := modelcatalog.BuildInfo("openai/gpt-6-sol", "", 1050000, []string{"text"}, []string{"high"}, "high")
	if !ok {
		t.Fatal("fixture entry rejected")
	}
	modelmanagement.NewCatalogSource = func() modelmanagement.CatalogSource {
		return staticCatalog{catalog: modelcatalog.NewCatalog([]modelcatalog.Entry{entry})}
	}
	app, state, _ := setupCurrentAuthApp(t, `{"OPENAI_API_KEY":"secret"}`, "model_provider='corp'\n[model_providers.corp]\nbase_url='https://example.com/v1'")
	initial, err := app.GetModelManagementState()
	if err != nil {
		t.Fatal(err)
	}
	saved, err := app.SaveModels(modelmanagement.SaveRequest{FileHash: initial.FileHash, RawModels: []string{`{"slug":"gpt-6-sol"}`}, UpdateParams: true})
	if err != nil || len(saved.UnmatchedModels) != 0 {
		t.Fatalf("saved=%+v err=%v", saved, err)
	}
	data, err := os.ReadFile(filepath.Join(state.CodexDir, "models.json"))
	if err != nil || !strings.Contains(string(data), "272000") {
		t.Fatalf("data=%s err=%v", data, err)
	}
}
```

并在 import 块加入 `"context"` 与 `"agentpal/internal/modelcatalog"`。测试里替换全局钩子后要恢复：

```go
	original := modelmanagement.NewCatalogSource
	defer func() { modelmanagement.NewCatalogSource = original }()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run TestSaveModelsPassesUpdateParamsThroughApp -v`
Expected: FAIL（`undefined: staticCatalog` 或 `UpdateParams` 字段不存在）

- [ ] **Step 3: Write minimal implementation**

若 Task 4 已完成，Go 侧无需改动（`app_models.go` 直接透传 `SaveRequest`）。仅更新前端类型，`frontend/src/wails.ts`：

```ts
export type ModelManagementState = { available: boolean; error?: string; provider: string; baseUrl: string; discoveryError?: string; enrichmentError?: string; unmatchedModels: string[]; fileValid: boolean; writable: boolean; fileExists: boolean; fileHash: string; models: ModelEntry[]; rawModels: string[] };
```

并把 Backend 中的签名改为：

```ts
  SaveModels(request: { fileHash: string; rawModels: string[]; visibilityOverrides: Record<number, string>; updateParams: boolean }): Promise<ModelManagementState>;
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test . -run TestSaveModelsPassesUpdateParamsThroughApp -v && go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add app_models_test.go frontend/src/wails.ts
git commit -m "feat: expose catalog enrichment through the app bindings"
```

---

### Task 6: Models 页 Update params 与结果提示

**Files:**
- Modify: `frontend/src/ModelsTab.tsx`
- Modify: `frontend/src/style.css`

**Interfaces:**
- Consumes: `ModelManagementState.enrichmentError`、`ModelManagementState.unmatchedModels`。
- Produces: 供 Task 7 复用的 `draftFromModels`、`dirty` 计算与 `save()` 流程。

- [ ] **Step 1: 实现并构建**

在 `ModelsTab.tsx` 中：

新增状态：

```tsx
  const [updateParams, setUpdateParams] = useState(true);
```

在 `save()` 请求体中传入 `updateParams`：

```tsx
      const next = await api.SaveModels({ fileHash: state.fileHash, rawModels: draft.map((entry) => entry.raw), visibilityOverrides: Object.fromEntries(draft.flatMap((entry, index) => entry.visibility ? [[index, entry.visibility]] : [])), updateParams });
```

保存成功后的提示改为按结果生成：

```tsx
      const enriched = updateParams ? summarizeEnrichment(next) : null;
      setMessage({ error: false, text: enriched || 'Models saved and catalog configured. Restart ChatGPT / Codex to reload the model list.' });
```

新增辅助函数（文件顶部，导出以便测试）：

```tsx
export function summarizeEnrichment(state: ModelManagementState) {
  if (state.enrichmentError) return `Models saved, but model parameters were not updated: ${state.enrichmentError}`;
  const unmatched = state.unmatchedModels || [];
  if (!unmatched.length) return 'Models saved and parameters updated from the catalog. Restart ChatGPT / Codex to reload the model list.';
  return `Models saved. Kept local parameters for ${unmatched.length} unmatched model${unmatched.length === 1 ? '' : 's'}: ${unmatched.join(', ')}`;
}
```

在工具栏按钮行加入复选框：

```tsx
        <label className="model-toggle" title="Refresh reasoning levels, input modalities and context window from the model catalog on save"><input type="checkbox" checked={updateParams} disabled={busy} onChange={(event) => setUpdateParams(event.target.checked)} /><span>Update params</span></label>
```

列表行对未匹配模型加标记（在 `stale` 提示同一位置）：

```tsx
        const unmatched = !!state?.unmatchedModels?.includes(typeof model?.slug === 'string' ? model.slug : '');
```

并在 `profile-main` 内渲染：

```tsx
            {unmatched && <small className="sync-status sync-needs_sync">Not found in model catalog · kept local parameters</small>}
```

`style.css` 追加：

```css
.model-toggle { display:flex; align-items:center; gap:6px; font-size:13px; color:#172033; }
.button-row .model-toggle { margin-right:4px; }
```

- [ ] **Step 2: 构建与浏览器验证**

Run: `npm run build --prefix frontend`
Expected: 构建成功

用 Playwright 注入桩后端验证：
- 默认 `Update params` 勾选；取消勾选时保存请求体 `updateParams` 为 `false`；
- `enrichmentError` 非空时提示「parameters were not updated」且保存仍成功；
- `unmatchedModels` 非空时提示中列出模型名，且对应行显示未匹配标记；
- `enrichmentError` 与 `unmatchedModels` 均空时提示「parameters updated」。

- [ ] **Step 3: Commit**

```bash
git add frontend/src/ModelsTab.tsx frontend/src/style.css
git commit -m "feat: add Update params control and catalog result notices"
```

---

### Task 7: models.json 手动编辑

**Files:**
- Modify: `frontend/src/ModelsTab.tsx`
- Modify: `frontend/src/style.css`

**Interfaces:**
- Consumes: Task 6 的 `save()` 与 `state`。
- Produces: `parseModelDraft(text: string): { models: ModelEntry[] } | { error: string }`（导出以便测试）。

- [ ] **Step 1: 实现并构建**

新增导出函数：

```tsx
export function parseModelDraft(text: string): { models: ModelEntry[] } | { error: string } {
  let parsed: unknown;
  try { parsed = JSON.parse(text); } catch (error) { return { error: `Invalid JSON: ${String(error)}` }; }
  const list = Array.isArray(parsed) ? parsed : (parsed && typeof parsed === 'object' && Array.isArray((parsed as { models?: unknown }).models) ? (parsed as { models: unknown[] }).models : null);
  if (!list) return { error: 'Expected a JSON array of models, or an object with a "models" array.' };
  for (const item of list) {
    if (!item || typeof item !== 'object' || Array.isArray(item)) return { error: 'Every model entry must be a JSON object.' };
  }
  return { models: list as ModelEntry[] };
}
```

新增状态与切换：

```tsx
  const [jsonOpen, setJsonOpen] = useState(false);
  const [jsonText, setJsonText] = useState('');
```

打开编辑器时填充当前草稿：

```tsx
  function toggleJson() {
    if (!jsonOpen) { setJsonText(JSON.stringify(draft.map((entry) => JSON.parse(entry.raw)), null, 2)); setJsonOpen(true); return; }
    setJsonOpen(false); setJsonText('');
  }
```

`dirty` 计算纳入 JSON 文本：

```tsx
  const jsonDirty = jsonOpen && jsonText !== JSON.stringify(draft.map((entry) => JSON.parse(entry.raw)), null, 2);
  const dirty = modelsDirty || defaultsDirty || jsonDirty;
```

`save()` 改为先解析 JSON 文本、再统一构建请求体：

```tsx
    if (jsonOpen) {
      const parsed = parseModelDraft(jsonText);
      if ('error' in parsed) { setMessage({ error: true, text: parsed.error }); setBusy(false); return; }
    }
    const entries = (jsonOpen ? (parseModelDraft(jsonText) as { models: ModelEntry[] }).models : draft.map((entry) => JSON.parse(entry.raw) as ModelEntry));
    try {
      const next = await api.SaveModels({ fileHash: state.fileHash, rawModels: entries.map((model) => JSON.stringify(model)), visibilityOverrides: jsonOpen ? {} : Object.fromEntries(draft.flatMap((entry, index) => entry.visibility ? [[index, entry.visibility]] : [])), updateParams });
```

工具栏加入切换按钮：

```tsx
        <button className="secondary" disabled={busy || !editable} onClick={toggleJson}>{jsonOpen ? 'Hide JSON' : 'Edit JSON'}</button>
```

列表区域上方渲染编辑器：

```tsx
      {jsonOpen && <label className="field"><span>models.json (models array)</span><textarea className="code-editor" spellCheck={false} rows={16} value={jsonText} onChange={(event) => setJsonText(event.target.value)} /></label>}
```

`style.css` 追加：

```css
.model-row .sync-status { display:block; }
```

- [ ] **Step 2: 构建与浏览器验证**

Run: `npm run build --prefix frontend`
Expected: 构建成功

用 Playwright 验证：
- 打开 `Edit JSON` 后文本域显示当前草稿的 JSON 数组；
- 输入非法 JSON 后保存：出现 `Invalid JSON` 提示，未调用 `SaveModels`；
- 输入合法 JSON（含自定义 `context_window`）后保存：`SaveModels` 收到对应 `rawModels`；
- JSON 改动会让「Unsaved changes」状态与未保存确认弹窗生效。

- [ ] **Step 3: Commit**

```bash
git add frontend/src/ModelsTab.tsx frontend/src/style.css
git commit -m "feat: allow editing models.json directly"
```

---

### Task 8: 全量验证、构建与安装

**Files:**
- Modify: 无（仅验证与产物）

- [ ] **Step 1: 全量检查**

Run: `perl -pi -e 's/[ \t]+$//' frontend/wailsjs/go/models.ts && gofmt -l . && go vet ./... && go test -race ./... && npm run build --prefix frontend && git diff --check`
Expected: 无 gofmt 输出；vet 与测试全部 PASS；构建成功；diff 无空白错误。

- [ ] **Step 2: 构建并安装**

Run:

```bash
/Users/yp-mac-02/go/bin/wails build -clean
pkill -f '^/Applications/AgentPal.app/Contents/MacOS/AgentPal$' || true
ditto build/bin/AgentPal.app /Applications/AgentPal.app
codesign --verify --deep --strict /Applications/AgentPal.app
shasum -a 256 build/bin/AgentPal.app/Contents/MacOS/AgentPal /Applications/AgentPal.app/Contents/MacOS/AgentPal
```

Expected: 签名校验通过，两处哈希一致。

- [ ] **Step 3: 真实环境验证**

用真实 `~/.codex` 打开 AgentPal → Models → 勾选 `Update params` → 保存，确认：
- `~/.codex/models.json` 中 `context_window`/`max_context_window` 对 openai、anthropic 模型为 `272000`；
- `input_modalities` 只含 `text`/`image`/`audio`，无 `file`/`video`；
- `supported_reasoning_levels` 为对象数组且含 `description`；
- 未匹配模型保留原值且界面已列出；
- 无任何输出相关字段被写入。

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "chore: verify model parameter enrichment end to end"
```
