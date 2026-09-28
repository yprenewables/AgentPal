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
```

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
- 公网部署、HTTPS、账号、权限和多租户
- 认证备份加密
- Skills/MCP 静默自动应用
- 中心配置版本历史和回滚界面
- 点对点 Share/Sync 工作流
