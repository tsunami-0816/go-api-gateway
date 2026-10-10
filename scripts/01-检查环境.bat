@echo off
title 环境检查 - Go API Gateway

cd /d "%~dp0.."

echo ============================================================
echo           Go API Gateway 环境检查工具
echo ============================================================
echo.

set ALL_OK=1

echo [1/4] 检查 Go 语言环境...
where go >nul 2>&1
if %errorlevel%==0 (
    for /f "tokens=3" %%v in ('go version') do set GO_VER=%%v
    echo       [OK] 已安装 Go，版本: %GO_VER%
) else (
    echo       [FAIL] 未检测到 Go，请先安装 Go 1.22+
    echo       下载地址: https://go.dev/dl/
    set ALL_OK=0
)
echo.

echo [2/4] 检查 Node.js 环境...
where node >nul 2>&1
if %errorlevel%==0 (
    for /f "tokens=*" %%v in ('node -v') do set NODE_VER=%%v
    echo       [OK] 已安装 Node.js，版本: %NODE_VER%
) else (
    echo       [FAIL] 未检测到 Node.js，请先安装 Node.js 18+
    echo       下载地址: https://nodejs.org/
    set ALL_OK=0
)
echo.

echo [3/4] 检查 Docker 环境（可选，用于 Docker 一键部署）...
where docker >nul 2>&1
if %errorlevel%==0 (
    echo       [OK] 已安装 Docker
    docker info >nul 2>&1
    if %errorlevel%==0 (
        echo       [OK] Docker 引擎正在运行
    ) else (
        echo       [WARN] Docker 已安装但引擎未启动，请启动 Docker Desktop
    )
) else (
    echo       [--] 未检测到 Docker（不影响本地运行，仅 Docker 部署方式需要）
)
echo.

echo [4/4] 检查项目文件完整性...
if exist "go.mod" (
    echo       [OK] 后端项目文件 [go.mod] 存在
) else (
    echo       [FAIL] 未找到 go.mod，请确认脚本在项目 scripts 目录下运行
    set ALL_OK=0
)
if exist "web\package.json" (
    echo       [OK] 前端项目文件 [package.json] 存在
) else (
    echo       [FAIL] 未找到前端 package.json
    set ALL_OK=0
)
echo.

echo ============================================================
if %ALL_OK%==1 (
    echo   环境检查全部通过！可以继续下一步。
    echo   建议运行: scripts\02-安装依赖.bat
) else (
    echo   环境检查未完全通过，请根据上方提示安装缺失的软件。
    echo   安装完成后重新运行本脚本检查。
)
echo ============================================================
echo.
pause
