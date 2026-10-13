# AgentPal 当前认证 Profile 自动导入设计

## 1. 目标

每次读取认证 profile 列表时，以本地 Codex 当前正在使用的认证为准。如果当前认证不在 profile 列表中，AgentPal 自动将其导入并选中，不再要求用户点击 `Import`。

自动生成的名称规则：

- API Key profile 使用当前 `model_provider` 作为名称。
- ChatGPT profile 使用当前 ChatGPT User ID 作为名称。
- 如果列表中已有同名本地 profile 但认证内容不同，直接更新该 profile，不创建重名记录。
- 如果当前 provider 已存在 Central profile，以 Central profile 为唯一条目和事实源，不再自动创建同 provider 的本地 profile。

## 2. 非目标

- 不改变 profile 的手动创建、编辑、删除和切换流程。
- 不改变 Central profile 的同步、缓存和权限模型。
- 不改变 Codex 的 MCP、Projects、模型参数和其他非认证配置。
- 不通过自动导入修改本地 Codex 当前认证文件。
- 不移除现有后端手动 import 接口，避免不必要的接口兼容性变化。

## 3. 触发与数据流

`ListAuthProfiles()` 每次执行时按以下顺序协调本地状态：

1. 读取 `~/.agentpal/auth-profiles` 中的 profile。
2. 检查 Codex 目录中的 `auth.json` 和 `config.toml`。
3. 将当前 `auth.json` 规范化，并提取当前 provider 的完整认证配置片段，计算内容哈希。
4. 如果当前 provider 已存在 Central profile，先清理严格命名的旧 `<provider> (Local)` 副本并重新读取列表。
5. 如果内容哈希与剩余 profile 完全一致，返回列表并将该项标记为 `Current`。
6. 如果当前认证未精确匹配任何 profile，但可以识别为有效 API Key 或 ChatGPT 认证，则自动导入；当前 provider 已存在 Central profile 时不导入本地副本。
7. 自动导入完成后重新读取 profile 列表并比较内容哈希。
8. 以精确匹配得到的 `CurrentProfileID` 标记当前项并返回。

自动导入完成后重新读取 Codex 当前认证和 profile 列表，不依赖写入前的内存快照。如果认证在协调过程中发生变化，则基于新快照再重试一次；仍不稳定时返回明确错误。

`GetCurrentAuthStatus()` 保持只读，不触发自动导入。这样自动写操作只发生在明确的 profile 列表加载流程中。

## 4. 认证类型与名称

### 4.1 API Key

当当前 Codex 配置能够读取到 API Key 且 `model_provider` 非空时：

- profile 类型为 `api_key`；
- profile 名称为 `model_provider`；
- 保存当前 API Key；
- 保存当前 provider 对应的认证配置片段，包括 `base_url` 等字段。

当前行为继续保持 API Key 优先：如果 `auth.json` 同时包含 API Key 和 ChatGPT token，则按 API Key profile 导入。API Key 可以来自 `auth.json` 或当前 `model_provider` 对应的 provider 配置表；不扫描无关 provider、MCP 或其他配置。新建与覆盖都保存同一份完整 provider 认证配置片段，不合成或丢弃 headers、`wire_api` 等字段。

### 4.2 ChatGPT

当前 Codex 未检测到 API Key，能从 token 中识别 ChatGPT User ID，且 `model_provider` 非空时：

- profile 类型为 `chatgpt`；
- profile 名称为 ChatGPT User ID；
- `auth.json` 按现有导入逻辑完整保存；
- `config-auth.toml` 只保存当前 provider 的认证配置片段。

### 4.3 无法命名或识别

以下情况不自动创建 profile：

- 没有可识别的 API Key 或 ChatGPT User ID；
- API Key 存在但 `model_provider` 为空；
- ChatGPT User ID 存在但 `model_provider` 为空；
- 当前 `model_provider` 对应的 provider 配置表不存在；
- 当前认证文件无法解析。

解析错误继续返回给调用方；认证信息不足则保持列表不变。

## 5. 同名覆盖

自动生成的名称已存在，但现有本地 profile 不匹配当前 Codex 认证时，更新同名 profile，而不是创建重复记录。名称按大小写精确比较。

如果当前 provider 已存在 `source=central_login` 的 Central profile，即使 Central 内容与当前 Codex 认证不同，也保留 Central profile 作为该 provider 的唯一列表条目，不自动导入本地副本。Central profile 只有在其内容与当前认证精确匹配时才标记为 `Current`；内容不同时保持非 Current，等待用户主动切换。

旧版本曾自动创建 `<provider> (Local)`。列表协调时执行一次收敛迁移：

- 仅删除名称严格等于 `<provider> (Local)`、`Provider` 相同且 `Source` 不是 `central_login` 的 profile。
- 同一 provider 的其他手工命名 profile 保留，不做推断式删除。
- 如果 `active.json` 指向被删除的旧副本，同时清除该标记。
- Central profile 的文件和元数据不因迁移改变。
- 迁移发生在精确内容匹配之前；迁移后重新读取列表，再判断 Central 是否与当前认证精确匹配。

更新规则：

- 保留现有 profile ID，避免引用和选中关系变化。
- 保留 `CreatedAt`。
- 更新 `UpdatedAt`。
- 将 profile 类型、provider 和认证内容更新为当前 Codex 认证。
- 手动本地 profile 保持本地来源语义。
- 不自动覆盖 `source=central_login` 的 Central 缓存 profile；同 provider 已存在 Central profile 时不创建本地副本。
- 只清理由旧自动导入规则创建的严格命名 `<provider> (Local)` 副本，不删除其他本地 profile。
- 如果存在多个同名非 Central profile，则返回歧义错误，不随机选择覆盖对象。

写入先在同一父目录创建隐藏临时 profile 目录并完成读取校验，再通过带备份的目录替换提交。提交前失败不修改原 profile；提交过程中失败则尝试恢复并校验原 profile，回滚失败时同时报告原始错误与回滚错误。提交成功后的备份清理失败不回滚有效的新 profile。

## 6. 组件边界

### 6.1 Store 层

新增“协调当前认证”的 store 操作，负责：

- 在进程内串行执行读取、精确内容匹配和写入，避免同一 AgentPal 进程内的并发列表请求重复创建；
- 检查当前认证内容是否已有精确匹配项；
- 判断认证类型；
- 生成 profile 名称；
- 创建新 profile 或原位更新同名本地 profile；
- 返回协调后的完整 profile 列表和当前状态。

该操作复用现有认证解析、配置片段提取和 profile 写入逻辑，不在 `InspectCurrent()` 中引入写入副作用。

### 6.2 App 层

`ListAuthProfiles()` 负责调用协调操作，并只使用协调操作返回的已复核列表和当前状态。

### 6.3 前端

删除 Authentication 页面中“当前认证未导入”提示条及两个 `Import` 按钮。没有 Central provider 时，后端确保可识别的当前认证存在并被选中；已有 Central provider 且内容不同时，列表只返回 Central 条目并保持非 Current。

前端不新增自动导入请求，不维护第二套协调逻辑。

## 7. 错误处理

- 当前认证文件 JSON/TOML 无法解析：列表请求返回解析错误，不写 profile。
- 自动创建失败：列表请求返回创建错误。
- 同名本地 profile 更新失败：保留原 profile，并返回更新错误。
- 当前 provider 已有 Central profile：保留 Central 缓存，不自动导入本地副本。
- 旧 `<provider> (Local)` 副本清理失败：返回错误，不继续写入其他 profile。
- 自动导入后仍无法匹配：返回错误，避免展示一个已导入但未选中的不一致状态。

## 8. 测试与验收

后端测试：

- 当前 API Key 不在列表时，以 provider 命名自动导入，并在返回列表中选中。
- 当前 ChatGPT 认证不在列表时，以 User ID 命名自动导入，并在返回列表中选中。
- 同名本地 profile 内容不同时，保留原 ID、覆盖内容、列表数量不增加并选中。
- 当前认证已有匹配项时，不改写 profile 文件和更新时间。
- 无有效认证时不创建 profile。
- API Key 与 ChatGPT token 同时存在时，按 API Key 导入。
- 当前 provider 已有 Central profile 时，Central 内容不变且不创建本地副本。
- 已存在旧 `<provider> (Local)` 副本时自动删除；如果 active 标记指向它则一并清除。
- 同 provider 的其他手工命名 profile 保持不变。
- Central 内容与当前认证不同时不标记为 Current；用户切换 Central 后才标记为 Current。
- 导入后重新检查仍无法匹配时返回错误。

前端验收：

- 页面不再显示手动 `Import` 提示和按钮。
- 首次打开 Authentication 页面时，没有 Central provider 的当前认证自动出现在列表中；已有 Central provider 时只显示 Central 条目。
- 非 Central provider 自动导入或更新后立即显示 `Current`；Central 条目仅在内容与当前认证精确匹配时显示 `Current`。

验证命令：

```bash
go test ./...
cd frontend && npm run build
git diff --check
```
