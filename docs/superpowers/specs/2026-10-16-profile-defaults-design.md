# 默认模型绑定 Auth Profile

用户确认将默认模型及推理强度绑定到 Auth Profile。采用本地 `model-defaults.json` 存储 `model` 与 `effort`，不参与认证内容哈希。Models 保存默认值后立即写入当前档案；切换前保存当前设置，切换后恢复目标档案设置。目标无设置时清除根级 `model` 和 `model_reasoning_effort`，使用 Codex 默认值。

首次切换迁移当前值，即使没有 models.json 也必须迁移。沿用模型目录切换锁及失败回滚。Central 同步、重命名和相同凭据档案合并保留本地设置。MCP、Projects 及其他配置保持现有处理。

实施与验证：先添加 A/B 往返及保存即时持久化测试并确认失败；实现保存、恢复及目录替换携带；跑全量测试、竞态检查、go vet 和构建，重新安装本地客户端。
