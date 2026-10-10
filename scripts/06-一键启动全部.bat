@echo off
title 一键启动全部 - Go API Gateway

cd /d "%~dp0"

echo ============================================================
echo           Go API Gateway 一键启动
echo ============================================================
echo.
echo 将依次启动以下服务（每个服务一个独立窗口）：
echo   1. 测试后端    [端口 9001] - 模拟业务服务
echo   2. 网关后端    [端口 8080/8081] - 核心网关
echo   3. 管理前端    [端口 5173] - 可视化管理界面
echo.
echo 启动完成后：
echo   - 管理后台地址: http://localhost:5173
echo   - 登录 Token: admin-token-dev-001
echo   - 网关代理地址: http://localhost:8080
echo.
echo 提示: 要停止所有服务，请运行 07-停止所有服务.bat
echo.
pause

echo.
echo [1/3] 启动测试后端...
start "测试后端-9001" cmd /k "%~dp005-启动测试后端.bat"
timeout /t 2 /nobreak >nul

echo [2/3] 启动网关后端...
start "网关后端-8080/8081" cmd /k "%~dp003-启动网关后端.bat"
timeout /t 3 /nobreak >nul

echo [3/3] 启动管理前端...
start "管理前端-5173" cmd /k "%~dp004-启动管理前端.bat"

echo.
echo ============================================================
echo   所有服务已启动！
echo   等待几秒后在浏览器打开: http://localhost:5173
echo   登录 Token: admin-token-dev-001
echo.
echo   想体验完整流程？运行 08-演示主流程.bat
echo ============================================================
echo.
pause
