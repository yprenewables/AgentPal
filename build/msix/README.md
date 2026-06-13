# AgentPal MSIX 打包与签名

MSIX 打包和签名需要在 Windows 上执行，因为依赖 Windows SDK 的 `makeappx.exe` 和 `signtool.exe`。

## 准备

1. 安装 Windows SDK。
2. 确认已生成 Windows x64 程序：`build\bin\AgentPal-windows-x64.exe`。
3. 从 Microsoft Partner Center 获取应用包身份信息：
   - Package/Identity Name
   - Publisher，例如 `CN=...`
   - Publisher Display Name
4. 如果需要本地签名，准备 `.pfx` 代码签名证书及密码。

## 生成 MSIX

在项目根目录运行：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build-msix.ps1 `
  -IdentityName "你的PackageIdentityName" `
  -Publisher "CN=你的Publisher" `
  -PublisherDisplayName "你的发布者显示名" `
  -Version "0.1.0.0"
```

输出文件：

```text
build\bin\AgentPal-windows-x64.msix
```

## 生成并签名 MSIX

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build-msix.ps1 `
  -IdentityName "你的PackageIdentityName" `
  -Publisher "CN=你的Publisher" `
  -PublisherDisplayName "你的发布者显示名" `
  -Version "0.1.0.0" `
  -CertificatePath "C:\path\to\certificate.pfx" `
  -CertificatePassword "证书密码"
```

## Device Guard / WDAC 注意事项

如果目标 Windows 电脑启用了组织的 Device Guard 或 WDAC 策略，仅生成 MSIX 不一定足够。组织策略通常还要求：

- 包使用组织信任的代码签名证书签名。
- 证书发布者或包身份已加入组织白名单。
- 或由 Microsoft Store 分发并由组织策略允许。
