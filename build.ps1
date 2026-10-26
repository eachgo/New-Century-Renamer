# 构建 reamer 的 CLI 与 GUI
#   .\build.ps1          # 只构建本机平台
#   .\build.ps1 -All     # 交叉编译 Windows/Linux/macOS 的 amd64 与 arm64
param(
    [switch]$All,
    [string]$Version = "0.1.0"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$dist = Join-Path $root "dist"
New-Item -ItemType Directory -Force -Path $dist | Out-Null

# CLI: 控制台程序
function Build-Cli([string]$goos, [string]$goarch) {
    $name = "reamer-$goos-$goarch"
    if ($goos -eq "windows") { $name += ".exe" }
    Write-Host "  CLI  $name"
    Push-Location $root
    $env:GOOS = $goos; $env:GOARCH = $goarch
    go build -trimpath -ldflags "-s -w -X main.version=$Version" -o (Join-Path $dist $name) ./cmd/reamer
    Pop-Location
}

# GUI: -H=windowsgui 去掉控制台黑框(仅 Windows)
function Build-Gui([string]$goos, [string]$goarch) {
    $name = "reamer-gui-$goos-$goarch"
    if ($goos -eq "windows") { $name += ".exe" }
    Write-Host "  GUI  $name"
    Push-Location (Join-Path $root "gui")
    $env:GOOS = $goos; $env:GOARCH = $goarch
    $ldflags = "-s -w"
    if ($goos -eq "windows") { $ldflags += " -H=windowsgui" }
    go build -trimpath -ldflags $ldflags -o (Join-Path $dist $name) .
    Pop-Location
}

$targets = if ($All) {
    @(@("windows","amd64"), @("windows","arm64"), @("linux","amd64"), @("linux","arm64"), @("darwin","arm64"))
} else {
    @(@($env:GOOS, $env:GOARCH))
}

foreach ($t in $targets) {
    $goos, $goarch = $t
    if (-not $goos) {
        $goos = (go env GOOS); $goarch = (go env GOARCH)
    }
    Write-Host "构建 $goos/$goarch"
    Build-Cli $goos $goarch
    if ($goos -ne "linux") { Build-Gui $goos $goarch }  # 无框 UI 依赖各平台桌面环境,Linux 上自行构建
}

$env:GOOS = ""; $env:GOARCH = ""
Write-Host "`n完成,产物在 $dist"
Get-ChildItem $dist | Select-Object Name, @{n='MB';e={[math]::Round($_.Length/1MB,1)}} | Format-Table
