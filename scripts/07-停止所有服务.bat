@echo off
title 停止所有服务 - Go API Gateway

echo ============================================================
echo           停止 Go API Gateway 所有服务
echo ============================================================
echo.

echo 正在停止测试后端 [backend.exe / go run test-services]...
taskkill /f /im backend.exe >nul 2>&1
echo       已停止 backend.exe

echo 正在停止网关后端 [gateway.exe]...
taskkill /f /im gateway.exe >nul 2>&1
echo       已停止 gateway.exe

echo 正在停止前端开发服务器 [node.exe - Vite]...
for /f "tokens=2" %%p in ('netstat -ano ^| findstr :5173 ^| findstr LISTENING') do (
    taskkill /f /pid %%p >nul 2>&1
    echo       已停止 PID %%p [端口 5173]
)

echo 正在停止 Go 运行进程 [go run 相关]...
for /f "tokens=2" %%p in ('netstat -ano ^| findstr :8080 ^| findstr LISTENING') do (
    taskkill /f /pid %%p >nul 2>&1
    echo       已停止 PID %%p [端口 8080]
)
for /f "tokens=2" %%p in ('netstat -ano ^| findstr :8081 ^| findstr LISTENING') do (
    taskkill /f /pid %%p >nul 2>&1
    echo       已停止 PID %%p [端口 8081]
)
for /f "tokens=2" %%p in ('netstat -ano ^| findstr :9001 ^| findstr LISTENING') do (
    taskkill /f /pid %%p >nul 2>&1
    echo       已停止 PID %%p [端口 9001]
)

echo.
echo ============================================================
echo   所有服务已停止！
echo   端口 8080、8081、9001、5173 已释放
echo ============================================================
echo.
pause
