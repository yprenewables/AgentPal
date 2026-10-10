# AgentPal 中心节点/管理后台独立为单独仓库 `agentpal-central`（设计）

日期：2026-10-10

## 背景与目标

当前仓库只有一个 Go module `agentpal`：桌面客户端与「中心节点服务端（含管理后台）」混在同一仓库、同一 module、共用 `internal/*` 包。上一轮已移除客户端内嵌 server 模式，但服务端代码（`internal/centralnode`、`cmd/central-node`）仍与客户端同仓。

目标：把**整个中心节点服务端**（客户端-facing 的配置清单/备份/设备登录 + 管理后台）迁到**一个独立仓库**，与客户端仓库彻底分离，避免任何歧义。

本次要达成：
- 新仓库 `agentpal-central`（本地 `~/aiProjs/agentpal-central`，远端 `github.com/cjb1234567/agentpal-central`，私有）。
- 新仓库可独立 `go build` / `go test`，**不依赖** AgentPal 仓库。
- AgentPal 仓库不再包含任何服务端代码。
- 新仓库以**一次全新的初始提交**起步（不抽取旧历史；历史保留在 AgentPal 中可查）。

## 非目标

- 不抽取 git 历史到新仓库（明确选择全新初始提交）。
- 不引入两仓库共享的 module/包（明确选择「不共享」）。
- 不改变中心节点对客户端暴露的 HTTP 接口与数据格式。
- 不改变管理后台的功能与 UI。

## 目标结构

两个独立仓库，互不依赖：

```
~/aiProjs/AgentPal/                （客户端仓库，module github.com/cjb1234567/AgentPal 保持不变）
  main.go / app.go / frontend/
  internal/
    authprofile/ codex/ security/ constants/ centralscan/ types/   （客户端仍在使用，原样保留）
    centralclient/ centraltest/ config/ login/ update/ platform/
  （删除 internal/centralnode/、cmd/central-node/）

~/aiProjs/agentpal-central/        （新仓库，module github.com/cjb1234567/agentpal-central）
  go.mod
  main.go
  internal/
    server/        server.go / admin_profiles.go / admin_page.html /
                   userprofiles.go / settings.go / profile_template.go /
                   auth.go / jwt.go / login.go / device.go / manifest.go + 测试
    types/         服务端用到的结构体子集
    codex/         仅 ReadMCP + MCPEntry
    security/      路径校验（SafeJoin / ValidateSkillRelPath / IsIgnoredName）
    authprofile/   content / archive / central / AuthProfile 子集
    constants/     AppName / AppVersion / DefaultPort
    centralscan/   ScanSkills
    testutil/      MakeCodexDir（测试用）
  README.md / .gitignore
```

- AgentPal 根 module 名保持 `agentpal`，**现有 `agentpal/internal/...` 导入不动**。
- 新仓库 module 名 `github.com/cjb1234567/agentpal-central`，导入路径为 `github.com/cjb1234567/agentpal-central/internal/...`。
- 两仓库无 `go.work`、无 `replace`、无任何交叉引用。

## 新仓库自带的公共子集（最小复制）

新仓库只复制**编译与自测所需**的符号/文件；**不复制**客户端专用逻辑（`authprofile` 的 switch/ensure/current/import、`codex` 的 auth 合并、`config`、`login` 等）。

- `types/`：`CenterManifest`、`CenterSkill`、`CenterMCP`、`FileEntry`、`ScanError`、`ServerOptions`、`AuthProfile`、`RemoteAuthProfile`、`RemoteAuthProfileDetail`、`AdminAuthProfile`、`CasdoorUser`、`ReservedSettings`、`TemplateVariable`、`AdminStatus`、`BackupInfo`、`LoginStart`、`LoginStatus`。
- `authprofile/`：生产代码只需 `content.go`（`ValidateReservedName`、`ComputeContentHash`）、`archive.go`（`Pack`、`Unpack`）、`central.go`（`CentralProfile`）与 `AuthProfile` 类型（`server.go` 读取 `profile.json` 用）。测试还需 `NewStore`/`CreateAPIKey`（`backup_test` 构造归档），按需复制其最小读写实现。
- `codex/`：仅 `MCPEntry` + `ReadMCP`（新建裁剪文件，去掉 `UpsertMCP` 及其对 `toml_sections` 的依赖）。
- `security/`：`paths.go` 整体（`IsIgnoredName`、`ValidateSkillRelPath`、`SafeJoin`）。
- `constants/`：`AppName`、`AppVersion`、`DefaultPort`。
- `centralscan/`：`ScanSkills` 整体。
- `testutil/`：`MakeCodexDir`。

## AgentPal 仓库侧变更

- 删除 `internal/centralnode/` 与 `cmd/central-node/`。
- `internal/{types,authprofile,codex,security,constants,centralscan,centraltest}` 原样保留（客户端自身使用）。
- 客户端不再包含任何服务端代码；`app.go` 只依赖 `centralclient`。
- README 中关于「中心节点」的说明改为指向新仓库。

## 协议与格式契约（不共享的代价与保障）

两仓库各自维护、必须保持一致的两处约定：

1. **HTTP JSON 契约**：AgentPal 的 `centralclient`/`types` 与 `agentpal-central/internal/types` 之间。
2. **预留 profile 落盘格式**：`users/<sub>/<name>/{profile.json, auth.json, config-auth.toml}`（客户端 `authprofile` 写、服务端 `userProfileStore` 读写）。

保障措施：在两仓库各加一个**契约测试**，用同一份固定样例（golden JSON / golden 目录结构）断言字段名与结构一致；服务端侧保留 HTTP 集成测试。

## `e2e_test` 处置

`internal/centralnode/e2e_test.go` 同时使用服务端 `NewServer` 与客户端 `NewClient`，分离后无处安放。方案：移入新仓库，改写成**服务端侧 HTTP 集成测试**——用标准库 `net/http` 直接对 `server.Handler()` 发请求，不再 import `centralclient`。AgentPal 的 `centralclient` 保留自身 httptest 单测。

## 构建与测试

- 新仓库 `go.mod`：`module github.com/cjb1234567/agentpal-central`，`go 1.22.0`，依赖 `github.com/pelletier/go-toml/v2`。
- 验证命令：
  - AgentPal：`go build ./...`、`go test ./...`；`go list -deps ./...` 不含服务端包。
  - agentpal-central：`go build ./...`、`go test ./...`；`go list -m all` 不含 `github.com/cjb1234567/AgentPal`。
- 新仓库部署：`go build -o agentpal-central .` 产出独立二进制。

## 迁移步骤（概览）

1. 新建本地目录 `~/aiProjs/agentpal-central`，`go mod init github.com/cjb1234567/agentpal-central`。
2. 复制最小公共子集到 `internal/{types,codex,security,authprofile,constants,centralscan,testutil}`，导入路径改为 `github.com/cjb1234567/agentpal-central/internal/...`。
3. 复制 `internal/centralnode/*` 到 `internal/server/`（改包名为 `server`、改导入路径，`admin_page.html` 随之）。
4. 复制 `cmd/central-node/main.go` 到新仓库 `main.go`。
5. 改写 `e2e_test.go` 为标准库 HTTP 集成测试。
6. 补 `README.md`、`.gitignore`；跑 `go build ./... && go test ./...` 全绿。
7. `git init` → 初始提交 → `gh repo create cjb1234567/agentpal-central --private --source=. --remote=origin --push`。
8. 回到 AgentPal：删除 `internal/centralnode/`、`cmd/central-node/`，更新 README，跑 `go build ./... && go test ./...` 全绿，提交。

## 验证与验收标准

- agentpal-central 能独立 `go build`/`go test`，且 `go list -m all` 无 AgentPal。
- AgentPal `go list -deps ./...` 不含任何服务端包。
- 两仓库各自测试全绿。
- 契约测试通过（HTTP JSON + 落盘格式各一份 golden）。
- 中心节点对外行为与接口不变（原 `centralnode` 测试迁移后仍通过）。
- 新仓库已创建并推送，`gh repo view` 可见。

## 风险与权衡

- **重复代码漂移**：`types` 与落盘格式两边各一份，靠契约测试兜底；这是「不共享」的既定代价。
- **两仓库协作**：接口变更需两边同步改；靠契约测试与文档约束。
- **历史**：新仓库为全新初始提交，旧历史留在 AgentPal（明确接受）。
- **复制时的路径/包名遗漏**：迁移后用 `go build`/`go vet` 全量校验，避免遗漏导入。
