$ErrorActionPreference = "Stop"

# 1. 构建前端
Push-Location web
npm install
npm run build
Pop-Location

# 2. 构建单 exe（纯 Go，无 CGO；-H=windowsgui 无控制台窗口）
$windres = Get-Command windres.exe -ErrorAction SilentlyContinue
if ($windres) {
    & $windres.Source --codepage=65001 --target=pe-x86-64 -i resource.rc -o resource_windows_amd64.syso -O coff
    if ($LASTEXITCODE -ne 0) { throw "Windows 图标/版本资源生成失败" }
} elseif (-not (Test-Path resource_windows_amd64.syso)) {
    throw "缺少 windres.exe，且未找到已生成的 resource_windows_amd64.syso"
}

$env:CGO_ENABLED = "0"
go build -trimpath -ldflags "-s -w -H=windowsgui -X main.version=1.0.0" -o 路由管理.exe .

# 3. UPX 压缩（输出到临时文件再替换，失败不影响已构建的 exe）
$upx = Get-Command upx.exe -ErrorAction SilentlyContinue
if ($upx) {
    & $upx.Source --best -o 路由管理._upx.exe 路由管理.exe
    if ($LASTEXITCODE -ne 0) { throw "UPX 压缩失败" }
    Move-Item -Force 路由管理._upx.exe 路由管理.exe
    Write-Host "UPX: 已压缩（$(Get-Item 路由管理.exe | Select-Object -ExpandProperty Length | ForEach-Object { '{0:N0} KB' -f ($_ / 1KB) })）"
} else {
    Write-Warning "未找到 upx.exe（scoop install upx），跳过压缩"
}

# 4. 发布：覆盖 scoop 路径下的 router.exe（PATH 上的部署入口）。
#    router.exe 正在运行时文件被占用会导致复制失败，提示先关闭旧实例。
$scoopRoot = if ($env:SCOOP) { $env:SCOOP } else { "D:\scoop" }
$shims = Join-Path $scoopRoot "shims"
$target = Join-Path $shims "router.exe"
if (Test-Path $shims) {
    try {
        Copy-Item -Path "路由管理.exe" -Destination $target -Force -ErrorAction Stop
        Write-Host "发布: 已覆盖 $target"
    } catch {
        Write-Warning "发布: 无法覆盖 $target —— 若程序正在运行请先退出 router.exe 再手动复制"
    }
} else {
    Write-Warning "发布: 未找到 scoop 目录 $shims，跳过复制"
}

Write-Host ""
Write-Host "Build OK: 路由管理.exe"
Write-Host "部署：拷贝 路由管理.exe 到跳板机，双击启动（UAC 确认后浏览器自动打开）；或直接运行 router.exe（已自动更新）。"
