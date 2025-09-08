#!/bin/bash

# MailGateway Docker 启动脚本

set -e

echo "=== MailGateway Docker 启动脚本 ==="

# 检查Docker是否安装
if ! command -v docker &> /dev/null; then
    echo "错误: Docker 未安装，请先安装 Docker"
    exit 1
fi

# 检查docker-compose是否安装
if ! command -v docker-compose &> /dev/null; then
    echo "错误: docker-compose 未安装，请先安装 docker-compose"
    exit 1
fi

# 创建日志目录
echo "创建日志目录..."
mkdir -p logs

# 构建并启动服务
echo "构建并启动 MailGateway 服务..."
docker-compose up --build -d

# 等待服务启动
echo "等待服务启动..."
sleep 10

# 检查服务状态
echo "检查服务状态..."
docker-compose ps

# 显示日志
echo "显示最近的日志..."
docker-compose logs --tail=20 mailgateway

echo ""
echo "=== 启动完成 ==="
echo "服务地址: http://localhost:8089"
echo "查看日志: docker-compose logs -f mailgateway"
echo "停止服务: docker-compose down"
echo "重启服务: docker-compose restart"
echo "========================="