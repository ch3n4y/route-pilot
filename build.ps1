$ErrorActionPreference = "Stop"

# 1. 构建前端
Push-Location web
npm install
npm run build
Pop-Location

# 2. 构建单 exe（纯 Go，无 CGO；-H=windowsgui 无控制台窗口）
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags "-s -w -H=windowsgui -X main.version=1.0.0" -o RouteManager.exe .

Write-Host ""
Write-Host "Build OK: RouteManager.exe"
Write-Host "部署：拷贝 RouteManager.exe 到跳板机，双击启动（UAC 确认后浏览器自动打开）。"
