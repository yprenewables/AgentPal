# AgentPal 中心登录与预留 Auth Profile 同步 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 客户端通过 Casdoor 登录后，从配置的 Central Host 拉取该用户预留的 `model_provider` auth profile，并按“新增 / 已同步 / 需要同步”三态同步到本地，冲突时不自动覆盖。

**Architecture:** Central Host 承担 OAuth 回调与授权码换 token（Authorization Code + PKCE），按 JWT `sub`（只解析不验签）隔离用户预留 profile 的只读目录。客户端发起登录、轮询取 token、请求中心 profile 列表，本地匹配 `reservedName` 后决定新增、标记已同步或提示冲突。覆盖必须由用户显式触发，且永不修改 `~/.codex`。

**Tech Stack:** Go 1.22+（`net/http`、`encoding/json`、`github.com/pelletier/go-toml/v2`、`github.com/google/uuid`）、Wails v2、React + TypeScript + Vite。

## Global Constraints

- 第一版只支持 Codex；不引入 OpenCode。
- 不校验 JWT 签名，只 Base64URL 解析 payload 的 `sub`；仅适用于可信内网。
- 使用 PKCE（`code_challenge_method=S256`），token 交换不携带 `client_secret`。
- 预留 profile 目录由管理员手动维护，服务端只读，不提供写入接口。
- 同步永不自动激活/切换，永不修改 `~/.codex` 下任何文件。
- `needs_sync` 绝不自动覆盖本地。
- 用户名（`sub`）、预留名（`name`）必须做路径穿越校验。
- 新增文件权限：目录 `0700`，文件 `0600`。
- 内容哈希算法固定为「规范化 auth.json + `\n` + 规范化 config-auth.toml」的 SHA-256，输出 `sha256:<hex>`。
- 所有新增 Go 代码必须 `gofmt` 且 `go vet ./...` 通过。
- 测试命令统一为 `go test ./...`；前端构建为 `cd frontend && npm run build`。
- 提交信息使用仓库既有英文祈使句风格（仅在用户明确要求提交时执行）。

---

### Task 1: 共享内容哈希与预留名校验

**Files:**
- Create: `internal/authprofile/content.go`
- Test: `internal/authprofile/content_test.go`

**Interfaces:**
- Consumes: 无
- Produces:
  - `func ComputeContentHash(authJSON, configTOML []byte) (string, error)`
  - `func ValidateReservedName(name string) error`

- [ ] **Step 1: 写失败测试**

创建 `internal/authprofile/content_test.go`：

```go
package authprofile

import "testing"

func TestComputeContentHashIgnoresFormatting(t *testing.T) {
	authA := []byte(`{"OPENAI_API_KEY":"sk-1","nested":{"b":"2","a":"1"}}`)
	authB := []byte("{\n  \"nested\": {\"a\": \"1\", \"b\": \"2\"},\n  \"OPENAI_API_KEY\": \"sk-1\"\n}\n")
	configA := []byte("model_provider = \"yangpu\"\n\n[model_providers.yangpu]\nbase_url = \"u\"\nname = \"yangpu\"\n")
	configB := []byte("# reordered and reformatted\nmodel_provider = \"yangpu\"\n\n[model_providers.yangpu]\nname  =  \"yangpu\"\nbase_url = \"u\"\n")
	hashA, err := ComputeContentHash(authA, configA)
	if err != nil {
		t.Fatal(err)
	}
	hashB, err := ComputeContentHash(authB, configB)
	if err != nil {
		t.Fatal(err)
	}
	if hashA != hashB {
		t.Fatalf("hashA=%q hashB=%q", hashA, hashB)
	}
	if len(hashA) < len("sha256:") || hashA[:len("sha256:")] != "sha256:" {
		t.Fatalf("unexpected prefix: %q", hashA)
	}
}

func TestComputeContentHashRejectsInvalidInput(t *testing.T) {
	if _, err := ComputeContentHash([]byte("not-json"), []byte("model_provider = \"x\"\n")); err == nil {
		t.Fatal("invalid auth json accepted")
	}
	if _, err := ComputeContentHash([]byte(`{}`), []byte("===")); err == nil {
		t.Fatal("invalid toml accepted")
	}
}

func TestValidateReservedName(t *testing.T) {
	for _, name := range []string{"yangpu", "Team-1", "a.b_c"} {
		if err := ValidateReservedName(name); err != nil {
			t.Fatalf("%q rejected: %v", name, err)
		}
	}
	for _, name := range []string{"", ".", "..", "a/b", `a\b`, "a b", "a:b"} {
		if err := ValidateReservedName(name); err == nil {
			t.Fatalf("%q accepted", name)
		}
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/authprofile/ -run 'TestComputeContentHash|TestValidateReservedName' -v`
Expected: FAIL，报 `undefined: ComputeContentHash` / `undefined: ValidateReservedName`。

- [ ] **Step 3: 写实现**

创建 `internal/authprofile/content.go`：

```go
package authprofile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"

	"github.com/pelletier/go-toml/v2"
)

var reservedNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// ValidateReservedName 校验中心预留的 model_provider 名称是否可安全用作目录名。
func ValidateReservedName(name string) error {
	if name == "" || name == "." || name == ".." {
		return errors.New("reserved name is invalid")
	}
	if !reservedNamePattern.MatchString(name) {
		return errors.New("reserved name contains an invalid character")
	}
	return nil
}

// ComputeContentHash 用规范化后的 auth.json 与 config-auth.toml 计算内容哈希。
// 服务端与客户端必须使用同一算法，避免仅因键顺序或空白差异误判为冲突。
func ComputeContentHash(authJSON, configTOML []byte) (string, error) {
	var auth map[string]any
	if err := json.Unmarshal(authJSON, &auth); err != nil {
		return "", err
	}
	canonicalAuth, err := json.Marshal(auth)
	if err != nil {
		return "", err
	}
	var config map[string]any
	if err := toml.Unmarshal(configTOML, &config); err != nil {
		return "", err
	}
	canonicalConfig, err := toml.Marshal(config)
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	digest.Write(canonicalAuth)
	digest.Write([]byte("\n"))
	digest.Write(canonicalConfig)
	return "sha256:" + hex.EncodeToString(digest.Sum(nil)), nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/authprofile/ -run 'TestComputeContentHash|TestValidateReservedName' -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/authprofile/content.go internal/authprofile/content_test.go
git commit -m "Add shared auth profile content hash and reserved name validation"
```

---

### Task 2: AuthProfile 增加 reservedName 与 contentHash

**Files:**
- Modify: `internal/authprofile/store.go`（`AuthProfile` 结构体、`readSavedProfile`、`writeProfile`）
- Modify: `internal/types/types.go:131-143`（`AuthProfile`）
- Modify: `app.go:157-159`（`profileType`）
- Modify: `frontend/src/wails.ts:6`
- Modify: `frontend/wailsjs/go/models.ts`（`AuthProfile` 类）
- Test: `internal/authprofile/store_test.go`

**Interfaces:**
- Consumes: `ComputeContentHash`（Task 1）
- Produces:
  - `authprofile.AuthProfile.ReservedName string`
  - `authprofile.AuthProfile.ContentHash string`
  - `types.AuthProfile.ReservedName string` / `.ContentHash string`（JSON: `reservedName` / `contentHash`）
  - `writeProfile` 写入时自动填充 `ContentHash`

- [ ] **Step 1: 写失败测试**

在 `internal/authprofile/store_test.go` 末尾追加：

```go
func TestSavedProfileCarriesContentHashAndReservedName(t *testing.T) {
	store := NewStore(t.TempDir())
	profile, err := store.CreateAPIKey(APIKeyRequest{Name: "yangpu", Provider: "yangpu", BaseURL: "https://example/v1", APIKey: "sk-test"})
	if err != nil {
		t.Fatal(err)
	}
	if profile.ContentHash == "" {
		t.Fatal("created profile has no content hash")
	}
	if profile.ReservedName != "" {
		t.Fatalf("locally created profile must not have reservedName: %#v", profile)
	}
	profiles, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].ContentHash == "" {
		t.Fatalf("listed profile missing content hash: %#v", profiles)
	}
	if profiles[0].ContentHash != profile.ContentHash {
		t.Fatalf("hash mismatch: %q vs %q", profiles[0].ContentHash, profile.ContentHash)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/authprofile/ -run TestSavedProfileCarriesContentHashAndReservedName -v`
Expected: FAIL，报 `profile.ContentHash undefined`（编译错误）。

- [ ] **Step 3: 扩展结构体与持久化**

在 `internal/authprofile/store.go` 的 `AuthProfile` 结构体中，`Source` 之后加入：

```go
	ReservedName     string    `json:"reservedName,omitempty"`
	ContentHash      string    `json:"contentHash,omitempty"`
```

在 `readSavedProfile` 中，`profile.ChatGPTExpiresAt = findExpiresAt(auth)` 之后加入：

```go
	if hash, err := ComputeContentHash(authData, configData); err == nil {
		profile.ContentHash = hash
	}
```

在 `writeProfile` 中，`meta, _ := json.MarshalIndent(p, "", "  ")` 之前加入：

```go
	if hash, err := ComputeContentHash(auth, config); err == nil {
		p.ContentHash = hash
	}
```

由于 Go 值传递，`writeProfile` 不会更新调用方的 `p`。把 `CreateAPIKey` 的成功返回改为重新读取，使返回值带上 `ContentHash`：

```go
	if err := writeProfile(dir, p, auth, config); err != nil {
		return p, err
	}
	return readSavedProfile(dir)
```

（`ImportCurrent` 的返回路径保持不变；同步流程在本地 `ContentHash` 为空时用 `ContentHashFor` 现算兜底。）

- [ ] **Step 4: 扩展 types 与前端类型**

在 `internal/types/types.go` 的 `AuthProfile` 结构体（`ChatGPTExpiresAt` 之后）加入：

```go
	ReservedName     string `json:"reservedName,omitempty"`
	ContentHash      string `json:"contentHash,omitempty"`
```

在 `app.go` 的 `profileType` 返回值中加入两个字段：

```go
func profileType(p authprofile.AuthProfile) types.AuthProfile {
	return types.AuthProfile{ID: p.ID, Name: p.Name, Type: p.Type, Provider: p.Provider, Source: p.Source, Active: p.Active, APIKeyPreview: p.APIKeyPreview, BaseURL: p.BaseURL, ChatGPTUserID: p.ChatGPTUserID, ChatGPTExpiresAt: p.ChatGPTExpiresAt, ReservedName: p.ReservedName, ContentHash: p.ContentHash}
}
```

在 `frontend/src/wails.ts` 第 6 行 `AuthProfile` 类型末尾加入：

```ts
reservedName?: string; contentHash?: string;
```

在 `frontend/wailsjs/go/models.ts` 的 `AuthProfile` 类中，`chatgptExpiresAt` 之后加入字段声明与赋值：

```ts
	    reservedName?: string;
	    contentHash?: string;
```

并在该 `from` 中追加：

```ts
	        this.reservedName = source["reservedName"];
	        this.contentHash = source["contentHash"];
```

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/authprofile/ ./internal/types/ -v`
Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add internal/authprofile/store.go internal/authprofile/store_test.go internal/types/types.go app.go frontend/src/wails.ts frontend/wailsjs/go/models.ts
git commit -m "Add reservedName and contentHash to auth profiles"
```

---

### Task 3: Store 的中心 profile 操作

**Files:**
- Create: `internal/authprofile/central.go`
- Test: `internal/authprofile/central_test.go`

**Interfaces:**
- Consumes: `AuthProfile{ReservedName,ContentHash}`（Task 2）、`ValidateReservedName` / `ComputeContentHash`（Task 1）、`writeProfile` / `readSavedProfile`（既有）
- Produces:
  - `type CentralProfile struct { Name, Provider, Type string; AuthJSON, ConfigTOML []byte }`
  - `func (s *Store) FindByReservedName(name string) (AuthProfile, bool, error)`
  - `func (s *Store) AddCentral(c CentralProfile) (AuthProfile, error)`
  - `func (s *Store) OverwriteCentral(profileID string, c CentralProfile) (AuthProfile, error)`
  - `func (s *Store) ContentHashFor(profileID string) (string, error)`

- [ ] **Step 1: 写失败测试**

创建 `internal/authprofile/central_test.go`：

```go
package authprofile

import (
	"os"
	"path/filepath"
	"testing"
)

func centralFixture() CentralProfile {
	return CentralProfile{
		Name:     "yangpu",
		Provider: "yangpu",
		Type:     "api_key",
		AuthJSON: []byte(`{"OPENAI_API_KEY":"sk-central"}`),
		ConfigTOML: []byte("model_provider = \"yangpu\"\n\n[model_providers.yangpu]\nname = \"yangpu\"\nbase_url = \"https://central.example/v1\"\n"),
	}
}

func TestAddCentralCreatesReservedProfile(t *testing.T) {
	store := NewStore(t.TempDir())
	profile, err := store.AddCentral(centralFixture())
	if err != nil {
		t.Fatal(err)
	}
	if profile.ReservedName != "yangpu" || profile.Source != "central_login" || profile.ContentHash == "" {
		t.Fatalf("unexpected profile: %#v", profile)
	}
	found, ok, err := store.FindByReservedName("yangpu")
	if err != nil || !ok || found.ID != profile.ID {
		t.Fatalf("find: ok=%v err=%v found=%#v", ok, err, found)
	}
	if _, ok, _ := store.FindByReservedName("other"); ok {
		t.Fatal("unexpected match for other")
	}
}

func TestAddCentralRejectsProviderMismatch(t *testing.T) {
	store := NewStore(t.TempDir())
	bad := centralFixture()
	bad.ConfigTOML = []byte("model_provider = \"other\"\n\n[model_providers.other]\nbase_url = \"u\"\n")
	if _, err := store.AddCentral(bad); err == nil {
		t.Fatal("provider mismatch accepted")
	}
}

func TestOverwriteCentralReplacesFilesAndKeepsID(t *testing.T) {
	store := NewStore(t.TempDir())
	profile, err := store.AddCentral(centralFixture())
	if err != nil {
		t.Fatal(err)
	}
	next := centralFixture()
	next.AuthJSON = []byte(`{"OPENAI_API_KEY":"sk-updated"}`)
	next.ConfigTOML = []byte("model_provider = \"yangpu\"\n\n[model_providers.yangpu]\nname = \"yangpu\"\nbase_url = \"https://updated.example/v1\"\n")
	updated, err := store.OverwriteCentral(profile.ID, next)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != profile.ID {
		t.Fatalf("id changed: %q -> %q", profile.ID, updated.ID)
	}
	data, err := os.ReadFile(filepath.Join(store.Root, profile.ID, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"OPENAI_API_KEY":"sk-updated"}` {
		t.Fatalf("auth.json not overwritten: %s", data)
	}
	if updated.ContentHash == profile.ContentHash {
		t.Fatal("content hash did not change")
	}
}

func TestContentHashForReadsStoredFiles(t *testing.T) {
	store := NewStore(t.TempDir())
	profile, err := store.AddCentral(centralFixture())
	if err != nil {
		t.Fatal(err)
	}
	hash, err := store.ContentHashFor(profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if hash != profile.ContentHash {
		t.Fatalf("hash=%q profile=%q", hash, profile.ContentHash)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/authprofile/ -run 'TestAddCentral|TestOverwriteCentral|TestContentHashFor' -v`
Expected: FAIL，报 `undefined: CentralProfile`。

- [ ] **Step 3: 写实现**

创建 `internal/authprofile/central.go`：

```go
package authprofile

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/pelletier/go-toml/v2"
)

// CentralProfile 是中心预留 profile 的传输形态。
type CentralProfile struct {
	Name       string
	Provider   string
	Type       string
	AuthJSON   []byte
	ConfigTOML []byte
}

func validateCentralProfile(c CentralProfile) error {
	if err := ValidateReservedName(c.Name); err != nil {
		return err
	}
	if c.Type == "" || c.Provider == "" {
		return errors.New("central profile metadata is incomplete")
	}
	var config map[string]any
	if err := toml.Unmarshal(c.ConfigTOML, &config); err != nil {
		return err
	}
	if provider, _ := config["model_provider"].(string); provider != c.Name {
		return errors.New("central profile provider does not match its name")
	}
	return nil
}

func (s *Store) FindByReservedName(name string) (AuthProfile, bool, error) {
	profiles, err := s.List()
	if err != nil {
		return AuthProfile{}, false, err
	}
	for _, profile := range profiles {
		if profile.ReservedName == name {
			return profile, true, nil
		}
	}
	return AuthProfile{}, false, nil
}

func (s *Store) AddCentral(c CentralProfile) (AuthProfile, error) {
	if err := validateCentralProfile(c); err != nil {
		return AuthProfile{}, err
	}
	now := time.Now().UTC()
	p := AuthProfile{ID: uuid.NewString(), Name: c.Name, Type: c.Type, Provider: c.Provider, Source: "central_login", ReservedName: c.Name, CreatedAt: now, UpdatedAt: now}
	dir := filepath.Join(s.Root, p.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return p, err
	}
	if err := writeProfile(dir, p, c.AuthJSON, c.ConfigTOML); err != nil {
		return p, err
	}
	return readSavedProfile(dir)
}

func (s *Store) OverwriteCentral(profileID string, c CentralProfile) (AuthProfile, error) {
	if profileID == "" || filepath.Base(profileID) != profileID || profileID == "." || profileID == ".." {
		return AuthProfile{}, errors.New("invalid profile ID")
	}
	if err := validateCentralProfile(c); err != nil {
		return AuthProfile{}, err
	}
	dir := filepath.Join(s.Root, profileID)
	existing, err := readSavedProfile(dir)
	if err != nil {
		return AuthProfile{}, err
	}
	existing.Name = c.Name
	existing.Type = c.Type
	existing.Provider = c.Provider
	existing.ReservedName = c.Name
	existing.Source = "central_login"
	existing.UpdatedAt = time.Now().UTC()
	if err := writeProfile(dir, existing, c.AuthJSON, c.ConfigTOML); err != nil {
		return existing, err
	}
	return readSavedProfile(dir)
}

func (s *Store) ContentHashFor(profileID string) (string, error) {
	if profileID == "" || filepath.Base(profileID) != profileID {
		return "", errors.New("invalid profile ID")
	}
	dir := filepath.Join(s.Root, profileID)
	authData, err := os.ReadFile(filepath.Join(dir, "auth.json"))
	if err != nil {
		return "", err
	}
	configData, err := os.ReadFile(filepath.Join(dir, "config-auth.toml"))
	if err != nil {
		return "", err
	}
	return ComputeContentHash(authData, configData)
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/authprofile/ -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/authprofile/central.go internal/authprofile/central_test.go
git commit -m "Add store operations for central reserved profiles"
```

---

### Task 4: JWT sub 解析与校验

**Files:**
- Create: `internal/central/jwt.go`
- Test: `internal/central/jwt_test.go`

**Interfaces:**
- Consumes: `b64JSON`（既有测试辅助，见 `internal/central/auth_test.go:83`）
- Produces: `func ParseJWTSubject(token string) (string, error)`

- [ ] **Step 1: 写失败测试**

创建 `internal/central/jwt_test.go`：

```go
package central

import (
	"strings"
	"testing"
)

func unsignedJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	return b64JSON(t, map[string]string{"alg": "RS256", "typ": "JWT"}) + "." + b64JSON(t, claims) + ".signature"
}

func TestParseJWTSubjectReadsSub(t *testing.T) {
	token := unsignedJWT(t, map[string]any{"sub": "user-1", "iss": "https://id.yprenew.com"})
	subject, err := ParseJWTSubject(token)
	if err != nil || subject != "user-1" {
		t.Fatalf("subject=%q err=%v", subject, err)
	}
}

func TestParseJWTSubjectRejectsInvalidToken(t *testing.T) {
	for _, token := range []string{"", "a.b", "a.b.c.d", "a.!!!.x", "a." + b64JSON(t, "not-an-object") + ".x"} {
		if _, err := ParseJWTSubject(token); err == nil {
			t.Fatalf("token accepted: %q", token)
		}
	}
}

func TestParseJWTSubjectRejectsUnsafeSub(t *testing.T) {
	for _, sub := range []string{"", ".", "..", "a/b", `a\b`, "a..b", "a:b", "a\x00b", strings.Repeat("x", 257)} {
		token := unsignedJWT(t, map[string]any{"sub": sub})
		if _, err := ParseJWTSubject(token); err == nil {
			t.Fatalf("sub accepted: %q", sub)
		}
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/central/ -run TestParseJWTSubject -v`
Expected: FAIL，报 `undefined: ParseJWTSubject`。

- [ ] **Step 3: 写实现**

创建 `internal/central/jwt.go`：

```go
package central

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

// ParseJWTSubject 只解析 JWT payload 的 sub，不校验签名。
// 设计前提是 Central Host 只运行在可信内网。
func ParseJWTSubject(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", errors.New("invalid jwt")
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", errors.New("invalid jwt payload")
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", errors.New("invalid jwt payload")
	}
	subject, _ := payload["sub"].(string)
	if err := validateSubject(subject); err != nil {
		return "", err
	}
	return subject, nil
}

func validateSubject(subject string) error {
	if subject == "" {
		return errors.New("jwt sub is required")
	}
	if len(subject) > 256 {
		return errors.New("jwt sub is too long")
	}
	if subject != strings.TrimSpace(subject) {
		return errors.New("jwt sub has surrounding whitespace")
	}
	if subject == "." || subject == ".." || strings.Contains(subject, "..") {
		return errors.New("jwt sub is invalid")
	}
	for _, r := range subject {
		if r == '/' || r == '\\' || r == ':' || r < 0x20 || r == 0x7f {
			return errors.New("jwt sub contains an invalid character")
		}
	}
	return nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/central/ -run TestParseJWTSubject -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/central/jwt.go internal/central/jwt_test.go
git commit -m "Add JWT sub parsing for central user isolation"
```

---

### Task 5: 服务端用户预留 profile 只读存储

**Files:**
- Create: `internal/central/userprofiles.go`
- Modify: `internal/types/types.go`（新增 `RemoteAuthProfile`、`RemoteAuthProfileDetail`）
- Test: `internal/central/userprofiles_test.go`

**Interfaces:**
- Consumes: `authprofile.ValidateReservedName`、`authprofile.ComputeContentHash`（Task 1）、`security.SafeJoin`
- Produces:
  - `types.RemoteAuthProfile{Name, Provider, Type, ContentHash string; UpdatedAt time.Time}`
  - `types.RemoteAuthProfileDetail{Name, Provider, Type, ContentHash string; UpdatedAt time.Time; Auth map[string]any; ConfigAuthToml string}`
  - `type userProfileStore struct{ root string }`
  - `func newUserProfileStore(root string) *userProfileStore`
  - `func (s *userProfileStore) list(sub string) ([]types.RemoteAuthProfile, error)`
  - `func (s *userProfileStore) read(sub, name string) (reservedProfile, error)`

- [ ] **Step 1: 写失败测试**

创建 `internal/central/userprofiles_test.go`：

```go
package central

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeReservedProfile(t *testing.T, root, sub, name, providerInToml string) {
	t.Helper()
	dir := filepath.Join(root, sub, "auth-profiles", name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"profile.json":     `{"name":"` + name + `","type":"api_key","provider":"` + name + `"}`,
		"auth.json":        `{"OPENAI_API_KEY":"sk-` + name + `"}`,
		"config-auth.toml": "model_provider = \"" + providerInToml + "\"\n\n[model_providers." + providerInToml + "]\nname = \"" + providerInToml + "\"\nbase_url = \"https://example/v1\"\n",
	}
	for file, content := range files {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUserProfileStoreListsMetadataOnly(t *testing.T) {
	root := t.TempDir()
	writeReservedProfile(t, root, "user-1", "yangpu", "yangpu")
	store := newUserProfileStore(root)
	items, err := store.list("user-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "yangpu" || items[0].Provider != "yangpu" || items[0].Type != "api_key" {
		t.Fatalf("items=%#v", items)
	}
	if items[0].ContentHash == "" {
		t.Fatal("missing content hash")
	}
	if _, err := os.Stat(filepath.Join(root, "user-2")); !os.IsNotExist(err) {
		t.Fatal("unexpected cross-user directory")
	}
	if other, err := store.list("user-2"); err != nil || len(other) != 0 {
		t.Fatalf("missing user should list empty: %#v err=%v", other, err)
	}
}

func TestUserProfileStoreSkipsInvalidEntries(t *testing.T) {
	root := t.TempDir()
	writeReservedProfile(t, root, "user-1", "good", "good")
	writeReservedProfile(t, root, "user-1", "mismatch", "other")
	incomplete := filepath.Join(root, "user-1", "auth-profiles", "incomplete")
	if err := os.MkdirAll(incomplete, 0o700); err != nil {
		t.Fatal(err)
	}
	store := newUserProfileStore(root)
	items, err := store.list("user-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "good" {
		t.Fatalf("items=%#v", items)
	}
}

func TestUserProfileStoreReadDetail(t *testing.T) {
	root := t.TempDir()
	writeReservedProfile(t, root, "user-1", "yangpu", "yangpu")
	store := newUserProfileStore(root)
	profile, err := store.read("user-1", "yangpu")
	if err != nil {
		t.Fatal(err)
	}
	if string(profile.AuthJSON) == "" || string(profile.ConfigTOML) == "" || profile.ContentHash == "" {
		t.Fatalf("profile=%#v", profile)
	}
	if profile.UpdatedAt.IsZero() || profile.UpdatedAt.After(time.Now().Add(time.Minute)) {
		t.Fatalf("unexpected updatedAt: %v", profile.UpdatedAt)
	}
}

func TestUserProfileStoreRejectsPathTraversal(t *testing.T) {
	store := newUserProfileStore(t.TempDir())
	if _, err := store.read("user-1", "../secret"); err == nil {
		t.Fatal("path traversal accepted")
	}
	if _, err := store.list("../secret"); err == nil {
		t.Fatal("subject traversal accepted")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/central/ -run TestUserProfileStore -v`
Expected: FAIL，报 `undefined: newUserProfileStore`。

- [ ] **Step 3: 加类型**

在 `internal/types/types.go` 末尾加入：

```go
type RemoteAuthProfile struct {
	Name        string    `json:"name"`
	Provider    string    `json:"provider"`
	Type        string    `json:"type"`
	ContentHash string    `json:"contentHash"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type RemoteAuthProfileDetail struct {
	Name           string         `json:"name"`
	Provider       string         `json:"provider"`
	Type           string         `json:"type"`
	ContentHash    string         `json:"contentHash"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	Auth           map[string]any `json:"auth"`
	ConfigAuthToml string         `json:"configAuthToml"`
}
```

- [ ] **Step 4: 写实现**

创建 `internal/central/userprofiles.go`：

```go
package central

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"agentpal/internal/authprofile"
	"agentpal/internal/security"
	"agentpal/internal/types"

	"github.com/pelletier/go-toml/v2"
)

type userProfileStore struct{ root string }

func newUserProfileStore(root string) *userProfileStore { return &userProfileStore{root: root} }

type reservedMeta struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Provider string `json:"provider"`
}

type reservedProfile struct {
	Meta        reservedMeta
	AuthJSON    []byte
	ConfigTOML  []byte
	ContentHash string
	UpdatedAt   time.Time
}

func (p reservedProfile) remote() types.RemoteAuthProfile {
	return types.RemoteAuthProfile{Name: p.Meta.Name, Provider: p.Meta.Provider, Type: p.Meta.Type, ContentHash: p.ContentHash, UpdatedAt: p.UpdatedAt}
}

func (s *userProfileStore) list(sub string) ([]types.RemoteAuthProfile, error) {
	dir, err := security.SafeJoin(s.root, sub, "auth-profiles")
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []types.RemoteAuthProfile{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := []types.RemoteAuthProfile{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		profile, err := s.read(sub, entry.Name())
		if err != nil {
			continue
		}
		result = append(result, profile.remote())
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (s *userProfileStore) read(sub, name string) (reservedProfile, error) {
	if err := authprofile.ValidateReservedName(name); err != nil {
		return reservedProfile{}, err
	}
	dir, err := security.SafeJoin(s.root, sub, "auth-profiles", name)
	if err != nil {
		return reservedProfile{}, err
	}
	metaData, err := os.ReadFile(filepath.Join(dir, "profile.json"))
	if err != nil {
		return reservedProfile{}, err
	}
	var meta reservedMeta
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return reservedProfile{}, err
	}
	if meta.Name != name || meta.Type == "" || meta.Provider == "" {
		return reservedProfile{}, errors.New("reserved profile metadata is invalid")
	}
	authData, err := os.ReadFile(filepath.Join(dir, "auth.json"))
	if err != nil {
		return reservedProfile{}, err
	}
	configData, err := os.ReadFile(filepath.Join(dir, "config-auth.toml"))
	if err != nil {
		return reservedProfile{}, err
	}
	var config map[string]any
	if err := toml.Unmarshal(configData, &config); err != nil {
		return reservedProfile{}, err
	}
	if provider, _ := config["model_provider"].(string); provider != name {
		return reservedProfile{}, errors.New("reserved profile provider does not match its name")
	}
	hash, err := authprofile.ComputeContentHash(authData, configData)
	if err != nil {
		return reservedProfile{}, err
	}
	updated := time.Time{}
	if info, err := os.Stat(filepath.Join(dir, "profile.json")); err == nil {
		updated = info.ModTime().UTC()
	}
	return reservedProfile{Meta: meta, AuthJSON: authData, ConfigTOML: configData, ContentHash: hash, UpdatedAt: updated}, nil
}
```

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/central/ -run TestUserProfileStore -v`
Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add internal/central/userprofiles.go internal/central/userprofiles_test.go internal/types/types.go
git commit -m "Add read-only central user profile store"
```

---

### Task 6: 服务端登录状态机（PKCE + start/status/callback）

**Files:**
- Create: `internal/central/login.go`
- Modify: `internal/central/server.go`（Server 字段、NewServer 初始化、路由）
- Modify: `internal/types/types.go`（`ServerOptions` 增加字段；新增 `LoginStart`、`LoginStatus`）
- Test: `internal/central/login_test.go`

**Interfaces:**
- Consumes: `randomToken`（`device.go:127`）、`ParseJWTSubject`（Task 4）、`writeJSON`
- Produces:
  - `types.LoginStart{LoginID, PollToken, AuthorizeURL string; ExpiresAt time.Time}`
  - `types.LoginStatus{Status, Subject, AccessToken string; ExpiresIn int; Error string}`
  - `func (s *Server) oauthStart(w, r)`、`oauthStatus`、`oauthCallback`
  - `func codeChallenge(verifier string) string`
  - `type pendingLogins`，方法 `start` / `approve` / `fail` / `take` / `peekByState`

- [ ] **Step 1: 写失败测试**

创建 `internal/central/login_test.go`：

```go
package central

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"agentpal/internal/types"
)

type capturedExchange struct {
	Code     string
	Verifier string
}

func startLogin(t *testing.T, server *Server) types.LoginStart {
	t.Helper()
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/auth/login/start", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("start status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var start types.LoginStart
	if err := json.Unmarshal(recorder.Body.Bytes(), &start); err != nil {
		t.Fatal(err)
	}
	return start
}

func TestLoginFlowApprovesOnceWithPKCE(t *testing.T) {
	captured := &capturedExchange{}
	accessToken := unsignedJWT(t, map[string]any{"sub": "user-1"})
	// challenge 在 start 之后才能确定，用可变持有。
	var challenge string
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]string
		_ = json.Unmarshal(body, &payload)
		captured.Code = payload["code"]
		captured.Verifier = payload["code_verifier"]
		digest := sha256.Sum256([]byte(payload["code_verifier"]))
		if base64.RawURLEncoding.EncodeToString(digest[:]) != challenge {
			http.Error(w, "challenge mismatch", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": accessToken, "expires_in": 3600})
	}))
	defer stub.Close()

	server := NewServer(types.ServerOptions{CodexDir: t.TempDir(), Port: 28888, CasdoorURL: stub.URL, CasdoorClientID: "cid", OAuthRedirectURL: "http://central.test/callback"})
	start := startLogin(t, server)
	parsed, err := url.Parse(start.AuthorizeURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("authorize url=%s", start.AuthorizeURL)
	}
	challenge = parsed.Query().Get("code_challenge")
	state := parsed.Query().Get("state")
	if state == "" || challenge == "" {
		t.Fatalf("authorize url=%s", start.AuthorizeURL)
	}

	callback := httptest.NewRecorder()
	server.Handler().ServeHTTP(callback, httptest.NewRequest(http.MethodGet, "/callback?code=abc&state="+url.QueryEscape(state), nil))
	if callback.Code != http.StatusOK {
		t.Fatalf("callback status=%d body=%s", callback.Code, callback.Body.String())
	}
	if captured.Code != "abc" || captured.Verifier == "" {
		t.Fatalf("exchange captured=%#v", captured)
	}

	status := httptest.NewRecorder()
	server.Handler().ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/v1/auth/login/status/"+start.LoginID+"?pollToken="+url.QueryEscape(start.PollToken), nil))
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), accessToken) {
		t.Fatalf("status=%d body=%s", status.Code, status.Body.String())
	}

	again := httptest.NewRecorder()
	server.Handler().ServeHTTP(again, httptest.NewRequest(http.MethodGet, "/v1/auth/login/status/"+start.LoginID+"?pollToken="+url.QueryEscape(start.PollToken), nil))
	if again.Code != http.StatusGone {
		t.Fatalf("second status=%d body=%s", again.Code, again.Body.String())
	}
}

func TestLoginRejectsWrongStateAndPollToken(t *testing.T) {
	server := NewServer(types.ServerOptions{CodexDir: t.TempDir(), Port: 28888, CasdoorURL: "http://casdoor.test", CasdoorClientID: "cid", OAuthRedirectURL: "http://central.test/callback"})
	start := startLogin(t, server)

	badState := httptest.NewRecorder()
	server.Handler().ServeHTTP(badState, httptest.NewRequest(http.MethodGet, "/callback?code=abc&state=nope", nil))
	if badState.Code == http.StatusOK {
		t.Fatalf("bad state accepted: %d", badState.Code)
	}

	badPoll := httptest.NewRecorder()
	server.Handler().ServeHTTP(badPoll, httptest.NewRequest(http.MethodGet, "/v1/auth/login/status/"+start.LoginID+"?pollToken=wrong", nil))
	if badPoll.Code != http.StatusGone {
		t.Fatalf("bad poll token status=%d", badPoll.Code)
	}
}

func TestLoginStartRequiresCasdoorConfiguration(t *testing.T) {
	server := NewServer(types.ServerOptions{CodexDir: t.TempDir(), Port: 28888})
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/auth/login/start", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/central/ -run TestLogin -v`
Expected: FAIL，报 `types.LoginStart` / `CasdoorURL` 未定义。

- [ ] **Step 3: 加 types 与 ServerOptions 字段**

在 `internal/types/types.go` 的 `ServerOptions` 中追加：

```go
	CasdoorURL       string `json:"casdoorURL,omitempty"`
	CasdoorClientID  string `json:"casdoorClientID,omitempty"`
	CasdoorScopes    string `json:"casdoorScopes,omitempty"`
	OAuthRedirectURL string `json:"oauthRedirectURL,omitempty"`
	UserStoreDir     string `json:"userStoreDir,omitempty"`
```

在 `internal/types/types.go` 末尾追加：

```go
type LoginStart struct {
	LoginID      string    `json:"loginId"`
	PollToken    string    `json:"pollToken"`
	AuthorizeURL string    `json:"authorizeUrl"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

type LoginStatus struct {
	Status      string `json:"status"`
	Subject     string `json:"subject,omitempty"`
	AccessToken string `json:"accessToken,omitempty"`
	ExpiresIn   int    `json:"expiresIn,omitempty"`
	Error       string `json:"error,omitempty"`
}
```

- [ ] **Step 4: 写登录实现**

创建 `internal/central/login.go`：

```go
package central

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"agentpal/internal/types"
)

const loginTTL = 5 * time.Minute

type pendingLogin struct {
	ID           string
	PollToken    string
	State        string
	CodeVerifier string
	Challenge    string
	RedirectURL  string
	CreatedAt    time.Time
	ExpiresAt    time.Time
	Approved     bool
	Failed       bool
	FailReason   string
	Subject      string
	AccessToken  string
	ExpiresIn    int
	Consumed     bool
}

type pendingLogins struct {
	mu    sync.Mutex
	now   func() time.Time
	items map[string]*pendingLogin
}

func newPendingLogins() *pendingLogins {
	return &pendingLogins{now: time.Now, items: make(map[string]*pendingLogin)}
}

func codeChallenge(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func (p *pendingLogins) start(redirectURL string) (*pendingLogin, error) {
	id, err := randomToken(24)
	if err != nil {
		return nil, err
	}
	pollToken, err := randomToken(24)
	if err != nil {
		return nil, err
	}
	state, err := randomToken(24)
	if err != nil {
		return nil, err
	}
	verifier, err := randomToken(32)
	if err != nil {
		return nil, err
	}
	now := p.now()
	item := &pendingLogin{
		ID: id, PollToken: pollToken, State: state,
		CodeVerifier: verifier, Challenge: codeChallenge(verifier),
		RedirectURL: redirectURL, CreatedAt: now, ExpiresAt: now.Add(loginTTL),
	}
	p.mu.Lock()
	p.items[id] = item
	p.mu.Unlock()
	return item, nil
}

func (p *pendingLogins) findByStateLocked(state string) *pendingLogin {
	for _, item := range p.items {
		if item.State == state {
			return item
		}
	}
	return nil
}

func (p *pendingLogins) peekByState(state string) *pendingLogin {
	p.mu.Lock()
	defer p.mu.Unlock()
	item := p.findByStateLocked(state)
	if item == nil || p.now().After(item.ExpiresAt) || item.Consumed {
		return nil
	}
	return item
}

func (p *pendingLogins) approve(state, subject, token string, expiresIn int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	item := p.findByStateLocked(state)
	if item == nil || p.now().After(item.ExpiresAt) || item.Consumed {
		return errors.New("login request expired")
	}
	item.Approved = true
	item.Subject = subject
	item.AccessToken = token
	item.ExpiresIn = expiresIn
	return nil
}

func (p *pendingLogins) fail(state, reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if item := p.findByStateLocked(state); item != nil {
		item.Failed = true
		item.FailReason = reason
	}
}

func (p *pendingLogins) take(id, pollToken string) (types.LoginStatus, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	item := p.items[id]
	if item == nil || p.now().After(item.ExpiresAt) {
		return types.LoginStatus{}, errors.New("login request expired")
	}
	if item.PollToken != pollToken {
		return types.LoginStatus{}, errors.New("login request is not authorized")
	}
	if item.Consumed {
		return types.LoginStatus{}, errors.New("login request already consumed")
	}
	if item.Failed {
		return types.LoginStatus{Status: "failed", Error: item.FailReason}, nil
	}
	if !item.Approved {
		return types.LoginStatus{Status: "pending"}, nil
	}
	item.Consumed = true
	return types.LoginStatus{Status: "approved", Subject: item.Subject, AccessToken: item.AccessToken, ExpiresIn: item.ExpiresIn}, nil
}

func (s *Server) oauthStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if strings.TrimSpace(s.options.CasdoorURL) == "" || strings.TrimSpace(s.options.CasdoorClientID) == "" {
		http.Error(w, "casdoor is not configured", http.StatusServiceUnavailable)
		return
	}
	redirect := strings.TrimSpace(s.options.OAuthRedirectURL)
	if redirect == "" {
		redirect = "http://" + r.Host + "/callback"
	}
	item, err := s.logins.start(redirect)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	scopes := strings.TrimSpace(s.options.CasdoorScopes)
	if scopes == "" {
		scopes = "openid profile"
	}
	values := url.Values{}
	values.Set("client_id", s.options.CasdoorClientID)
	values.Set("response_type", "code")
	values.Set("redirect_uri", redirect)
	values.Set("scope", scopes)
	values.Set("state", item.State)
	values.Set("code_challenge", item.Challenge)
	values.Set("code_challenge_method", "S256")
	authorizeURL := strings.TrimRight(s.options.CasdoorURL, "/") + "/login/oauth/authorize?" + values.Encode()
	writeJSON(w, types.LoginStart{LoginID: item.ID, PollToken: item.PollToken, AuthorizeURL: authorizeURL, ExpiresAt: item.ExpiresAt})
}

func (s *Server) oauthStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/auth/login/status/")
	status, err := s.logins.take(id, r.URL.Query().Get("pollToken"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusGone)
		return
	}
	writeJSON(w, status)
}

func (s *Server) oauthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		http.Error(w, "authorization code is missing", http.StatusBadRequest)
		return
	}
	item := s.logins.peekByState(state)
	if item == nil {
		http.Error(w, "authorization state is invalid", http.StatusGone)
		return
	}
	token, expiresIn, err := s.exchangeCode(item.RedirectURL, code, item.CodeVerifier)
	if err != nil {
		s.logins.fail(state, err.Error())
		http.Error(w, "login failed", http.StatusBadGateway)
		return
	}
	subject, err := ParseJWTSubject(token)
	if err != nil {
		s.logins.fail(state, err.Error())
		http.Error(w, "login failed", http.StatusBadGateway)
		return
	}
	if err := s.logins.approve(state, subject, token, expiresIn); err != nil {
		http.Error(w, err.Error(), http.StatusGone)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, "<!doctype html><meta name=viewport content=width=device-width><title>AgentPal login</title><style>body{font:16px system-ui;max-width:600px;margin:60px auto;padding:20px;color:#172033}</style><h1>Login complete</h1><p>You can return to the AgentPal application.</p>")
}

func (s *Server) exchangeCode(redirectURL, code, verifier string) (string, int, error) {
	if s.oauthClient == nil {
		return "", 0, errors.New("oauth client is not configured")
	}
	payload, err := json.Marshal(map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     s.options.CasdoorClientID,
		"code":          code,
		"redirect_uri":  redirectURL,
		"code_verifier": verifier,
	})
	if err != nil {
		return "", 0, err
	}
	resp, err := s.oauthClient.Post(strings.TrimRight(s.options.CasdoorURL, "/")+"/api/login/oauth/access_token", "application/json", bytes.NewReader(payload))
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", 0, errors.New("casdoor token exchange failed: " + resp.Status)
	}
	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", 0, err
	}
	if result.AccessToken == "" {
		if result.Error != "" {
			return "", 0, errors.New(result.Error)
		}
		return "", 0, errors.New("casdoor did not return an access token")
	}
	return result.AccessToken, result.ExpiresIn, nil
}
```

- [ ] **Step 5: 接入 Server**

在 `internal/central/server.go` 的 `Server` 结构体中加入：

```go
	logins       *pendingLogins
	userProfiles *userProfileStore
	oauthClient  *http.Client
```

在 `NewServer` 中 `s := &Server{...}` 之后加入：

```go
	s.logins = newPendingLogins()
	userStore := options.UserStoreDir
	if strings.TrimSpace(userStore) == "" {
		userStore = filepath.Join(os.TempDir(), "agentpal-user-profiles")
	}
	s.userProfiles = newUserProfileStore(userStore)
	s.oauthClient = &http.Client{Timeout: 15 * time.Second}
```

在 `NewServer` 的路由注册中（`mux.HandleFunc("/agentpal/authorize", ...)` 附近）加入：

```go
	mux.HandleFunc("/v1/auth/login/start", s.oauthStart)
	mux.HandleFunc("/v1/auth/login/status/", s.oauthStatus)
	mux.HandleFunc("/callback", s.oauthCallback)
```

- [ ] **Step 6: 运行测试确认通过**

Run: `go test ./internal/central/ -run TestLogin -v`
Expected: PASS。

- [ ] **Step 7: 提交**

```bash
git add internal/central/login.go internal/central/login_test.go internal/central/server.go internal/types/types.go
git commit -m "Add central PKCE login endpoints and callback"
```

---

### Task 7: 服务端 auth-profiles 接口与启动参数

**Files:**
- Modify: `internal/central/userprofiles.go`（追加 handler 与鉴权辅助）
- Modify: `internal/central/server.go`（路由注册）
- Modify: `cmd/central-node/main.go`
- Test: `internal/central/userprofiles_test.go`

**Interfaces:**
- Consumes: `userProfileStore`（Task 5）、`ParseJWTSubject`（Task 4）、`types.RemoteAuthProfile` / `RemoteAuthProfileDetail`（Task 5）
- Produces:
  - `func (s *Server) requireSubject(r *http.Request) (string, error)`
  - `func (s *Server) authProfiles(w, r)`
  - 路由 `GET /v1/auth-profiles`、`GET /v1/auth-profiles/<name>`

- [ ] **Step 1: 写失败测试**

在 `internal/central/userprofiles_test.go` 末尾追加：

```go
func TestAuthProfilesRouteIsolatesUsers(t *testing.T) {
	root := t.TempDir()
	writeReservedProfile(t, root, "user-1", "yangpu", "yangpu")
	writeReservedProfile(t, root, "user-2", "other", "other")
	server := NewServer(types.ServerOptions{CodexDir: t.TempDir(), Port: 28888, UserStoreDir: root})

	list := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/auth-profiles", nil)
	req.Header.Set("Authorization", "Bearer "+unsignedJWT(t, map[string]any{"sub": "user-1"}))
	server.Handler().ServeHTTP(list, req)
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	if !strings.Contains(list.Body.String(), "yangpu") || strings.Contains(list.Body.String(), "other") {
		t.Fatalf("list body=%s", list.Body.String())
	}

	detail := httptest.NewRecorder()
	detailReq := httptest.NewRequest(http.MethodGet, "/v1/auth-profiles/yangpu", nil)
	detailReq.Header.Set("Authorization", "Bearer "+unsignedJWT(t, map[string]any{"sub": "user-1"}))
	server.Handler().ServeHTTP(detail, detailReq)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), "configAuthToml") {
		t.Fatalf("detail status=%d body=%s", detail.Code, detail.Body.String())
	}

	missing := httptest.NewRecorder()
	server.Handler().ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/v1/auth-profiles", nil))
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("missing auth status=%d", missing.Code)
	}

	traversal := httptest.NewRecorder()
	traversalReq := httptest.NewRequest(http.MethodGet, "/v1/auth-profiles/..%2Fsecret", nil)
	traversalReq.Header.Set("Authorization", "Bearer "+unsignedJWT(t, map[string]any{"sub": "user-1"}))
	server.Handler().ServeHTTP(traversal, traversalReq)
	if traversal.Code == http.StatusOK {
		t.Fatalf("traversal accepted: %s", traversal.Body.String())
	}
}
```

在文件头部 import 中加入 `net/http`、`net/http/httptest`、`strings`、`agentpal/internal/types`。

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/central/ -run TestAuthProfilesRouteIsolatesUsers -v`
Expected: FAIL，404（路由未注册）。

- [ ] **Step 3: 写 handler**

在 `internal/central/userprofiles.go` 末尾追加：

```go
func (s *Server) requireSubject(r *http.Request) (string, error) {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if token == "" {
		return "", errors.New("bearer token is required")
	}
	return ParseJWTSubject(token)
}

func (s *Server) authProfiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	subject, err := s.requireSubject(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/v1/auth-profiles")
	name = strings.TrimPrefix(name, "/")
	if name == "" {
		items, err := s.userProfiles.list(subject)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, items)
		return
	}
	profile, err := s.userProfiles.read(subject, name)
	if err != nil {
		http.Error(w, "profile not found", http.StatusNotFound)
		return
	}
	var auth map[string]any
	if err := json.Unmarshal(profile.AuthJSON, &auth); err != nil {
		auth = map[string]any{}
	}
	writeJSON(w, types.RemoteAuthProfileDetail{
		Name: profile.Meta.Name, Provider: profile.Meta.Provider, Type: profile.Meta.Type,
		ContentHash: profile.ContentHash, UpdatedAt: profile.UpdatedAt,
		Auth: auth, ConfigAuthToml: string(profile.ConfigTOML),
	})
}
```

并在 `userprofiles.go` 的 import 中加入 `"strings"`。

- [ ] **Step 4: 注册路由**

在 `internal/central/server.go` 的 `NewServer` 路由注册中追加：

```go
	mux.HandleFunc("/v1/auth-profiles", s.authProfiles)
	mux.HandleFunc("/v1/auth-profiles/", s.authProfiles)
```

- [ ] **Step 5: 增加启动参数**

在 `cmd/central-node/main.go` 的 flag 定义后追加：

```go
	casdoorURL := flag.String("casdoor-url", "https://id.yprenew.com", "Casdoor base URL")
	casdoorClientID := flag.String("casdoor-client-id", "", "Casdoor application client ID")
	casdoorScopes := flag.String("casdoor-scopes", "openid profile", "Casdoor authorization scopes")
	oauthRedirectURL := flag.String("oauth-redirect-url", "", "OAuth redirect URL registered on the Casdoor application")
	userStoreDir := flag.String("user-store-dir", "~/.agentpal/server/users", "central user reserved profile directory")
```

在 `flag.Parse()` 之后、`expandedBackupDir` 之后追加：

```go
	expandedUserStoreDir, err := codex.ExpandPath(*userStoreDir)
	if err != nil {
		log.Fatal(err)
	}
```

在 `central.NewServer(types.ServerOptions{...})` 中追加字段：

```go
		CasdoorURL:       *casdoorURL,
		CasdoorClientID:  *casdoorClientID,
		CasdoorScopes:    *casdoorScopes,
		OAuthRedirectURL: *oauthRedirectURL,
		UserStoreDir:     expandedUserStoreDir,
```

- [ ] **Step 6: 运行测试确认通过**

Run: `go test ./internal/central/ -v && go build ./cmd/central-node/`
Expected: PASS，构建成功。

- [ ] **Step 7: 提交**

```bash
git add internal/central/userprofiles.go internal/central/userprofiles_test.go internal/central/server.go cmd/central-node/main.go
git commit -m "Serve per-user reserved auth profiles with JWT sub isolation"
```

---

### Task 8: 客户端 HTTP 方法与登录状态存储

**Files:**
- Modify: `internal/central/client.go`
- Create: `internal/login/store.go`
- Test: `internal/central/client_test.go`、`internal/login/store_test.go`
- Modify: `internal/types/types.go`（新增 `CentralLoginState`）

**Interfaces:**
- Consumes: `types.LoginStart`、`types.LoginStatus`、`types.RemoteAuthProfile`、`types.RemoteAuthProfileDetail`
- Produces:
  - `func (c Client) StartLogin(base string) (types.LoginStart, error)`
  - `func (c Client) LoginStatus(base, loginID, pollToken string) (types.LoginStatus, error)`
  - `func (c Client) CentralAuthProfiles(base, token string) ([]types.RemoteAuthProfile, error)`
  - `func (c Client) CentralAuthProfile(base, token, name string) (types.RemoteAuthProfileDetail, error)`
  - `types.CentralLoginState{LoggedIn bool; Subject string; ExpiresAt time.Time}`
  - `login.State{Subject, AccessToken string; ExpiresAt time.Time}`、`login.Read/Write/Clear(root string)`

- [ ] **Step 1: 写失败测试**

创建 `internal/login/store_test.go`：

```go
package login

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoginStateRoundTrip(t *testing.T) {
	root := t.TempDir()
	if state, err := Read(root); err != nil || state.AccessToken != "" {
		t.Fatalf("empty read: %#v err=%v", state, err)
	}
	state := State{Subject: "user-1", AccessToken: "token", ExpiresAt: time.Now().Add(time.Hour).UTC().Truncate(time.Second)}
	if err := Write(root, state); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(root, "login.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
	got, err := Read(root)
	if err != nil || got.Subject != "user-1" || got.AccessToken != "token" {
		t.Fatalf("read: %#v err=%v", got, err)
	}
	if err := Clear(root); err != nil {
		t.Fatal(err)
	}
	if state, err := Read(root); err != nil || state.AccessToken != "" {
		t.Fatalf("read after clear: %#v err=%v", state, err)
	}
}
```

在 `internal/central/client_test.go` 末尾追加：

```go
func TestClientLoginAndProfileCalls(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/auth/login/start", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"loginId": "id-1", "pollToken": "poll-1", "authorizeUrl": "http://casdoor/authorize", "expiresAt": time.Now().Add(time.Minute)})
	})
	mux.HandleFunc("/v1/auth/login/status/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pollToken") != "poll-1" {
			http.Error(w, "bad poll token", http.StatusGone)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "approved", "subject": "user-1", "accessToken": "token", "expiresIn": 3600})
	})
	mux.HandleFunc("/v1/auth-profiles", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{"name": "yangpu", "provider": "yangpu", "type": "api_key", "contentHash": "sha256:x", "updatedAt": time.Now()}})
	})
	mux.HandleFunc("/v1/auth-profiles/yangpu", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "yangpu", "provider": "yangpu", "type": "api_key", "contentHash": "sha256:x", "updatedAt": time.Now(), "auth": map[string]any{"OPENAI_API_KEY": "sk-1"}, "configAuthToml": "model_provider = \"yangpu\"\n"})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient(nil, 5*time.Second)
	start, err := client.StartLogin(server.URL)
	if err != nil || start.LoginID != "id-1" {
		t.Fatalf("start=%#v err=%v", start, err)
	}
	status, err := client.LoginStatus(server.URL, "id-1", "poll-1")
	if err != nil || status.Status != "approved" || status.AccessToken != "token" {
		t.Fatalf("status=%#v err=%v", status, err)
	}
	list, err := client.CentralAuthProfiles(server.URL, "token")
	if err != nil || len(list) != 1 || list[0].Name != "yangpu" {
		t.Fatalf("list=%#v err=%v", list, err)
	}
	detail, err := client.CentralAuthProfile(server.URL, "token", "yangpu")
	if err != nil || detail.Name != "yangpu" || detail.ConfigAuthToml == "" {
		t.Fatalf("detail=%#v err=%v", detail, err)
	}
	if _, err := client.CentralAuthProfiles(server.URL, "wrong"); err == nil {
		t.Fatal("expected unauthorized error")
	}
}
```

确保 `client_test.go` 导入 `encoding/json`、`net/http`、`net/http/httptest`、`time`。

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/central/ -run TestClientLoginAndProfileCalls -v; go test ./internal/login/ -v`
Expected: FAIL（方法未定义 / 包不存在）。

- [ ] **Step 3: 写 login 包**

创建 `internal/login/store.go`：

```go
package login

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type State struct {
	Subject     string    `json:"subject"`
	AccessToken string    `json:"accessToken"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

func path(root string) string { return filepath.Join(root, "login.json") }

func Read(root string) (State, error) {
	data, err := os.ReadFile(path(root))
	if os.IsNotExist(err) {
		return State{}, nil
	}
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, err
	}
	return state, nil
}

func Write(root string, state State) error {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path(root), data, 0o600)
}

func Clear(root string) error {
	err := os.Remove(path(root))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
```

- [ ] **Step 4: 写 client 方法**

在 `internal/central/client.go` 末尾追加（并在 import 中加入 `"bytes"`、`"net/url"`）：

```go
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
```

在 `internal/types/types.go` 末尾追加：

```go
type CentralLoginState struct {
	LoggedIn  bool      `json:"loggedIn"`
	Subject   string    `json:"subject,omitempty"`
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
}
```

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/central/ ./internal/login/ -v`
Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add internal/central/client.go internal/central/client_test.go internal/login/store.go internal/login/store_test.go internal/types/types.go
git commit -m "Add client login and central profile HTTP calls with login state store"
```

---

### Task 9: App 登录与同步方法

**Files:**
- Modify: `app.go`
- Modify: `internal/types/types.go`（新增 `AuthSyncItem`、`AuthSyncReport`）
- Test: `app_test.go`

**Interfaces:**
- Consumes: `client.StartLogin/LoginStatus/CentralAuthProfiles/CentralAuthProfile`（Task 8）、`login.Read/Write/Clear`（Task 8）、`authprofile.Store.FindByReservedName/AddCentral/OverwriteCentral/ContentHashFor`（Task 3）
- Produces:
  - `func (a *App) StartCentralLogin() (types.LoginStart, error)`
  - `func (a *App) PollCentralLogin(loginID, pollToken string) (types.LoginStatus, error)`
  - `func (a *App) GetCentralLoginState() (types.CentralLoginState, error)`
  - `func (a *App) LogoutCentralLogin() error`
  - `func (a *App) SyncCentralAuthProfiles() (types.AuthSyncReport, error)`
  - `func (a *App) ApplyCentralAuthProfile(name string) (types.AuthProfile, error)`
  - `types.AuthSyncItem{Name, Status, Detail}`、`types.AuthSyncReport{Items []AuthSyncItem}`

- [ ] **Step 1: 写失败测试**

在 `app_test.go` 末尾追加（该文件 package 为 `main`，在 import 中加入 `agentpal/internal/types`，`agentpal/internal/authprofile` 已存在）：

```go
func TestFindLocalByReservedName(t *testing.T) {
	profiles := []authprofile.AuthProfile{
		{ID: "a", Name: "Work", Provider: "openai"},
		{ID: "b", Name: "yangpu", ReservedName: "yangpu"},
	}
	found, ok := findLocalReserved(profiles, "yangpu")
	if !ok || found.ID != "b" {
		t.Fatalf("found=%#v ok=%v", found, ok)
	}
	if _, ok := findLocalReserved(profiles, "missing"); ok {
		t.Fatal("unexpected match")
	}
	if _, ok := findLocalReserved(profiles, "openai"); ok {
		t.Fatal("must not match by provider")
	}
}

func TestCentralProfileFromDetail(t *testing.T) {
	detail := types.RemoteAuthProfileDetail{Name: "yangpu", Provider: "yangpu", Type: "api_key", Auth: map[string]any{"OPENAI_API_KEY": "sk-1"}, ConfigAuthToml: "model_provider = \"yangpu\"\n"}
	profile := centralProfileFrom(detail)
	if profile.Name != "yangpu" || profile.Provider != "yangpu" || string(profile.AuthJSON) == "" || string(profile.ConfigTOML) == "" {
		t.Fatalf("profile=%#v", profile)
	}
}
```

确保 `app_test.go` 导入 `agentpal/internal/authprofile` 与 `agentpal/internal/types`。

- [ ] **Step 2: 运行测试确认失败**

Run: `go test . -run 'TestFindLocalByReservedName|TestCentralProfileFromDetail' -v`
Expected: FAIL，报 `undefined: findLocalReserved`。

- [ ] **Step 3: 加 types**

在 `internal/types/types.go` 末尾追加：

```go
type AuthSyncItem struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type AuthSyncReport struct {
	Items []AuthSyncItem `json:"items"`
}
```

- [ ] **Step 4: 写 App 方法**

在 `app.go` 的 import 中加入 `"agentpal/internal/login"` 与 `"encoding/json"`。

在 `app.go` 末尾追加：

```go
func findLocalReserved(profiles []authprofile.AuthProfile, name string) (authprofile.AuthProfile, bool) {
	for _, profile := range profiles {
		if profile.ReservedName == name {
			return profile, true
		}
	}
	return authprofile.AuthProfile{}, false
}

func centralProfileFrom(detail types.RemoteAuthProfileDetail) authprofile.CentralProfile {
	authJSON, _ := json.Marshal(detail.Auth)
	return authprofile.CentralProfile{
		Name: detail.Name, Provider: detail.Provider, Type: detail.Type,
		AuthJSON: authJSON, ConfigTOML: []byte(detail.ConfigAuthToml),
	}
}

func (a *App) StartCentralLogin() (types.LoginStart, error) {
	state, err := appconfig.ReadState()
	if err != nil {
		return types.LoginStart{}, err
	}
	result, err := central.NewClient(nil, 15*time.Second).StartLogin(centerBase(state))
	if err != nil {
		return types.LoginStart{}, err
	}
	if a.ctx != nil {
		wailsruntime.BrowserOpenURL(a.ctx, result.AuthorizeURL)
	}
	return result, nil
}

func (a *App) PollCentralLogin(loginID, pollToken string) (types.LoginStatus, error) {
	state, err := appconfig.ReadState()
	if err != nil {
		return types.LoginStatus{}, err
	}
	status, err := central.NewClient(nil, 15*time.Second).LoginStatus(centerBase(state), loginID, pollToken)
	if err != nil {
		return types.LoginStatus{}, err
	}
	if status.Status == "approved" {
		root, err := appconfig.StateDir()
		if err != nil {
			return types.LoginStatus{}, err
		}
		expires := time.Now().Add(24 * time.Hour)
		if status.ExpiresIn > 0 {
			expires = time.Now().Add(time.Duration(status.ExpiresIn) * time.Second)
		}
		if err := login.Write(root, login.State{Subject: status.Subject, AccessToken: status.AccessToken, ExpiresAt: expires}); err != nil {
			return types.LoginStatus{}, err
		}
	}
	return status, nil
}

func (a *App) GetCentralLoginState() (types.CentralLoginState, error) {
	root, err := appconfig.StateDir()
	if err != nil {
		return types.CentralLoginState{}, err
	}
	state, err := login.Read(root)
	if err != nil {
		return types.CentralLoginState{}, err
	}
	if state.AccessToken == "" || time.Now().After(state.ExpiresAt) {
		return types.CentralLoginState{LoggedIn: false}, nil
	}
	return types.CentralLoginState{LoggedIn: true, Subject: state.Subject, ExpiresAt: state.ExpiresAt}, nil
}

func (a *App) LogoutCentralLogin() error {
	root, err := appconfig.StateDir()
	if err != nil {
		return err
	}
	return login.Clear(root)
}

func (a *App) SyncCentralAuthProfiles() (types.AuthSyncReport, error) {
	state, err := appconfig.ReadState()
	if err != nil {
		return types.AuthSyncReport{}, err
	}
	root, err := appconfig.StateDir()
	if err != nil {
		return types.AuthSyncReport{}, err
	}
	session, err := login.Read(root)
	if err != nil {
		return types.AuthSyncReport{}, err
	}
	if session.AccessToken == "" || time.Now().After(session.ExpiresAt) {
		return types.AuthSyncReport{}, errors.New("not logged in")
	}
	client := central.NewClient(nil, 15*time.Second)
	base := centerBase(state)
	remote, err := client.CentralAuthProfiles(base, session.AccessToken)
	if err != nil {
		return types.AuthSyncReport{}, err
	}
	profilesRoot, err := codex.ExpandPath("~/.agentpal/auth-profiles")
	if err != nil {
		return types.AuthSyncReport{}, err
	}
	store := authprofile.NewStore(profilesRoot)
	local, err := store.List()
	if err != nil {
		return types.AuthSyncReport{}, err
	}
	report := types.AuthSyncReport{Items: []types.AuthSyncItem{}}
	for _, item := range remote {
		existing, found := findLocalReserved(local, item.Name)
		if !found {
			detail, err := client.CentralAuthProfile(base, session.AccessToken, item.Name)
			if err != nil {
				report.Items = append(report.Items, types.AuthSyncItem{Name: item.Name, Status: "error", Detail: err.Error()})
				continue
			}
			if _, err := store.AddCentral(centralProfileFrom(detail)); err != nil {
				report.Items = append(report.Items, types.AuthSyncItem{Name: item.Name, Status: "error", Detail: err.Error()})
				continue
			}
			report.Items = append(report.Items, types.AuthSyncItem{Name: item.Name, Status: "added"})
			continue
		}
		localHash := existing.ContentHash
		if localHash == "" {
			if computed, err := store.ContentHashFor(existing.ID); err == nil {
				localHash = computed
			}
		}
		if localHash != "" && localHash == item.ContentHash {
			report.Items = append(report.Items, types.AuthSyncItem{Name: item.Name, Status: "synced"})
			continue
		}
		report.Items = append(report.Items, types.AuthSyncItem{Name: item.Name, Status: "needs_sync"})
	}
	return report, nil
}

func (a *App) ApplyCentralAuthProfile(name string) (types.AuthProfile, error) {
	state, err := appconfig.ReadState()
	if err != nil {
		return types.AuthProfile{}, err
	}
	root, err := appconfig.StateDir()
	if err != nil {
		return types.AuthProfile{}, err
	}
	session, err := login.Read(root)
	if err != nil {
		return types.AuthProfile{}, err
	}
	if session.AccessToken == "" || time.Now().After(session.ExpiresAt) {
		return types.AuthProfile{}, errors.New("not logged in")
	}
	profilesRoot, err := codex.ExpandPath("~/.agentpal/auth-profiles")
	if err != nil {
		return types.AuthProfile{}, err
	}
	store := authprofile.NewStore(profilesRoot)
	local, err := store.List()
	if err != nil {
		return types.AuthProfile{}, err
	}
	existing, found := findLocalReserved(local, name)
	if !found {
		return types.AuthProfile{}, errors.New("no local profile for reserved name: " + name)
	}
	detail, err := central.NewClient(nil, 15*time.Second).CentralAuthProfile(centerBase(state), session.AccessToken, name)
	if err != nil {
		return types.AuthProfile{}, err
	}
	updated, err := store.OverwriteCentral(existing.ID, centralProfileFrom(detail))
	if err != nil {
		return types.AuthProfile{}, err
	}
	return profileType(updated), nil
}
```

- [ ] **Step 5: 运行测试确认通过**

Run: `go test . -v && go build ./...`
Expected: PASS，构建成功。

- [ ] **Step 6: 提交**

```bash
git add app.go app_test.go internal/types/types.go
git commit -m "Add app-level central login and reserved profile sync"
```

---

### Task 10: 前端登录区与同步报告

**Files:**
- Modify: `frontend/src/wails.ts`
- Modify: `frontend/wailsjs/go/main/App.d.ts`
- Modify: `frontend/wailsjs/go/main/App.js`
- Modify: `frontend/wailsjs/go/models.ts`
- Modify: `frontend/src/App.tsx`

**Interfaces:**
- Consumes: Task 9 的 6 个 App 方法
- Produces: 客户端 `Authentication` 标签中的登录区、同步报告与冲突确认

- [ ] **Step 1: 加 TypeScript 类型与绑定**

在 `frontend/src/wails.ts` 的 `SettingsRequest` 之后加入：

```ts
export type LoginStart = { loginId: string; pollToken: string; authorizeUrl: string; expiresAt: string };
export type LoginStatus = { status: 'pending' | 'approved' | 'failed'; subject?: string; accessToken?: string; expiresIn?: number; error?: string };
export type CentralLoginState = { loggedIn: boolean; subject?: string; expiresAt?: string };
export type AuthSyncItem = { name: string; status: 'added' | 'synced' | 'needs_sync' | 'error'; detail?: string };
export type AuthSyncReport = { items: AuthSyncItem[] };
```

在该文件的 `Backend` 类型中加入：

```ts
  StartCentralLogin(): Promise<LoginStart>;
  PollCentralLogin(loginId: string, pollToken: string): Promise<LoginStatus>;
  GetCentralLoginState(): Promise<CentralLoginState>;
  LogoutCentralLogin(): Promise<void>;
  SyncCentralAuthProfiles(): Promise<AuthSyncReport>;
  ApplyCentralAuthProfile(name: string): Promise<AuthProfile>;
```

在 `frontend/wailsjs/go/main/App.d.ts` 末尾加入：

```ts
export function StartCentralLogin():Promise<types.LoginStart>;

export function PollCentralLogin(arg1:string,arg2:string):Promise<types.LoginStatus>;

export function GetCentralLoginState():Promise<types.CentralLoginState>;

export function LogoutCentralLogin():Promise<void>;

export function SyncCentralAuthProfiles():Promise<types.AuthSyncReport>;

export function ApplyCentralAuthProfile(arg1:string):Promise<types.AuthProfile>;
```

在 `frontend/wailsjs/go/main/App.js` 末尾加入：

```js
export function StartCentralLogin() {
  return window['go']['main']['App']['StartCentralLogin']();
}

export function PollCentralLogin(arg1, arg2) {
  return window['go']['main']['App']['PollCentralLogin'](arg1, arg2);
}

export function GetCentralLoginState() {
  return window['go']['main']['App']['GetCentralLoginState']();
}

export function LogoutCentralLogin() {
  return window['go']['main']['App']['LogoutCentralLogin']();
}

export function SyncCentralAuthProfiles() {
  return window['go']['main']['App']['SyncCentralAuthProfiles']();
}

export function ApplyCentralAuthProfile(arg1) {
  return window['go']['main']['App']['ApplyCentralAuthProfile'](arg1);
}
```

在 `frontend/wailsjs/go/models.ts` 的 `types` namespace 内追加新类（放在 `AuthProfile` 类之后即可）。注意该文件对 Go `time.Time` 的统一写法是 `// Go type: time` + `any` + `this.convertValues(...)`，且含嵌套类型引用的类必须带 `convertValues` 方法：

```ts
	export class LoginStart {
	    loginId: string;
	    pollToken: string;
	    authorizeUrl: string;
	    // Go type: time
	    expiresAt: any;

	    static createFrom(source: any = {}) {
	        return new LoginStart(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.loginId = source["loginId"];
	        this.pollToken = source["pollToken"];
	        this.authorizeUrl = source["authorizeUrl"];
	        this.expiresAt = this.convertValues(source["expiresAt"], null);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

	export class LoginStatus {
	    status: string;
	    subject?: string;
	    accessToken?: string;
	    expiresIn?: number;
	    error?: string;

	    static createFrom(source: any = {}) {
	        return new LoginStatus(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.subject = source["subject"];
	        this.accessToken = source["accessToken"];
	        this.expiresIn = source["expiresIn"];
	        this.error = source["error"];
	    }
	}

	export class CentralLoginState {
	    loggedIn: boolean;
	    subject?: string;
	    // Go type: time
	    expiresAt?: any;

	    static createFrom(source: any = {}) {
	        return new CentralLoginState(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.loggedIn = source["loggedIn"];
	        this.subject = source["subject"];
	        this.expiresAt = this.convertValues(source["expiresAt"], null);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

	export class AuthSyncItem {
	    name: string;
	    status: string;
	    detail?: string;

	    static createFrom(source: any = {}) {
	        return new AuthSyncItem(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.status = source["status"];
	        this.detail = source["detail"];
	    }
	}

	export class AuthSyncReport {
	    items: AuthSyncItem[];

	    static createFrom(source: any = {}) {
	        return new AuthSyncReport(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], AuthSyncItem);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
```

- [ ] **Step 2: 加前端登录组件**

在 `frontend/src/App.tsx` 中确认顶部已 `import { api, ... } from './wails';`（既有代码已如此）。本任务只新增组件逻辑，不需要引入自动生成的绑定模块。
在 `AuthenticationTabCompact` 组件内部（`const hasCurrentProfile = ...` 之前）加入登录状态与逻辑：

```tsx
  const [login, setLogin] = useState<{ loggedIn: boolean; subject?: string }>({ loggedIn: false });
  const [syncItems, setSyncItems] = useState<{ name: string; status: string; detail?: string }[]>([]);
  const [loginBusy, setLoginBusy] = useState(false);

  useEffect(() => { void api?.GetCentralLoginState().then((state) => setLogin({ loggedIn: state.loggedIn, subject: state.subject })).catch(() => setLogin({ loggedIn: false })); }, []);

  async function runSync() {
    if (!api) return;
    try {
      const report = await api.SyncCentralAuthProfiles();
      setSyncItems(report.items);
      await reload();
      setNotice({ kind: 'success', text: `Central profiles checked: ${report.items.length}.` });
    } catch (error) {
      setNotice({ kind: 'error', text: String(error) });
    }
  }

  async function startLogin() {
    if (!api || loginBusy) return;
    setLoginBusy(true);
    try {
      const start = await api.StartCentralLogin();
      const deadline = new Date(start.expiresAt).getTime();
      while (Date.now() < deadline) {
        await new Promise((resolve) => setTimeout(resolve, 2000));
        let status;
        try {
          status = await api.PollCentralLogin(start.loginId, start.pollToken);
        } catch {
          break;
        }
        if (status.status === 'approved') {
          setLogin({ loggedIn: true, subject: status.subject });
          setNotice({ kind: 'success', text: 'Logged in with Casdoor.' });
          await runSync();
          return;
        }
        if (status.status === 'failed') {
          setNotice({ kind: 'error', text: status.error || 'Central login failed.' });
          return;
        }
      }
      setNotice({ kind: 'warning', text: 'Central login was not completed.' });
    } catch (error) {
      setNotice({ kind: 'error', text: String(error) });
    } finally {
      setLoginBusy(false);
    }
  }

  async function logout() {
    if (!api) return;
    await api.LogoutCentralLogin();
    setLogin({ loggedIn: false });
    setSyncItems([]);
  }

  async function overwrite(name: string) {
    if (!api) return;
    try {
      await api.ApplyCentralAuthProfile(name);
      await reload();
      await runSync();
      setNotice({ kind: 'success', text: `Overwrote local profile ${name} from central.` });
    } catch (error) {
      setNotice({ kind: 'error', text: String(error) });
    }
  }
```

在 `AuthenticationTabCompact` 返回的 JSX 中，`section-heading` 之后、当前档案提示之前插入登录区：

```tsx
    <div className="current-auth">
      <div className="current-auth-line">
        {login.loggedIn
          ? <><strong>Casdoor: {login.subject}</strong><button className="secondary" onClick={() => void runSync()}>Sync central profiles</button><button className="secondary" onClick={() => void logout()}>Logout</button></>
          : <><strong>Central login</strong><button className="primary" onClick={() => void startLogin()} disabled={loginBusy}>{loginBusy ? 'Waiting for login...' : 'Login with Casdoor'}</button></>}
      </div>
    </div>
    {syncItems.length > 0 && <div className="profile-list">{syncItems.map((item) => <article className="profile-row" key={item.name}>
      <div className="profile-main"><strong>{item.name}</strong><small>{item.status}{item.detail ? ` · ${item.detail}` : ''}</small></div>
      <div className="profile-actions">
        {item.status === 'needs_sync' && <button className="primary" onClick={() => void overwrite(item.name)}>Overwrite local</button>}
        {item.status === 'needs_sync' && <button className="secondary" onClick={() => setSyncItems(syncItems.filter((entry) => entry.name !== item.name))}>Keep local</button>}
      </div>
    </article>)}</div>}
```

- [ ] **Step 3: 构建前端确认通过**

Run: `cd frontend && npm run build`
Expected: `tsc` 无错误，`vite build` 输出 `dist/`。

- [ ] **Step 4: 运行完整 Go 测试与构建**

Run: `go test ./... && go vet ./... && go build ./...`
Expected: 全部通过。

- [ ] **Step 5: 提交**

```bash
git add frontend/src/wails.ts frontend/src/App.tsx frontend/wailsjs/go/main/App.d.ts frontend/wailsjs/go/main/App.js frontend/wailsjs/go/models.ts
git commit -m "Add central Casdoor login and sync panel to the client"
```

---

### Task 11: README 与端到端验证

**Files:**
- Modify: `README.md`
- Test: 手动端到端

**Interfaces:**
- Consumes: 全部前序任务
- Produces: 更新后的文档与一次完整验证记录

- [ ] **Step 1: 更新 README**

在 `README.md` 的 “Server API” 代码块中追加：

```text
POST /v1/auth/login/start
GET  /v1/auth/login/status/<loginId>?pollToken=<pollToken>
GET  /callback
GET  /v1/auth-profiles
GET  /v1/auth-profiles/<name>
```

在 “认证档案” 小节后新增一节：

````markdown
## 中心登录与预留 profile

Central Host 通过 Casdoor（默认 `https://id.yprenew.com`）做 OAuth 登录，OAuth 回调地址为 `{central_host}:{central_port}/callback`，必须在 Casdoor 应用里登记。

服务端启动参数：

```text
-casdoor-url         默认 https://id.yprenew.com
-casdoor-client-id   Casdoor 应用 client ID（必填，否则登录接口返回 503）
-casdoor-scopes      默认 "openid profile"
-oauth-redirect-url  Casdoor 中登记的 Redirect URI
-user-store-dir      默认 ~/.agentpal/server/users
```

管理员按用户 `sub` 手动放置预留 profile（服务端只读）：

```text
~/.agentpal/server/users/<sub>/auth-profiles/<name>/
  profile.json       {"name":"<name>","type":"api_key","provider":"<name>"}
  auth.json          {"OPENAI_API_KEY":"..."}
  config-auth.toml   model_provider = "<name>" 且 [model_providers.<name>]
```

客户端登录后按预留名称同步：本地无同名则新增；一致则标记已同步；不一致只提示，需用户确认后才覆盖本地 profile。同步不会激活 profile，也不会修改 `~/.codex`。

> 安全提示：Central Host 只解析 JWT 的 `sub`，不校验签名。任何能访问 Central Host 的客户端都能伪造 `sub` 读取对应用户的 profile。该能力仅适用于可信内网。
````

若有 “非目标” 列表，移除其中与本功能冲突的「账号、权限」条目或改为说明「仅中心登录，不做权限控制」。

- [ ] **Step 2: 运行完整验证**

Run: `go test ./... && go vet ./... && go build ./... && cd frontend && npm run build`
Expected: 全部通过。

- [ ] **Step 3: 端到端验收**

1. 准备一个 Central Host 用户目录：

```bash
mkdir -p ~/.agentpal/server/users/user-1/auth-profiles/yangpu
printf '{"name":"yangpu","type":"api_key","provider":"yangpu"}' > ~/.agentpal/server/users/user-1/auth-profiles/yangpu/profile.json
printf '{"OPENAI_API_KEY":"sk-mem-demo"}' > ~/.agentpal/server/users/user-1/auth-profiles/yangpu/auth.json
printf 'model_provider = "yangpu"\n\n[model_providers.yangpu]\nname = "yangpu"\nbase_url = "https://memory.yprenewables.com/codex/default"\nwire_api = "responses"\n' > ~/.agentpal/server/users/user-1/auth-profiles/yangpu/config-auth.toml
```

2. 启动 Central Host（本机演示）：

```bash
go run ./cmd/central-node -casdoor-client-id 177766c4b7eff69c31c5 -oauth-redirect-url http://127.0.0.1:28888/callback
```

3. 启动客户端（另一个终端）：

```bash
wails dev
```

4. 在客户端 `Authentication` 标签点击 `Login with Casdoor`，完成浏览器登录。
   - 预期：登录成功，自动同步，`yangpu` 显示为 `added`，本地档案列表出现 `yangpu`。
5. 再次点击 `Sync central profiles`。
   - 预期：`yangpu` 显示 `synced`。
6. 修改本地 `~/.agentpal/auth-profiles/<id>/auth.json` 内容，再同步。
   - 预期：`yangpu` 显示 `needs_sync`，本地未被改写。
7. 点击 `Overwrite local`。
   - 预期：本地内容恢复为与中心一致；`~/.codex` 未发生变化（用 `git status` 或文件时间戳确认）。

- [ ] **Step 4: 提交**

```bash
git add README.md
git commit -m "Document central Casdoor login and reserved profile sync"
```

---

## 已知风险与后续

- 不验签的 `sub` 解析允许内网伪造身份；如需收紧，后续在 `requireSubject` 中接入 Casdoor JWKS 或 introspection。
- access token 以 0600 明文存 `~/.agentpal/login.json`，与现有认证档案同一安全档位；后续可迁移到系统凭据存储。
- 未实现 refresh token：token 过期后需重新登录。
- 中心预留 profile 目前需管理员登录 Central Host 手动放目录；后续可加管理页面。
