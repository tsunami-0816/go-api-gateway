@echo off
title 管理前端 - Go API Gateway

cd /d "%~dp0..\web"

echo ============================================================
echo   正在启动管理前端 [Vue 3 + Vite]...
echo   访问地址: http://localhost:5173
echo   默认登录 Token: admin-token-dev-001
echo ============================================================
echo.
echo 提示: 关闭此窗口即可停止前端
echo.

if not exist "node_modules" (
    echo 未检测到 node_modules，正在自动安装前端依赖...
    call npm install
    if %errorlevel% neq 0 (
        echo.
        echo 依赖安装失败，请手动运行 02-安装依赖.bat
        pause
        exit /b 1
    )
    echo.
)

call npm run dev

pause
