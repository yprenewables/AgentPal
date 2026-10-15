# AgentPal 模型管理设计

## 目标

在 AgentPal 中增加独立的 `Models` Tab，用于管理当前 Codex 目录下的 `models.json`。页面默认展示本地配置，用户可基于当前 API Key Profile 的 `/models` 端点按需发现新模型，并通过开关、删除和统一保存维护 Codex 可见模型。

## 导航

客户端 Tab 顺序固定为：

1. `Authentication`
2. `Models`
3. `Configuration`

默认仍进入 `Authentication`。

## 认证边界

进入 `Models` Tab 时，后端读取当前实际 Codex 认证状态，不依赖前端缓存。

- 当前为 API Key Profile：允许读取、自动更新、编辑和保存模型。
- 当前为 ChatGPT 登录：不读取或展示本地模型，不允许自动更新、编辑或保存，并显示：`Model management is only available for API Key profiles. Switch to an API Key profile in Authentication.`
- 无法识别当前认证：显示不可用状态和具体错误，不读取或覆盖 `models.json`。

## 本地文件

模型文件位于当前 AgentPal 设置中的 Codex 目录，而不是写死用户主目录：

```text
<codexDir>/models.json
```

加载规则：

- 文件存在且合法：读取顶层 `models` 数组并显示。
- 文件不存在：显示空列表；首次保存时创建文件。
- 文件不是合法 JSON、顶层不是对象或 `models` 不是数组：显示错误，禁止覆盖原文件。
- 保留顶层除 `models` 以外的所有未知字段。
- 保留已有模型条目的全部未知字段及原始值。

## 页面结构

页面标题为 `Models`，包含：

- 当前 Provider 名称和 `base_url` 摘要。
- `Auto update` 按钮。
- `Save models` 按钮，仅在草稿有修改且本地文件可写时启用。
- 模型列表，每行展示 `display_name`、`slug`、可见性开关和删除按钮。
- 文件不存在或列表为空时的空状态。

可见性开关直接映射 Codex 现有字段：

- 开启：`visibility: "list"`
- 关闭：`visibility: "hide"`

删除按钮需要二次确认。确认后仅从页面草稿删除；用户点击 `Save models` 后才持久化。

## 自动更新

点击 `Auto update` 时：

1. 从当前 `<codexDir>/config.toml` 读取 `model_provider`。
2. 读取对应 `[model_providers.<name>]` 的 `base_url`。
3. 从当前 `<codexDir>/auth.json` 读取 API Key。
4. 将 `base_url` 去除末尾 `/` 后请求 `<base_url>/models`。
5. 添加 `Authorization: Bearer <API Key>` 请求头。
6. 请求超时使用固定短超时，首版设为 10 秒。
7. 接受标准 OpenAI 响应 `{"data":[{"id":"..."}]}`。

合并规则：

- 使用模型 `slug` 与远端 `id` 比较。
- 远端仍存在的已有模型原样保留，不覆盖任何字段或草稿可见性设置。
- 仅将远端存在、本地不存在的模型追加到草稿。
- Provider 不再返回的本地模型从草稿移除；远端返回空清单时清空草稿。
- 远端重复 ID 只添加一次。
- 自动更新只修改页面草稿，不立即写文件。
- 更新完成后显示新增和删除数量；没有变化时显示已是最新。

缺少 API Key、`model_provider` 或 `base_url` 时，禁用自动更新并给出具体原因。HTTP 非 2xx、超时或响应格式错误时保留当前草稿，不写文件。

## 新模型模板

新发现模型使用 AgentPal 内置的保守 Codex 模板。模板只声明通用能力，不根据 `/models` 未返回的信息推断模型专属能力。

每个新条目至少包含：

```json
{
  "slug": "<remote id>",
  "display_name": "<remote id>",
  "description": "Discovered from the current model provider.",
  "visibility": "list",
  "supported_in_api": true,
  "priority": 100,
  "context_window": 128000,
  "max_context_window": 128000,
  "supports_parallel_tool_calls": true,
  "input_modalities": ["text"],
  "shell_type": "shell_command"
}
```

模板保持集中定义和测试。已有模型不会套用模板，也不会被模板补字段。

## 草稿与保存

页面维护独立草稿：

- 自动更新、开关和删除只修改草稿。
- 草稿与最近一次加载或保存的内容不同时，页面标记为未保存。
- 有未保存修改时切换到其他 Tab，显示确认对话框；确认放弃后再切换，取消则停留在 Models。
- `Save models` 将草稿替换到原顶层对象的 `models` 字段。
- 使用同目录临时文件、`0600` 权限和原子重命名写入。
- 保存成功后重新读取文件，刷新基线并清除未保存状态。
- 写入失败时保留草稿，显示错误。

## 后端边界

新增独立模型管理模块，负责：

- 判断当前认证类型并提取 Provider 请求参数。
- 读取和验证 `models.json`。
- 请求并解析 `/models`。
- 按固定规则合并远端模型。
- 原子保存本地模型配置。

Wails 暴露最小接口：

- `GetModelManagementState()`：返回认证可用性、Provider 摘要、本地模型和原始顶层配置状态。
- `DiscoverModels()`：使用当前实际 API Key Profile 请求并返回远端模型 ID，不写文件。
- `SaveModels(request)`：重新校验当前认证和本地文件状态后保存完整模型草稿。

保存请求携带加载时文件内容哈希。若保存前文件被其他程序修改，拒绝覆盖并提示重新加载，避免覆盖 Codex 或人工修改。

## 测试与验收

### 2026-10-15 默认模型与列表交互补充（用户已确认）

- 模型行使用名称与操作两列；名称和 slug 单行显示，过长省略并通过 title 展示全文。
- 可见性使用紧凑滑动开关，保留无障碍名称，移除 Visible 文本。
- Models 顶部增加 Default model（本地 slug 候选与手动输入）、Reasoning effort 和独立 Save defaults。
- 默认设置维护独立草稿，与模型列表共同参与离开页面确认；分别保存到 config.toml 与 models.json。
- 默认设置修改 config.toml 根级 model、model_reasoning_effort。空值移除该字段，恢复 Codex 默认。
- effort 支持 none、minimal、low、medium、high、xhigh；未知已有值可显示，不静默改写。
- 使用 TOML AST 精确修改值区间，保留注释、多行字符串和其他配置；校验配置哈希、实际 API Key 认证，0600 同目录临时文件原子保存。
- Wails 新增 GetDefaultModelSettings、SaveDefaultModelSettings；非法配置或哈希冲突不覆盖文件，失败保留前端草稿。
- 构建成功后自动安装到 /Applications/AgentPal.app，并核验签名和可执行文件哈希。

后端测试覆盖：

- API Key、ChatGPT 和未知认证状态。
- 文件不存在、合法文件、无效 JSON、非法 `models` 字段。
- 未知顶层字段和模型字段保留。
- `/models` URL 拼接、Bearer 鉴权、10 秒超时、非 2xx 和非法响应。
- 自动更新只新增、不覆盖、不删除，并按 `slug` 去重。
- 新模型模板完整且稳定。
- 开关保存为 `list` / `hide`。
- 删除后保存从数组移除对应条目。
- 文件内容哈希冲突时拒绝覆盖。
- 临时文件加原子重命名写入，最终权限为 `0600`。

前端与浏览器验收覆盖：

- Tab 顺序为 Authentication、Models、Configuration。
- ChatGPT 登录显示不可用空状态且不泄露本地模型。
- API Key 登录默认展示本地模型。
- 自动更新结果只进入草稿。
- 开关、删除确认、保存和错误提示正确。
- 有未保存修改时切换 Tab 会提示。
- 保存后重新打开页面与磁盘内容一致。
