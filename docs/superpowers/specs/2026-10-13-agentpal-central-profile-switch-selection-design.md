# AgentPal Central Profile 切换选择设计

## 1. 目标

用户确认 Provider 冲突并切换到 Central profile 后，AgentPal 直接将所选 Central profile 标记为 `Current`，不得在随后刷新列表时自动创建同 provider 的 `<名称> (Local)` profile。

切换仍按现有规则更新 Codex 的 `auth.json`、`model_provider` 和对应 Provider 配置，同时保留其他 Provider、MCP servers、projects 和 settings。

## 2. 根因

Central profile 的缓存配置与实际写入 Codex 的配置可能存在可预期的规范化差异。例如 Memory API Key 在激活时会向 Provider 配置补齐 `experimental_bearer_token`。

激活完成后，Authentication 页面立即重新读取 profile 列表。`ReconcileCurrent` 使用原始内容哈希匹配当前 Codex 配置；规范化差异导致哈希不相同，于是当前配置被误判为新的本地认证，并创建 `<名称> (Local)` profile。

## 3. 选择规则

`active.json` 继续记录最后一次由 AgentPal 成功激活的 profile ID。

协调当前认证时按以下顺序判断：

1. 先执行现有的精确内容哈希匹配。
2. 如果精确匹配失败，读取 `active.json`。
3. 只有当该 ID 指向 `source=central_login` 的 Central profile 时，才对缓存 Profile 配置执行与激活流程一致的规范化。
4. 再从规范化配置提取认证片段并计算内容哈希，与当前 Codex 认证比较。
5. 内容一致时，直接将该 Central profile 标记为 `Current`，停止自动导入。
6. 内容不一致时，视为用户在 AgentPal 外修改了认证，继续执行现有本地认证协调规则。

该规则不会仅凭 provider 名称认定 Central profile，避免把外部手工修改的同 provider 凭据错误归属于 Central。

## 4. 本地 Profile 保留策略

- 切换 Central profile 不删除、重命名或覆盖已有的其他本地 profile。
- 列表刷新不得因 Central 激活产生新的 `<名称> (Local)` profile。
- 已有手工本地 profile 即使 provider 与 Central 相同，也原样保留。
- 只有用户在 AgentPal 外修改当前认证，且修改后的内容不再匹配已激活 Central profile 时，才继续使用现有自动导入规则。

## 5. 数据与错误处理

- Central 缓存的 `auth.json` 和 `config-auth.toml` 保持服务端同步副本语义，不因本地激活规范化而改写。
- Provider 冲突确认弹窗和 `allowProviderConflict` 行为保持不变。
- `auth.json`、`config.toml` 和 `active.json` 仍由现有激活事务写入。
- Central 缓存读取、配置规范化或哈希计算失败时返回错误，不创建本地 profile。
- 非 Central 的 `active.json` 不参与宽松匹配，保持现有精确匹配行为。

## 6. 非目标

- 不改变前端弹窗文案、按钮和页面布局。
- 不按 provider 删除已有本地 profile。
- 不把规范化后的 Codex 配置反向写入 Central 缓存。
- 不改变 Central API、登录、同步和服务端数据。
- 不改变用户在 AgentPal 外手工切换认证后的自动导入行为。

## 7. 测试与验收

- Central profile 与本地同 provider 配置发生冲突时，确认切换后重新读取列表，只有所选 Central profile 为 `Current`，不产生 `(Local)` profile。
- Central Provider 缓存缺少激活时自动补齐的字段时，切换后仍能规范化匹配 Central。
- 切换前已存在同 provider、不同名称的手工本地 profile 时，该 profile 保持不变，Central 成为 `Current`，列表数量不增加。
- 当前 Codex 凭据在激活后被外部修改为不同内容时，不错误标记 Central 为 `Current`。
- 普通本地 profile 的切换与精确匹配行为保持不变。

验证命令：

```bash
go test ./internal/authprofile -count=1
go test ./... -count=1
cd frontend && npm run build
git diff --check
```
