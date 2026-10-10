@echo off
title Docker停止 - Go API Gateway

cd /d "%~dp0.."

echo ============================================================
echo           停止 Go API Gateway Docker 环境
echo ============================================================
echo.
echo 将停止并移除以下容器：
echo   - go-api-gateway
echo   - go-api-gateway-etcd
echo   - go-api-gateway-redis
echo   - go-api-gateway-test-a
echo   - go-api-gateway-test-b
echo.

docker-compose -f deploy/compose/docker-compose.yml down

if %errorlevel%==0 (
    echo.
    echo ============================================================
    echo   Docker 环境已停止并清理！
    echo   （数据卷 gateway-data 保留，如需彻底删除请加 -v 参数）
    echo ============================================================
) else (
    echo.
    echo ? 停止过程中出现问题，可能部分容器仍在运行
    echo 可运行 docker ps 查看当前容器状态
)
echo.
pause
