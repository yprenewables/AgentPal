# AgentPal 模型管理实施计划

**目标：** 按模型管理设计增加 Models Tab，安全维护当前设置目录的 models.json。

**架构：** `internal/modelmanagement` 独立负责实际认证检查、原始 JSON 字段保留、发现及原子保存；`app_models.go` 提供三个 Wails 接口；`ModelsTab.tsx` 维护独立草稿，父组件保护 Tab 切换。

**技术栈：** Go、go-toml/v2、net/http、React、TypeScript、Wails。

## 约束

- 请求固定 10 秒超时，API Key 只留在后端。
- JSON 使用 RawMessage 保留未知字段和数值精度；保存校验文件哈希。
- ChatGPT 和未知认证不读取 models.json；非法文件禁止覆盖。
- 更新、删除和开关只改草稿；0600 同目录临时文件原子保存。

## 实施步骤

- [ ] 后端：新增 `internal/modelmanagement/service_test.go`，覆盖认证、合法/非法/缺失文件、发现鉴权/格式/错误、模板合并、哈希冲突、字段保留、权限及失败不写入。先运行 `go test ./internal/modelmanagement` 确认缺失实现，再新增 `service.go` 并使测试通过。
- [ ] 接口：新增 `app_models.go`，每次从 appconfig 读取并展开 CodexDir；暴露 GetModelManagementState、DiscoverModels、SaveModels，增加 app 集成测试验证设置目录与认证重检。
- [ ] 前端：新增 ModelsTab 及合并帮助函数，接入类型与生成绑定。Models 位于 Authentication 与 Configuration 之间；草稿以 JSON 基线比较，删除/离开均确认，保存成功使用后端重读结果。
- [ ] 验收：`go test ./...`、`go test -race ./internal/modelmanagement`、`npm run build`；浏览器使用隔离的 Wails bridge fixture 验证草稿、错误、确认和不可用状态；构建桌面应用验证绑定。

本次沿用含既有未提交认证改动的工作区，按用户授权直接实施，完成后交付可审阅改动。
