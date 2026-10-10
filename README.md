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

切换档案会更新 `auth.json` 和 `model_provider`，并用档案中的 Provider 配置完整覆盖本机同名 Provider；其他 Provider、MCP、Projects 和本机设置保持不变。切换前会备份当前 `auth.json` 和 `config.toml`。

中心备份按设备名和档案 ID 存放。上传前请确认网络可信，因为第一版不加密、不登录、不做权限控制。

## 中心节点与预留 profile

中心节点服务端（含管理后台）已拆分为独立项目 **`agentpal-central`**（`https://github.com/cjb1234567/agentpal-central`）。本仓库（桌面客户端）只通过 HTTP 访问它，不包含任何服务端代码。

中心节点通过 Casdoor（默认 `https://id.yprenew.com`）做 OAuth 登录，OAuth 回调地址为 `{central_host}:{central_port}/callback`，必须在 Casdoor 应用里登记。服务端启动参数与管理后台说明见 `agentpal-central` 仓库 README。

管理员可以用中心节点的管理页 `/agentpal`（点击登录跳转 Casdoor）维护用户预留 profile，也可以直接放目录（格式如下）：

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

## 中心节点 API

中心节点对客户端暴露的 HTTP 接口（配置清单、认证备份、设备登录、预留 profile）与管理接口（`/v1/admin/*`）均由独立项目 `agentpal-central` 提供，完整列表见其 README。

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
