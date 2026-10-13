# AgentPal Central Profile 名称唯一归属设计

## 1. 目标

Central 提供的 profile 永久占用其原始名称。本地 profile 不允许与 Central profile 同名。

需要覆盖两类场景：

- Central profile 首次同步到本地时，处理已有的同名本地 profile；
- 打开 AgentPal 读取 profile 列表时，修复旧版本遗留的 Central 与本地同名状态。

## 2. 处理规则

对每个 `source=central_login` 的 Central profile，查找名称完全相同的非 Central profile：

- 内容哈希相同：只保留 Central profile。如果当前选择指向重复的本地 profile，则将当前选择改为 Central profile，然后删除重复本地 profile。
- 内容哈希不同：保留本地 profile 的 ID、内容、来源和创建时间，只把名称原位改为 `<Central 名称> (Local)`；Central profile 保留原名称。
- `<Central 名称> (Local)` 已被其他 profile 占用：返回明确冲突错误，不覆盖、删除或随机改名。
- 同一 Central 名称存在多个同名本地 profile：返回明确歧义错误，不随机选择。

名称比较区分大小写。Central profile 的服务端保留名称是名称归属的唯一依据。

## 3. Central 首次同步

`SyncCentral` 在创建新的 Central 缓存前检查同名本地 profile：

1. 校验 Central 内容并计算内容哈希。
2. 如果不存在同名本地 profile，按现有流程创建 Central 缓存。
3. 如果存在一个内容相同的同名本地 profile，将该 profile 原位转换为 Central 缓存：保留 ID 和 `CreatedAt`，写入 Central 元数据及内容。现有 `active.json` 无需改变，当前选择自然指向转换后的 Central profile。
4. 如果存在一个内容不同的同名本地 profile，先以事务方式将其原位重命名为 `<名称> (Local)`，再创建 Central 缓存。
5. 如果重命名或 Central 创建失败，恢复本地 profile 原名称和原内容，避免只完成一半迁移。

这样首次导入不会短暂产生两个同名 profile，也不会丢失当前选择。

## 4. 启动修复

`ListAuthProfiles()` 调用的 `ReconcileCurrent` 在自动导入当前 Codex 凭据之前修复遗留重名：

1. 读取全部 profile 和 `active.json`。
2. 对 Central 与同名本地 profile 执行第 2 节规则。
3. 修复后重新读取列表，再对当前 Codex 凭据做精确内容匹配。
4. 如果相同内容的本地重复项被删除且原来处于选中状态，把 `active.json` 改为 Central profile ID。
5. 如果不同内容的本地项被改名且当前 Codex 正在使用其内容，保持其 ID 不变，因此当前选择仍指向该本地 profile。

当前凭据已经与 Central profile 内容一致时，直接匹配并选中 Central，不创建 `<名称> (Local)`。当前凭据确实与 Central 不同时，允许按现有自动导入规则保存为 `<名称> (Local)`。

## 5. 文件与事务

- profile 重命名只更新对应目录内的 `profile.json`，不修改 `auth.json` 和 `config-auth.toml`。
- 写入先在同一父目录创建临时目录并校验，再替换目标目录。
- 内容相同的重复本地 profile 只有在 Central profile 已可正常读取后才删除。
- 删除重复项前先更新 `active.json`；更新失败时不删除本地项。
- 各协调入口继续使用现有进程内互斥锁，避免同步与列表修复并发操作同一 profile。

## 6. 非目标

- 不改变 Central API、鉴权和服务端数据。
- 不改变前端交互和展示文案。
- 不自动生成 `(Local 2)` 等名称。
- 不覆盖已占用的 `<名称> (Local)`。
- 不根据 provider 猜测名称冲突，只处理展示名称完全相同的 profile。

## 7. 测试与验收

- Central 首次同步遇到同名且同内容的本地 profile：列表最终只有一个 Central profile，保留本地 ID，当前选择指向该 Central profile。
- Central 首次同步遇到同名但内容不同的本地 profile：本地项原位改名为 `<名称> (Local)`，Central 使用原名称，两者内容不变。
- 打开 AgentPal 遇到历史同名且同内容的两项：删除本地重复项，并把当前选择从本地 ID 改为 Central ID。
- 打开 AgentPal 遇到历史同名但内容不同的两项：本地项原位改名，当前选择仍按实际内容匹配。
- `<名称> (Local)` 已占用或存在多个同名本地项：返回错误且所有 profile 文件保持不变。
- 激活 Central 后重新加载列表：不创建任何同内容本地副本，Central 保持 Current。
- 真实本地凭据与 Central 不同：仍可保存为 `<名称> (Local)`。

验证命令：

```bash
go test ./internal/authprofile -count=1
go test ./... -count=1
cd frontend && npm run build
git diff --check
```
