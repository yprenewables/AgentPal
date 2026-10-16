# Profile 独立模型目录

经用户确认：每个 Auth Profile 按 ID 保存自己的 models.json。Models 保存后写入当前 Profile；切换前保存当前工作文件，切换后恢复目标文件和 model_catalog_json 引用。目标没有模型设置时删除工作 models.json 及自定义目录引用，使用 Codex 默认目录。首次切换自动将旧版工作文件归入当前 Profile。

models.json 不参与认证内容哈希，不影响 Central 同步判断。更新或重命名同一 ID 的档案保留本地模型文件。ChatGPT 登录切换复用相同行为。默认模型和推理强度仍为原有全局配置。

认证、配置、工作模型文件和 active.json 作为一次切换处理，失败恢复原状态。应用内模型保存与切换使用同一锁，防止保存到错误账号。
