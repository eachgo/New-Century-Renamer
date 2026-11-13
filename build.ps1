# 构建 ncreamer(跨向更名器)的 CLI 与 GUI
#   .\build.ps1          # 只构建当前平台
#   .\build.ps1 -All     # 交叉编译 Windows/Linux/macOS 的 amd64 与 arm64
# 输出为纯文本(不用 Format-Table),便于被其他脚本管道消费。
#
# GUI 走 `go tool mygo build`,不用裸 `go build`:图标、版本信息、
# 清单(逐显示器 DPI 感知)都由 mygo 写成 .syso 资源链进 exe。裸 go build
# 出来的 exe 是白底默认图标——这就是以前"准备了图标但产物没图标"的原因。
# GUI 的版本号以 gui/mygo.json 为准(它写进 PE 版本信息和 Linux 包名),
# -Version 只影响 CLI;两者不一致时下面会提示。
param(
    [switch]$All,
    [string]$Version = ""
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$dist = Join-Path $root "dist"
New-Item -ItemType Directory -Force -Path $dist | Out-Null

# mygo build 的中间产物目录(已在 .gitignore 里)
$stage = Join-Path $root "_local\mygo-build"

# CLI 与 GUI 各自的版本:CLI 由 -Version 注入,GUI 读 mygo.json
$guiVersion = (Get-Content (Join-Path $root "gui\mygo.json") -Raw | ConvertFrom-Json).version
if (-not $Version) { $Version = $guiVersion }
elseif ($Version -ne $guiVersion) {
    Write-Host "注意:GUI 版本取自 gui/mygo.json 的 $guiVersion,-Version $Version 只用于 CLI"
    Write-Host "      要一起升,先改 gui/mygo.json 的 version"
}

# 用 "os/arch" 字符串避免嵌套数组被 PowerShell 展平
$targets = if ($All) {
    @("windows/amd64", "windows/arm64", "linux/amd64", "linux/arm64", "darwin/arm64")
} else {
    @(((go env GOOS).Trim() + "/" + (go env GOARCH).Trim()))
}

function Build-Cli([string]$goos, [string]$goarch) {
    $name = "ncreamer-$goos-$goarch"
    if ($goos -eq "windows") { $name += ".exe" }
    $out = Join-Path $dist $name
    Write-Host "  CLI  $name"
    Push-Location $root
    try {
        $env:GOOS = $goos; $env:GOARCH = $goarch
        go build -trimpath -ldflags "-s -w -X main.version=$Version" -o $out ./cmd/ncreamer
        if ($LASTEXITCODE) { throw "CLI 构建失败 ($goos/$goarch)" }
    } finally { Pop-Location }
}

function Build-Gui([string]$goos, [string]$goarch) {
    Write-Host "  GUI  $goos/$goarch"
    Push-Location (Join-Path $root "gui")
    try {
        $env:GOOS = ""; $env:GOARCH = ""   # mygo build 自己设 GOOS/GOARCH
        # -skip-installer:不拉 NSIS 做 Windows 安装器(发布流水线也不需要)
        # -skip-dmg:磁盘镜像只能在 macOS 上做
        go tool mygo build -platform "$goos/$goarch" -skip-installer -skip-dmg -o $stage
        if ($LASTEXITCODE) { throw "GUI 构建失败 ($goos/$goarch)" }
    } finally { Pop-Location }

    $built = Join-Path $stage "$goos-$goarch"
    switch ($goos) {
        "windows" {
            # exe 里已带图标,单个文件就能发
            $out = Join-Path $dist "ncreamer-gui-$goos-$goarch.exe"
            Copy-Item (Join-Path $built "ncreamer-gui.exe") $out -Force
        }
        "darwin" {
            # macOS 要的是 .app 包(双击即开),不是裸二进制
            $out = Join-Path $dist "ncreamer-gui-$goos-$goarch.zip"
            if (Test-Path $out) { Remove-Item $out -Force }
            Compress-Archive -Path (Join-Path $built "ncreamer-gui.app") -DestinationPath $out
        }
        default {
            # Linux 的图标在 .desktop 里指向 png,裸二进制会丢图标,
            # 所以发 mygo 打好的 tar.gz(二进制 + png + desktop + install.sh)
            $tarball = Get-ChildItem $built -Filter "*.tar.gz" | Select-Object -First 1
            $out = Join-Path $dist "ncreamer-gui-$goos-$goarch.tar.gz"
            Copy-Item $tarball.FullName $out -Force
        }
    }
    Write-Host "       -> $(Split-Path -Leaf $out)"
}

foreach ($t in $targets) {
    $goos, $goarch = $t -split '/'
    Write-Host "构建 $goos/$goarch"
    Build-Cli $goos $goarch
    # Linux GUI 要在有图形会话的机器上才跑得起来(X11/Wayland + OpenGL),
    # 但交叉编译没问题,所以照常出包;只是本脚本不把它当默认目标。
    if ($goos -ne "linux" -or $All) { Build-Gui $goos $goarch }
}

$env:GOOS = ""; $env:GOARCH = ""
Write-Host ""
Write-Host "完成,产物在 $dist"
foreach ($f in Get-ChildItem $dist -File) {
    Write-Host ("  {0,-40} {1,6:N1} MB" -f $f.Name, ($f.Length / 1MB))
}
