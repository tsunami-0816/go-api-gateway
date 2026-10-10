@echo off
title 主流程演示 - Go API Gateway

setlocal enabledelayedexpansion

cd /d "%~dp0.."

set ADMIN_URL=http://127.0.0.1:8081
set DATA_URL=http://127.0.0.1:8080
set TOKEN=admin-token-dev-001
set TMP_JSON=%TEMP%\gw_demo_body.json
set TMP_OUT=%TEMP%\gw_demo_out.txt
set TMP_STAT=%TEMP%\gw_demo_stat.txt

echo ============================================================
echo           Go API Gateway 核心主流程自动演示
echo ============================================================
echo.
echo 本脚本将自动完成以下操作：
echo   1. 检查网关是否正常运行
echo   2. 创建服务 [test-service]
echo   3. 创建路由 [/test/ -^> 转发到 test-service]
echo   4. 查看路由列表（确认创建成功）
echo   5. 通过网关代理访问测试后端（验证转发生效）
echo   6. 查看服务列表
echo.
echo 前提条件：已运行 06-一键启动全部.bat 启动所有服务
echo.
pause

echo.
echo ============================================================
echo [步骤 1/6] 检查网关健康状态...
echo ============================================================
curl -s -o "%TMP_OUT%" -w "%%{http_code}" "%ADMIN_URL%/healthz" > "%TMP_STAT%"
set /p STATUS_CODE=<"%TMP_STAT%"
if "%STATUS_CODE%"=="200" (
    echo       [OK] 网关管理面运行正常 [HTTP 200]
    type "%TMP_OUT%"
    echo.
) else (
    echo       [FAIL] 网关未响应 [HTTP %STATUS_CODE%]
    echo       请先运行 06-一键启动全部.bat 启动网关
    goto :cleanup_fail
)
echo.

echo ============================================================
echo [步骤 2/6] 创建服务 test-service...
echo ============================================================
set SVC_JSON={"service_name":"test-service","display_name":"Demo Test Service","description":"A test service for gateway proxy demo"}
echo !SVC_JSON! > "%TMP_JSON%"
curl -s -X POST "%ADMIN_URL%/api/v1/services" ^
  -H "Authorization: Bearer %TOKEN%" ^
  -H "Content-Type: application/json" ^
  -d @"%TMP_JSON%" > "%TMP_OUT%"
echo       服务创建响应：
type "%TMP_OUT%"
echo.
echo.

echo ============================================================
echo [步骤 3/6] 创建路由 /test/ [转发到 test-service]...
echo ============================================================
set ROUTE_JSON={"route_name":"Demo Route","path":"/test/","method":"*","target_service":"test-service","rewrite_path":"strip_prefix=/test","timeout_ms":5000,"description":"Demo route: forward /test/** to test-service"}
echo !ROUTE_JSON! > "%TMP_JSON%"
curl -s -X POST "%ADMIN_URL%/api/v1/routes" ^
  -H "Authorization: Bearer %TOKEN%" ^
  -H "Content-Type: application/json" ^
  -d @"%TMP_JSON%" > "%TMP_OUT%"
echo       路由创建响应：
type "%TMP_OUT%"
echo.
echo.
echo       等待配置热更新生效（2秒）...
timeout /t 2 /nobreak >nul
echo.

echo ============================================================
echo [步骤 4/6] 查看路由列表（确认路由已创建）...
echo ============================================================
curl -s "%ADMIN_URL%/api/v1/routes?page=1&page_size=10" ^
  -H "Authorization: Bearer %TOKEN%" > "%TMP_OUT%"
type "%TMP_OUT%"
echo.
echo.

echo ============================================================
echo [步骤 5/6] 通过网关代理访问测试后端（验证转发）...
echo ============================================================
echo       请求: GET %DATA_URL%/test/hello
echo       说明: 网关收到 /test/hello 请求，匹配路由后去掉 /test 前缀，
echo             转发到测试后端的 /hello 路径
echo.
curl -s -w "\n\nHTTP状态码: %%{http_code}\n" "%DATA_URL%/test/hello" > "%TMP_OUT%"
type "%TMP_OUT%"
echo.
echo       如果上方返回了包含 backend_port、request_count 等字段的 JSON，
echo       说明网关代理转发成功！
echo.

echo ============================================================
echo [步骤 6/6] 查看服务列表...
echo ============================================================
curl -s "%ADMIN_URL%/api/v1/services?page=1&page_size=10" ^
  -H "Authorization: Bearer %TOKEN%" > "%TMP_OUT%"
type "%TMP_OUT%"
echo.
echo.

echo ============================================================
echo   主流程演示完成！
echo ============================================================
echo.
echo 你已经成功体验了 API 网关的核心流程：
echo   1. 创建了一个后端服务 [test-service]
echo   2. 创建了一条路由规则 [/test/ -^> test-service]
echo   3. 在列表中看到了创建的路由和服务
echo   4. 通过网关 [8080端口] 成功代理访问了后端服务
echo.
echo 接下来你可以：
echo   - 在浏览器打开 http://localhost:5173 用可视化界面操作
echo   - 登录 Token: admin-token-dev-001
echo   - 尝试创建限流规则、熔断策略、灰度发布等高级功能
echo.

:cleanup
del "%TMP_JSON%" "%TMP_OUT%" "%TMP_STAT%" 2>nul
pause
exit /b 0

:cleanup_fail
del "%TMP_JSON%" "%TMP_OUT%" "%TMP_STAT%" 2>nul
echo.
pause
exit /b 1
