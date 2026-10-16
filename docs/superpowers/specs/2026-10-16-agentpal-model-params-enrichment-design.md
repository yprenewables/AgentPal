# AgentPal 模型参数补全与 models.json 手动编辑

## 目标

让 Models 页能够手动编辑 `models.json`，并在保存时按需从 OpenRouter 模型接口补全 Codex 需要的可选参数（推理档位、输入模态、上下文长度），接口不可用或模型未匹配时保留本地值并提示。

## 背景与现状

- `models.json` 由 `internal/modelmanagement` 读写，`Save` 会补齐 Codex 必需字段，并写入 `config.toml` 的 `model_catalog_json`。
- 现有模板只保证字段存在，不含模型专属能力；`supported_reasoning_levels`、`input_modalities`、`context_window` 往往缺失或不准确。
- 当前网关模型 id 多为不带 provider 前缀的短名（如 `gpt-6-sol`、`claude-opus-5`）。

## 数据源

- 接口：`GET https://openrouter.ai/api/v1/models`。
- 后端请求，超时 10 秒；会话内缓存 10 分钟，避免每次保存都拉取。
- 相关字段：`id`、`canonical_slug`、`context_length`、`architecture.input_modalities`、`reasoning.supported_efforts`、`reasoning.default_effort`。

## 匹配规则

1. 用模型 `slug` 与 OpenRouter 条目的 `id` / `canonical_slug` 的**最后一段**比较，不区分大小写；两侧都生成归一化候选后求交集，双向都能命中。
2. 归一化候选：
   - 原样小写；
   - 去掉前导 `~`；
   - 去掉结尾 `:batch`、`:free`、`:extended`、`:thinking`、`:online`、`:nitro`、`-latest`；
   - `-<数字>-<数字>` → `-<数字>.<数字>`（如 `claude-opus-5-5` → `claude-opus-5.5`，保留连字符）；
   - `:` → `-`（如 `qwen3.8:27b` → `qwen3.8-27b`）。
3. 打分与取舍：两侧最后一段完全相同记 3 分；仅归一化后相同记 2 分；命中带 `:batch` 等后缀的条目减 1，命中带 `~` 前缀的别名条目减 1。取最高分；最高分并列多个不同条目时视为未匹配，避免误匹配。
4. 未匹配的模型保留本地全部字段，并在结果中列出名称。
5. 不做跨命名体系的模糊匹配（例如 `nano-banana-2` 与 `google/gemini-nano-banana-2.1`、`gemma4:26b` 与 `google/gemma-4-26b-a4b-it` 视为未匹配）。

用当前网关 33 个模型实测：命中 21 个（其中 5 个依赖归一化），未匹配 12 个（非对话模型、OpenRouter 未收录、以及命名体系不同的模型）。

## 写入字段

仅在勾选 `Update params` 时写入，且只改以下字段，其他键一律保留：

- `input_modalities`：取 `architecture.input_modalities`，只保留 `text`、`image`、`audio`，丢弃 `file`、`video` 等；结果为空则写 `["text"]`。
- `context_window` 与 `max_context_window`：取 `context_length`；当命中的 OpenRouter provider 前缀为 `openai` 或 `anthropic` 时，两者统一写 `272000`。
- `supported_reasoning_levels`：取 `reasoning.supported_efforts`，映射为 `[{"effort": <档位>, "description": <标准描述>}]`；仅保留 Codex 支持的档位（`none`、`minimal`、`low`、`medium`、`high`、`xhigh`、`max`、`ultra`、`persistent`）；缺失或为空时写 `[]`。
- `default_reasoning_level`：取 `reasoning.default_effort`，仅当它是 Codex 支持的档位时写入；否则删除该键，交由 Codex 默认。

不写入任何输出相关字段（不设置 `output_modalities`、输出长度等）。

## 容错与降级

- 接口超时、非 2xx、JSON 非法：保留全部本地值，保存照常完成，返回 `enrichmentError` 供界面提示。
- 单个模型未匹配：保留该模型本地值，记入未匹配列表。
- 未勾选 `Update params`：完全不请求接口，行为与现状一致。

## 接口契约

`modelmanagement.SaveRequest` 增加 `updateParams`（布尔）。

`modelmanagement.State` 增加：

- `enrichmentError`：接口级失败原因，成功时为空；
- `unmatchedModels`：本次未匹配到的模型 slug 列表。

`SaveModels` 仍返回 `State`，界面据此提示匹配数量、未匹配名称或接口不可用。

## 界面

- Models 页工具栏增加 `Update params` 复选框，默认勾选，随草稿一起保存生效。
- 保存后提示：匹配 N 个、保留 M 个本地参数；未匹配时列出模型名；接口不可用时给出明确提醒且不阻断保存。
- 增加「编辑 JSON」切换：文本域直接编辑 `models.json` 的 models 数组，编辑内容纳入现有未保存变更提示；保存前校验 JSON 语法，非法时阻止保存并提示。
- 列表行对未匹配模型给出轻量标记。

## 架构

- 新增 `internal/modelcatalog`：负责请求、缓存、匹配与字段映射，纯函数便于测试，网络通过可注入的 `*http.Client`。
- `internal/modelmanagement.Service` 在 `Save` 中调用该包的补全逻辑，仅在 `updateParams` 为真时执行。
- 前端 `ModelsTab` 增加复选框与 JSON 编辑视图，复用现有 draft/dirty/save 流程。

## 测试

- Go：归一化匹配用例（含 `-5-5`→`5.5`、`~` 前缀、`-latest` 后缀、歧义回退）；272k 覆盖仅限 openai/anthropic；模态过滤；档位映射与非法档位剔除；`default_reasoning_level` 写入与删除；接口超时/非 2xx/坏 JSON 降级；未匹配列表；未勾选时不发起请求。
- 前端：构建通过；浏览器模拟验证复选框默认状态、保存提示文案、未匹配名称展示、JSON 编辑校验与保存。
- 全量 `go vet`、`go test -race ./...`、`git diff --check`。

## 非目标

- 不做模型能力的人工推断；接口没有的字段不臆造。
- 不修改 `~/.codex/models.json` 以外的 Codex 配置（`model_catalog_json` 注入沿用现有逻辑）。
- 不引入第三方依赖。
