@echo off
title 安装依赖 - Go API Gateway

echo ============================================================
echo           Go API Gateway 依赖安装
echo ============================================================
echo.
echo 本脚本将自动安装：
echo   1. Go 后端依赖 [go mod download]
echo   2. 前端依赖 [npm install]
echo.

cd /d "%~dp0.."

echo [1/2] 安装 Go 后端依赖...
where go >nul 2>&1
if %errorlevel% neq 0 (
    echo       ? 未检测到 Go，请先安装 Go 1.22+
    echo       下载地址: https://go.dev/dl/
    goto :fail
)
echo       正在下载 Go 依赖包，请稍候...
go mod download
if %errorlevel%==0 (
    echo       ? Go 依赖安装完成
) else (
    echo       ? Go 依赖安装失败，请检查网络连接
    goto :fail
)
echo.

echo [2/2] 安装前端依赖...
where node >nul 2>&1
if %errorlevel% neq 0 (
    echo       ? 未检测到 Node.js，请先安装 Node.js 18+
    echo       下载地址: https://nodejs.org/
    goto :fail
)
cd web
if exist "node_modules" (
    echo       node_modules 已存在，跳过安装（如需重新安装请先删除 node_modules 目录）
) else (
    echo       正在安装前端依赖包，首次安装可能需要几分钟...
    call npm install
    if %errorlevel%==0 (
        echo       ? 前端依赖安装完成
    ) else (
        echo       ? 前端依赖安装失败，请检查网络连接
        goto :fail
    )
)
cd ..
echo.

echo ============================================================
echo   全部依赖安装完成！
echo   下一步可以运行: 06-一键启动全部.bat
echo ============================================================
echo.
pause
exit /b 0

:fail
echo.
echo ============================================================
echo   安装过程中出现错误，请根据上方提示处理。
echo ============================================================
echo.
pause
exit /b 1
