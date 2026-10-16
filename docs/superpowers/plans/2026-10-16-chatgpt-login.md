# ChatGPT 登录入口实施计划

**Goal:** 从 API Key 安全切换到浏览器授权的 ChatGPT 本地档案。

**Architecture:** 独立登录服务执行 `codex login`，通过临时 CODEX_HOME 隔离当前文件。App 提供登录与取消方法，前端等待并复用 Profile 激活结果。

**Tech Stack:** Go、Wails、React、Codex CLI。

### Task 1：支持内置 OpenAI Provider
- [x] 在 `internal/codex/builtin_auth_test.go` 增加无 Provider 表的 OpenAI 提取、比较和合并测试。
- [x] 运行 `go test ./internal/codex` 确认新测试失败。
- [x] 调整 `internal/codex/auth_config.go`，允许内置 OpenAI 无表，并在应用时移除同名本地覆盖。
- [x] 运行相同测试确认通过。

### Task 2：隔离登录与安全应用
- [x] 新增 `internal/authprofile/chatgpt_login_test.go`，使用注入的 CLI 执行函数验证成功、失败、取消、回滚与 API Key 保留。
- [x] 运行 `go test ./internal/authprofile` 确认缺少实现。
- [x] 新增 `internal/authprofile/chatgpt_login.go`，执行隔离登录、校验 tokens、保留当前档案、保存新档案、应用及失败恢复。
- [x] 新增 `app_chatgpt.go` 提供 `LoginChatGPT`、`CancelChatGPTLogin`，防重复登录，关闭应用取消。

### Task 3：入口及验证
- [x] 更新 `frontend/src/wails.ts` 与 `frontend/src/App.tsx`，增加按钮、等待与取消对话框、成功刷新。
- [x] 更新 README 登录说明。
- [x] 执行 `go test ./...`、`go test -race ./internal/authprofile ./internal/codex`、`npm run build`。
- [x] 浏览器验证 API Key 入口、等待、取消及成功状态（模拟桌面桥接）；真实账号授权由用户在浏览器完成。
