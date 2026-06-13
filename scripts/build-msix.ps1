param(
  [Parameter(Mandatory=$true)]
  [string]$IdentityName,

  [Parameter(Mandatory=$true)]
  [string]$Publisher,

  [Parameter(Mandatory=$true)]
  [string]$PublisherDisplayName,

  [string]$Version = "0.1.0.0",

  [string]$ExePath = "build\bin\AgentPal-windows-x64.exe",

  [string]$OutputPath = "build\bin\AgentPal-windows-x64.msix",

  [string]$CertificatePath = "",

  [string]$CertificatePassword = ""
)

$ErrorActionPreference = "Stop"

function Find-WindowsSdkTool {
  param([string]$ToolName)

  $roots = @(
    "${env:ProgramFiles(x86)}\Windows Kits\10\bin",
    "${env:ProgramFiles}\Windows Kits\10\bin"
  )

  foreach ($root in $roots) {
    if (-not (Test-Path $root)) { continue }
    $match = Get-ChildItem -Path $root -Recurse -Filter $ToolName -ErrorAction SilentlyContinue |
      Where-Object { $_.FullName -match "\\x64\\$ToolName$" } |
      Sort-Object FullName -Descending |
      Select-Object -First 1
    if ($match) { return $match.FullName }
  }

  $cmd = Get-Command $ToolName -ErrorAction SilentlyContinue
  if ($cmd) { return $cmd.Source }

  throw "$ToolName not found. Install Windows SDK first."
}

function New-LogoPng {
  param(
    [string]$Path,
    [int]$Size
  )

  Add-Type -AssemblyName System.Drawing
  $bitmap = New-Object System.Drawing.Bitmap $Size, $Size
  $graphics = [System.Drawing.Graphics]::FromImage($bitmap)
  $graphics.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
  $graphics.Clear([System.Drawing.ColorTranslator]::FromHtml("#172033"))

  $fontSize = [Math]::Max(12, [int]($Size * 0.36))
  $font = New-Object System.Drawing.Font "Segoe UI", $fontSize, ([System.Drawing.FontStyle]::Bold), ([System.Drawing.GraphicsUnit]::Pixel)
  $brush = New-Object System.Drawing.SolidBrush ([System.Drawing.ColorTranslator]::FromHtml("#FFFFFF"))
  $format = New-Object System.Drawing.StringFormat
  $format.Alignment = [System.Drawing.StringAlignment]::Center
  $format.LineAlignment = [System.Drawing.StringAlignment]::Center
  $rect = New-Object System.Drawing.RectangleF 0, 0, $Size, $Size
  $graphics.DrawString("AP", $font, $brush, $rect, $format)

  $dir = Split-Path -Parent $Path
  New-Item -ItemType Directory -Force -Path $dir | Out-Null
  $bitmap.Save($Path, [System.Drawing.Imaging.ImageFormat]::Png)

  $graphics.Dispose()
  $bitmap.Dispose()
}

if (-not (Test-Path $ExePath)) {
  throw "Windows executable not found: $ExePath. Build it first with: wails build -platform windows/amd64 -webview2 download -o AgentPal-windows-x64.exe"
}

$makeAppx = Find-WindowsSdkTool "makeappx.exe"
$signTool = Find-WindowsSdkTool "signtool.exe"
$root = Resolve-Path "."
$stage = Join-Path $root "build\msix\stage"
$manifestTemplate = Join-Path $root "build\msix\AppxManifest.template.xml"
$manifestPath = Join-Path $stage "AppxManifest.xml"

Remove-Item -Recurse -Force $stage -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force -Path $stage | Out-Null
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $OutputPath) | Out-Null

Copy-Item $ExePath (Join-Path $stage "AgentPal-windows-x64.exe") -Force

$manifest = Get-Content $manifestTemplate -Raw
$manifest = $manifest.Replace("{{IDENTITY_NAME}}", $IdentityName)
$manifest = $manifest.Replace("{{PUBLISHER}}", $Publisher)
$manifest = $manifest.Replace("{{PUBLISHER_DISPLAY_NAME}}", $PublisherDisplayName)
$manifest = $manifest.Replace("{{VERSION}}", $Version)
Set-Content -Path $manifestPath -Value $manifest -Encoding UTF8

New-LogoPng -Path (Join-Path $stage "Assets\Square44x44Logo.png") -Size 44
New-LogoPng -Path (Join-Path $stage "Assets\Square150x150Logo.png") -Size 150
New-LogoPng -Path (Join-Path $stage "Assets\StoreLogo.png") -Size 50

Remove-Item -Force $OutputPath -ErrorAction SilentlyContinue
& $makeAppx pack /d $stage /p $OutputPath /overwrite

if ($CertificatePath -ne "") {
  if (-not (Test-Path $CertificatePath)) { throw "Certificate not found: $CertificatePath" }
  if ($CertificatePassword -ne "") {
    & $signTool sign /fd SHA256 /f $CertificatePath /p $CertificatePassword $OutputPath
  } else {
    & $signTool sign /fd SHA256 /f $CertificatePath $OutputPath
  }
  & $signTool verify /pa /v $OutputPath
} else {
  Write-Host "MSIX created without local signing: $OutputPath"
  Write-Host "For sideloading or strict Device Guard policies, sign it with a trusted code-signing certificate."
}

Write-Host "Done: $OutputPath"
