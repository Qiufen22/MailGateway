@echo off
chcp 65001 >nul
setlocal enabledelayedexpansion

echo === MailGateway Docker 启动脚本 ===
echo.

REM 检查Docker是否安装
docker --version >nul 2>&1
if errorlevel 1 (
    echo 错误: Docker 未安装，请先安装 Docker Desktop
    pause
    exit /b 1
)

REM 检查docker-compose是否安装
docker-compose --version >nul 2>&1
if errorlevel 1 (
    echo 错误: docker-compose 未安装，请先安装 docker-compose
    pause
    exit /b 1
)

REM 创建日志目录
echo 创建日志目录...
if not exist logs mkdir logs

REM 构建并启动服务
echo 构建并启动 MailGateway 服务...
docker-compose up --build -d

if errorlevel 1 (
    echo 启动失败，请检查错误信息
    pause
    exit /b 1
)

REM 等待服务启动
echo 等待服务启动...
timeout /t 10 /nobreak >nul

REM 检查服务状态
echo 检查服务状态...
docker-compose ps

REM 显示日志
echo 显示最近的日志...
docker-compose logs --tail=20 mailgateway

echo.
echo === 启动完成 ===
echo 服务地址: http://localhost:8089
echo 查看日志: docker-compose logs -f mailgateway
echo 停止服务: docker-compose down
echo 重启服务: docker-compose restart
echo =========================
echo.
echo 按任意键退出...
pause >nul