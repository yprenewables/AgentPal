# AgentPal

AgentPal 是 Codex 的中心化配置客户端。它从一个 AgentPal Server 拉取 Skills 和 MCP 配置，先展示差异，再由用户确认应用；本机的 ChatGPT 登录和 API Key 通过认证档案管理和切换。

第一版只支持 Codex、可信局域网 HTTP 和默认端口 `28888`。认证备份为明文且无账号鉴权，只适用于可信内网。

## 运行模式

同一个桌面程序通过 `~/.agentpal/config.json` 选择角色。

Client 示例：

```json
{
  "mode": "client",
  "codex_dir": "~/.codex",
  "central_host": "10.10.20.11",
  "central_port": 28888,
  "device_name": "yp-mac-02"
}
```

Server 示例：

```json
{
  "mode": "server",
  "codex_dir": "~/.codex",
  "port": 28888,
  "backup_dir": "~/.agentpal/server/auth-backups",
  "device_name": "central-node"
}
```

Server 直接读取配置的 Codex 目录。每次清单请求都会重新扫描，因此目录变化会在客户端刷新后体现，不需要手动发布快照。

## 客户端

默认进入 `Configuration` 标签并连接中心节点：

- `Configuration`：比较 Skills 和 `[mcp_servers.*]`，预览新增、修改、本地独有和一致项，确认后应用。
- `Authentication`：导入当前 Codex 登录、创建 API Key 档案、查看和切换本地档案。
- `Backups`：按设备查看中心认证备份，上传、恢复或删除均为明确操作。

本地独有的 Skill/MCP 默认保留。下载内容会先校验 SHA-256，应用前创建备份，失败时恢复原配置。恢复中心认证备份只创建本地档案，不会自动激活。

## 认证档案

档案存放在：

```text
~/.agentpal/auth-profiles/<profile-id>/
  profile.json
  auth.json
  config-auth.toml
```

切换档案只更新 `auth.json` 和模型/Provider 相关配置，不覆盖 MCP、Projects 或其他本机配置。切换前会备份当前 `auth.json` 和 `config.toml`。

中心备份按设备名和档案 ID 存放。上传前请确认网络可信，因为第一版不加密、不登录、不做权限控制。

## 中心登录与预留 profile

Central Host 通过 Casdoor（默认 `https://id.yprenew.com`）做 OAuth 登录，OAuth 回调地址为 `{central_host}:{central_port}/callback`，必须在 Casdoor 应用里登记。

服务端启动参数：

```text
-casdoor-url         默认 https://id.yprenew.com
-casdoor-client-id   Casdoor 应用 client ID（必填，否则登录接口返回 503）
-casdoor-scopes      默认 "openid profile"
-casdoor-admin-claim 判定管理员的 Casdoor token claim，默认 isAdmin
-casdoor-organization 列出 Casdoor 用户时使用的组织，默认 built-in
-admin-subs          逗号分隔、始终视为管理员的 Casdoor sub 列表（可选）
-oauth-redirect-url  Casdoor 中登记的 Redirect URI
-user-store-dir      默认 ~/.agentpal/server/users
```

管理员可以用管理页 `/agentpal`（点击登录跳转 Casdoor）维护用户预留 profile，也可以直接放目录（格式如下）：

管理页的「系统配置」页统一设置 provider 名称、base_url、wire_api、env_key，用于生成所有用户的预留 profile；「Auth Profiles」页直接列出所有用户的预留 profile（用户 sub 为 Casdoor 用户 id），右上角「新增 / 覆盖」按钮弹出对话框，在其中从 Casdoor 用户下拉列表选择关联用户并录入 API Key（类型固定 `api_key`），保存即按系统配置生成/覆盖该用户的预留 profile。Casdoor 不可用时回退为手动填写 sub。系统配置留空时为未配置的默认值：

```json
{
  "provider": "memory",
  "baseURL": "https://memory.yprenewables.com/codex/default",
  "wireAPI": "responses",
  "envKey": "OPENAI_API_KEY"
}
```

生成的目录格式：

```text
~/.agentpal/server/users/<sub>/auth-profiles/<name>/
  profile.json       {"name":"<name>","type":"api_key","provider":"<name>"}
  auth.json          {"OPENAI_API_KEY":"..."}
  config-auth.toml   model_provider = "<name>" 且 [model_providers.<name>]
```

客户端登录后按预留名称同步：本地无同名则新增；一致则标记已同步；不一致只提示，需用户确认后才覆盖本地 profile。同步不会激活 profile，也不会修改 `~/.codex`。

> 安全提示：Central Host 只解析 JWT 的 `sub`，不校验签名。任何能访问 Central Host 的客户端都能伪造 `sub` 读取对应用户的 profile。该能力仅适用于可信内网。

## Server API

```text
GET  /health
GET  /v1/config/manifest
GET  /v1/config/files/skills/<relative-path>
GET  /v1/config/mcp/<name>

GET    /v1/auth-backups
POST   /v1/auth-backups/<device>/<profile>?overwrite=true
GET    /v1/auth-backups/<device>/<profile>
DELETE /v1/auth-backups/<device>/<profile>

POST /v1/auth/login/start
GET  /v1/auth/login/status/<loginId>?pollToken=<pollToken>
GET  /callback
GET  /v1/auth-profiles
GET  /v1/auth-profiles/<name>

GET    /v1/admin/users
GET    /v1/admin/me
GET    /v1/admin/settings
PUT    /v1/admin/settings
GET    /v1/admin/profiles
GET    /v1/admin/casdoor-users
GET    /v1/admin/user-profiles/<sub>
POST   /v1/admin/user-profiles/<sub>
GET    /v1/admin/user-profiles/<sub>/<name>
POST   /v1/admin/user-profiles/<sub>/<name>
DELETE /v1/admin/user-profiles/<sub>/<name>
```

管理页在 `/agentpal`，使用与客户端相同的 Casdoor 登录流程：点击登录会跳转 Casdoor，回调后自动进入。只有管理员账号可以进入。`/v1/admin/*` 接受两类管理员令牌：带管理员 claim 的 Casdoor 令牌（默认 claim `isAdmin`，可用 `-casdoor-admin-claim` 修改），或 Portal 管理员令牌（`-admin-jwt-public-key`）。两者都未配置时返回 `503`；已登录但非管理员返回 `403`。

Server 不在配置清单中发布 `auth.json`。所有路径都会进行安全校验，拒绝绝对路径和路径穿越。

## 本地数据

```text
~/.agentpal/config.json
~/.agentpal/auth-profiles/
~/.agentpal/backups/
~/.agentpal/server/auth-backups/
```

## 非目标

- OpenCode 或其他 Agent 工具
- 公网部署、HTTPS、多租户
- 认证备份加密
- Skills/MCP 静默自动应用
- 中心配置版本历史和回滚界面
- 点对点 Share/Sync 工作流
