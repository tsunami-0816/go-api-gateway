@echo off
title 测试后端 - Go API Gateway

cd /d "%~dp0..\test-services"

echo ============================================================
echo   正在启动测试后端服务...
echo   监听端口: 9001
echo   作用: 模拟真实业务服务，用于演示网关代理转发
echo ============================================================
echo.
echo 提示: 关闭此窗口即可停止测试后端
echo.

if exist "backend.exe" (
    echo 使用已编译的二进制文件启动...
    backend.exe 9001
) else (
    echo 未找到编译好的二进制，使用 go run 启动...
    where go >nul 2>&1
    if %errorlevel% neq 0 (
        echo ? 未检测到 Go，无法启动测试后端
        echo 请先安装 Go 1.22+，或运行 01-检查环境.bat
        pause
        exit /b 1
    )
    go run main.go 9001
)

pause
