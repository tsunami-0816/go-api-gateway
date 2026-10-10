@echo off
title 网关后端 - Go API Gateway

cd /d "%~dp0.."

echo ============================================================
echo   正在启动 Go API Gateway 网关后端...
echo   管理面端口: 8081
echo   数据面端口: 8080
echo   配置文件: config/local-demo.yaml
echo ============================================================
echo.
echo 提示: 关闭此窗口即可停止网关后端
echo.

if exist "bin\gateway.exe" (
    echo 使用已编译的二进制文件启动...
    bin\gateway.exe --config config/local-demo.yaml
) else (
    echo 未找到编译好的二进制，使用 go run 启动（首次会稍慢）...
    go run ./cmd/gateway --config config/local-demo.yaml
)

pause
