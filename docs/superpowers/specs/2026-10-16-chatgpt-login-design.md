# ChatGPT 登录切换

已确认：Authentication 页面当前使用 API Key 时提供“使用 ChatGPT 账号登录”入口。复用本机 Codex CLI 浏览器登录，在独立临时 CODEX_HOME 中授权；成功后保存本地 ChatGPT Profile 并激活，保留当前 API Key Profile。支持等待、取消、超时和失败提示。

登录期间不改动当前 Codex 文件。授权结果必须包含 ChatGPT tokens；切换使用内置 OpenAI Provider，不能继续使用 API Key 网关。应用失败恢复原认证及配置。退出客户端取消登录并清理临时目录。找不到 Codex CLI 时明确提示安装。

验证覆盖独立登录目录、成功切换及原档案保留、登录失败和取消不改当前文件、应用失败回滚、内置 OpenAI Provider 往返切换，以及前端构建和页面交互。
