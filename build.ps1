# 构建 ncreamer(跨向更名器)的 CLI 与 GUI
#   .\build.ps1          # 只构建当前平台
#   .\build.ps1 -All     # 交叉编译 Windows/Linux/macOS 的 amd64 与 arm64
# 输出为纯文本(不用 Format-Table),便于被其他脚本管道消费。
param(
    [switch]$All,
    [string]$Version = "0.1.0"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$dist = Join-Path $root "dist"
New-Item -ItemType Directory -Force -Path $dist | Out-Null

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
    $name = "ncreamer-gui-$goos-$goarch"
    if ($goos -eq "windows") { $name += ".exe" }
    $out = Join-Path $dist $name
    Write-Host "  GUI  $name"
    Push-Location (Join-Path $root "gui")
    try {
        $env:GOOS = $goos; $env:GOARCH = $goarch
        $ldflags = "-s -w"
        if ($goos -eq "windows") { $ldflags += " -H=windowsgui" }  # 双击不弹控制台黑框
        go build -trimpath -ldflags $ldflags -o $out .
        if ($LASTEXITCODE) { throw "GUI 构建失败 ($goos/$goarch)" }
    } finally { Pop-Location }
}

foreach ($t in $targets) {
    $goos, $goarch = $t -split '/'
    Write-Host "构建 $goos/$goarch"
    Build-Cli $goos $goarch
    # mygo 的 Linux GUI 要连系统显示服务(X11/Wayland)+OpenGL,此脚本 Linux 只出 CLI
    if ($goos -ne "linux") { Build-Gui $goos $goarch }
}

$env:GOOS = ""; $env:GOARCH = ""
Write-Host ""
Write-Host "完成,产物在 $dist"
foreach ($f in Get-ChildItem $dist -File) {
    Write-Host ("  {0,-34} {1,6:N1} MB" -f $f.Name, ($f.Length / 1MB))
}
