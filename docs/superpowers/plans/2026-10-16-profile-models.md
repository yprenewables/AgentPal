# Profile 独立模型目录实施计划

1. 在 internal/authprofile/profile_models_test.go 编写往返切换、无模型档案、首次迁移、配置失败回滚、Central 替换保留测试；在 app_models_test.go 验证 Models 保存即写入当前档案。运行新测试确认缺少实现。
2. 新增 internal/authprofile/profile_models.go 管理当前档案模型快照和文件恢复；internal/codex/auth_config.go 增加模型目录引用更新函数。切换统一持有 reconcileMu，先保存当前模型，再事务化应用认证、Provider、模型及 active.json。
3. app_models.go 在锁定的当前 Profile 下保存模型并持久化副本。ChatGPT 登录使用同一内部切换方法，Central 目录替换携带模型文件。
4. 更新 README；执行 go test ./...、go test -race ./...、go vet ./...、前端构建和 Wails 打包。替换本地应用并验证签名、哈希与启动。
