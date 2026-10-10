# AgentPal 统一认证 Profile 列表设计

## 1. 目标

将 Local profiles 与 Central profiles 合并为一个统一列表。Central profile 登录后同步到本地，保存完整认证凭证和配置，并通过 `source=central_login` 标记来源。用户退出 Central 登录后，已缓存的 Central profile 仍显示在统一列表中，并允许继续切换使用。

本次改造同时统一 profile 行的视觉和交互：类型使用图标表示，profile name 作为标题，副标题显示关键配置，选择控件统一纵向对齐，选中与未选中状态有明确区分。

## 2. 非目标

- 不改变 Central 服务端 profile 的权限模型和 HTTP API 合同。
- 不把退出登录理解为删除本地 Central 缓存。
- 不允许本地删除操作影响 Central 服务端 profile。
- 不改变 MCP、Projects、Codex 配置合并和备份机制。

## 3. 数据流

### 3.1 本地 profile

本地 profile 继续使用 `~/.agentpal/auth-profiles/<id>/`，包含：

- `profile.json`
- `auth.json`
- `config-auth.toml`

Central 缓存 profile 使用现有字段：

- `source: "central_login"`
- `reservedName: <server profile name>`
- `contentHash: <auth.json + config-auth.toml hash>`

凭证和完整配置都保存在本地，沿用现有文件权限与 profile store 写入逻辑。

### 3.2 登录后的同步

以下场景触发 Central profile 同步：

1. Authentication 页面首次读取登录状态且已登录。
2. Central 登录成功后。
3. 用户点击 Central 刷新按钮。

同步流程：

1. 读取当前 Central login session。
2. 请求 `GET /v1/auth-profiles` 获取当前用户的 profile 元数据。
3. 对每个远端 profile 请求 `GET /v1/auth-profiles/<name>` 获取完整详情。
4. 本地没有同名 Central profile 时创建缓存。
5. 本地已有同名 Central profile 时比较 content hash；服务端版本变化时更新本地完整副本。
6. 返回同步状态，用于行内显示“已同步 / 服务端有更新 / 更新失败”等状态。

同步只更新 `source=central_login` 的本地副本，不覆盖同名的手动本地 profile。

### 3.3 未登录状态

未登录时不请求 Central HTTP 接口。统一列表直接读取本地 profile store，因此：

- 手动 Local profiles 正常显示。
- 已缓存的 Central profiles 正常显示，并带 Central 来源标记。
- Central profiles 仍允许选择和切换。
- Central 刷新、从服务端更新等动作不可用，重新登录后恢复。

## 4. 后端接口

保留并恢复以本地缓存为基础的 profile 操作：

- `ListAuthProfiles()`：返回本地手动 profile 和已缓存的 Central profile。
- `SyncCentralAuthProfiles()`：登录后从服务端同步 Central profile 到本地。
- `ApplyCentralAuthProfile(name)`：登录后更新指定 Central 缓存，或由行内更新操作调用。
- `ActivateAuthProfile(id)` / `PreviewAuthProfile(id)`：Central 缓存与手动 profile 统一按本地 profile ID 操作。

不再让 Central profile 激活依赖实时详情接口。用户退出登录后，激活操作仍可使用本地缓存中的 `auth.json` 和 `config-auth.toml`。

Central 缓存删除只删除本地目录，不调用服务端删除 API。激活中的 profile 仍禁止删除。

## 5. 前端列表

删除独立的 Central profiles 区域，只保留一个 `Local profiles` 区域。列表数据来自 `ListAuthProfiles()`，登录后的同步结果只负责更新该列表和行状态。

每行布局固定为：

1. 左侧类型图标：API Key 使用钥匙图标，ChatGPT 使用对话/身份图标。
2. 主内容：`profile.name` 作为标题。
3. 来源标记：Central profile 显示云图标和 `Central` 标签；手动 profile 不显示该标签。
4. 副标题：
   - API Key：provider、掩码后的 key、base URL。
   - ChatGPT：provider、user ID、过期时间。
5. 右侧操作区：当前状态、统一的选择按钮、更多菜单。

所有行使用相同的固定操作区宽度、`align-items: center` 和按钮尺寸，保证选择按钮纵向对齐。

### 5.1 选中状态

- 行显示高亮边框和浅色背景。
- 右侧显示 `Current` 状态徽章。
- 选择按钮显示选中对勾并禁用。

### 5.2 未选中状态

- 行使用普通边框和背景。
- 右侧显示可点击的统一选择按钮。
- 更多菜单可执行允许的编辑、删除和 Central 更新动作。

### 5.3 Central 行动作

- 已登录且服务端版本不同：显示“从服务端更新”按钮。
- 已登录且同步失败：显示失败状态和错误原因。
- 未登录：隐藏远端刷新/更新动作，但保留选择、删除本地缓存等本地操作。

## 6. 错误处理

- 未登录调用同步接口返回 `not logged in`，前端不把它当作 Central profile 列表错误。
- Central 服务不可达时保留现有本地 profile 列表，并在页面显示刷新失败提示。
- 单个 Central profile 详情获取失败时保留其他成功同步的 profile，并在该行显示失败状态。
- Central profile 文件写入失败时不替换旧副本，返回错误状态。
- Central profile 激活失败时沿用现有配置回滚和 backup 机制。

## 7. 测试与验收

后端测试：

- 登录后同步新增 Central profile，并验证完整 `auth.json` 和 `config-auth.toml` 已保存。
- 服务端 content hash 变化后同步更新本地副本。
- 未登录时本地缓存 Central profile 仍可通过 `ListAuthProfiles()` 返回。
- 未登录时可以通过统一激活流程切换缓存 Central profile。
- 删除缓存 Central profile 不影响服务端数据。

前端测试：

- Central 区不再单独渲染。
- Central profile 与手动 profile 使用同一 profile 行结构。
- Central 标签、类型图标、标题、副标题和操作区存在。
- 选中与未选中状态分别渲染，选择按钮位置一致。
- 退出登录后已缓存 Central profile 仍显示并可选择。

构建验证：

```bash
go test ./...
cd frontend && npm run build
git diff --check
```
