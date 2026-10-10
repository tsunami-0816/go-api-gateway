@echo off
title Docker一键启动 - Go API Gateway

cd /d "%~dp0.."

echo ============================================================
echo           Go API Gateway Docker 一键启动
echo ============================================================
echo.
echo 此方式将通过 Docker Compose 启动完整环境：
echo   - gateway      [网关核心，端口 8080/8081]
echo   - etcd         [服务发现，端口 2379]
echo   - redis        [缓存/限流，端口 6379]
echo   - test-service-a  [测试服务A，端口 8090]
echo   - test-service-b  [测试服务B，端口 8091]
echo.
echo 前提条件：已安装 Docker Desktop 并启动
echo.

where docker >nul 2>&1
if %errorlevel% neq 0 (
    echo ? 未检测到 Docker，请先安装 Docker Desktop
    echo 下载地址: https://www.docker.com/products/docker-desktop/
    pause
    exit /b 1
)

docker info >nul 2>&1
if %errorlevel% neq 0 (
    echo ? Docker 引擎未运行，请先启动 Docker Desktop
    pause
    exit /b 1
)

echo 正在构建并启动所有容器（首次构建可能需要几分钟）...
echo.
docker-compose -f deploy/compose/docker-compose.yml up -d --build

if %errorlevel%==0 (
    echo.
    echo ============================================================
    echo   Docker 环境启动成功！
    echo.
    echo   网关管理面: http://localhost:8081
    echo   网关数据面: http://localhost:8080
    echo   测试服务A:  http://localhost:8090
    echo   测试服务B:  http://localhost:8091
    echo.
    echo   查看容器状态: docker ps
    echo   查看网关日志: docker logs go-api-gateway -f
    echo   停止所有服务: 运行 10-Docker停止.bat
    echo ============================================================
) else (
    echo.
    echo ? 启动失败，请查看上方错误信息
)
echo.
pause
