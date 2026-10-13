# AgentPal Provider 冲突预览规范化设计

## 1. 目标

Provider 冲突预览必须比较“切换 Profile 时实际会写入本地的 Provider 配置”，而不是未经激活规范化的缓存原文。

用户首次从不同的本地 `memory` Provider 切换到 Central `memory` 时仍显示 `Replace local provider?`。用户确认并成功应用后，只要本地 `memory` Provider 未被外部修改，即使先切到 `newapi` 再切回 `memory`，也不再重复提示。

## 2. 根因

当前预览和实际激活使用了不同的 Profile Provider：

- `Preview` 直接使用 Profile 缓存的 `config-auth.toml` 调用 `CompareProviderConfig`；
- `Activate` 在冲突检查之后才读取 `auth.json`，并调用 `EnsureProviderBearerTokenInFragment`；
- Memory API Key 以 `sk-mem-` 开头时，激活流程会补入或更新 `experimental_bearer_token`；
- 实际本地 Provider 因此比缓存原文多一个激活必需字段；下一次预览仍拿缓存原文比较，导致永久出现伪冲突。

## 3. 统一比较输入

本地缓存 Profile 的预览与激活共享以下流程：

1. 读取 `profile.json`、`config-auth.toml` 和 `auth.json`。
2. 验证 `auth.json` 是合法 JSON。
3. 调用 `EnsureProviderBearerTokenInFragment(config, auth)` 生成规范化 Provider 片段。
4. 使用规范化片段调用 `CompareProviderConfig`。
5. 如果用户确认切换，直接使用同一个规范化片段执行 `MergeAuthConfig`。

远程 Profile 激活也先用传入的 `auth` 规范化 `fragment`，再进行冲突比较和写入，确保所有入口的预览结果与实际应用结果一致。

## 4. 冲突语义

- 本地不存在目标 Provider：显示添加 Provider 的确认提示。
- 本地 Provider 与规范化后的 Profile Provider 不同：显示替换 Provider 的确认提示。
- 本地 Provider 与规范化后的 Profile Provider 相同：不显示提示，直接切换 `auth.json` 和 `model_provider`。
- 本地 `experimental_bearer_token` 缺失或值不同：属于真实 Provider 差异，继续提示。
- 非 `sk-mem-` API Key 不增加 bearer token，保持现有比较行为。

比较不忽略字段，也不因 Profile 来源是 Central 而跳过冲突检查。

## 5. 数据与错误处理

- 不修改缓存 Profile 的 `auth.json`、`config-auth.toml` 或元数据。
- 不把规范化后的 Provider 反向写入 Central 缓存。
- `auth.json` 无法读取或解析时，预览和激活均返回错误，不显示错误的无冲突结论。
- Provider 规范化失败时不修改 Codex 文件。
- 保留现有备份、写入失败回滚和 `active.json` 更新流程。
- 不改变确认弹窗文案、按钮和布局。

## 6. 测试与验收

- Profile 缓存缺少 `experimental_bearer_token`，但本地 Provider 已包含由同一 Profile 凭据派生的 token：预览无冲突。
- 本地 Provider 缺少 bearer token：预览有冲突。
- 本地 Provider 使用不同 bearer token：预览有冲突。
- 从不同配置首次切换到 Central `memory`：先报告冲突；确认后成功切换。
- 再切换到 `newapi`，然后预览 Central `memory`：不再报告冲突。
- 非 Memory Profile 的既有冲突检测测试继续通过。
- 远程 Profile 预览与写入使用同一个规范化片段。

验证命令：

```bash
go test ./internal/authprofile -count=1
go test ./... -count=1
cd frontend && npm run build
git diff --check
/Users/yp-mac-02/go/bin/wails build -clean
```

构建成功后覆盖 `/Applications/AgentPal.app` 并验证安装二进制与构建二进制哈希一致、应用进程正常启动。
