# AgentPal 发布约定

## macOS 安装包

- 用户要求打包或发布 macOS 版本时，默认交付 **DMG 拖拽安装包**，命名为 `AgentPal-<版本>-macos-<架构>.dmg`。
- 构建 `.app` 后，使用 `bash scripts/build-dmg.sh` 打包；已有应用可使用 `bash scripts/build-dmg.sh <应用路径> <输出目录>`。使用方法见 README 的“macOS 安装包”。
- DMG 必须包含 AgentPal.app、指向 `/Applications` 的快捷入口和中文安装说明，并保留脚本设置的 Finder 图标布局。
- macOS 的默认发布产物为 DMG。只有用户明确要求时，才额外提供 ZIP。
- 发布前验证 DMG 完整性、挂载内容、应用签名和版本信息，并生成、核验 `SHA256SUMS`。
- 发布说明必须引导用户：退出旧客户端，打开 DMG，将 AgentPal 拖入 Applications，更新时选择“替换”，从“应用程序”启动，安装后推出安装磁盘。

## YPShare 发布

- 当前共享目录为 `/Users/yp-mac-02/SynologyDrive/YPShare/04_AI_Tools_Docs_工具_文档/AgentPal`；发布前确认该路径及目标版本。
- 用户要求发布到 YPShare 时，将安装包、`SHA256SUMS` 和发布说明放入对应的版本目录，并更新 `latest` 入口及共享目录的 README。
- 从 `latest` 入口执行 `shasum -a 256 -c SHA256SUMS`，确认用户下载到的发布文件校验通过。
