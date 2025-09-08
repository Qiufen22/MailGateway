# MailGateway Docker 部署指南

本文档介绍如何使用 Docker 部署 MailGateway 应用。

## 前置要求

- Docker Engine 20.10+
- Docker Compose 2.0+
- 至少 512MB 可用内存
- 至少 1GB 可用磁盘空间

## 快速启动

### 国际网络环境

#### Windows 用户

```bash
# 双击运行批处理文件
docker-start.bat

# 或者在命令行中运行
docker-compose up --build -d
```

#### Linux/macOS 用户

```bash
# 给脚本执行权限
chmod +x docker-start.sh

# 运行启动脚本
./docker-start.sh

# 或者直接使用 docker-compose
docker-compose up --build -d
```

### 中国网络环境（推荐）

如果遇到镜像拉取失败或速度慢的问题，请使用中国镜像源版本：

```bash
# 给脚本执行权限
chmod +x docker-start-china.sh

# 运行中国镜像源启动脚本
./docker-start-china.sh

# 或者直接使用中国镜像源配置
docker-compose -f docker-compose.china.yml up --build -d
```

## 服务管理

### 启动服务

```bash
# 构建并启动（首次运行）
docker-compose up --build -d

# 启动已构建的服务
docker-compose up -d
```

### 停止服务

```bash
# 停止服务
docker-compose down

# 停止服务并删除数据卷（谨慎使用）
docker-compose down -v
```

### 重启服务

```bash
# 重启所有服务
docker-compose restart

# 重启特定服务
docker-compose restart mailgateway
```

### 查看服务状态

```bash
# 查看服务状态
docker-compose ps

# 查看服务日志
docker-compose logs -f mailgateway

# 查看最近的日志
docker-compose logs --tail=50 mailgateway
```

## 配置说明

### 端口配置

- 应用端口：`8089`（可在 docker-compose.yml 中修改）
- Redis端口：`6379`（如果启用Redis服务）

### 数据持久化

- 配置文件：`./conf` 目录挂载到容器的 `/app/conf`
- 日志文件：`./logs` 目录挂载到容器的 `/app/logs`

### 环境变量

在 `docker-compose.yml` 中可以配置以下环境变量：

- `GIN_MODE`: Gin框架运行模式（release/debug）
- `TZ`: 时区设置（默认：Asia/Shanghai）

## 健康检查

容器包含健康检查功能，会定期检查应用状态：

```bash
# 查看健康状态
docker-compose ps

# 手动执行健康检查
docker exec mailgateway-app wget --spider http://localhost:8089/api/health
```

## 故障排除

### 常见问题

1. **镜像拉取失败（中国网络环境）**
   ```bash
   # 错误信息：failed to resolve source metadata for docker.io/library/golang
   # 解决方案：使用中国镜像源版本
   docker-compose -f docker-compose.china.yml up --build -d
   
   # 或者配置Docker镜像加速器
   # 阿里云: https://cr.console.aliyun.com/cn-hangzhou/instances/mirrors
   # 腾讯云: https://mirror.ccs.tencentyun.com
   ```

2. **端口被占用**
   ```bash
   # 检查端口占用
   netstat -tulpn | grep 8089
   
   # 修改 docker-compose.yml 中的端口映射
   ports:
     - "8090:8089"  # 将主机端口改为8090
   ```

3. **权限问题**
   ```bash
   # Linux/macOS 下确保目录权限正确
   sudo chown -R $USER:$USER logs/
   chmod 755 logs/
   ```

4. **内存不足**
   ```bash
   # 检查Docker资源使用
   docker stats
   
   # 清理未使用的镜像和容器
   docker system prune -f
   ```

### 调试模式

```bash
# 以调试模式启动（查看详细日志）
docker-compose up --build

# 进入容器调试
docker exec -it mailgateway-app sh
```

## 生产环境部署

### 安全建议

1. 修改默认端口
2. 使用反向代理（Nginx/Traefik）
3. 启用HTTPS
4. 定期更新镜像
5. 监控资源使用情况

### 备份策略

```bash
# 备份配置文件
tar -czf mailgateway-config-$(date +%Y%m%d).tar.gz conf/

# 备份日志文件
tar -czf mailgateway-logs-$(date +%Y%m%d).tar.gz logs/
```

## 更新应用

```bash
# 停止服务
docker-compose down

# 拉取最新代码（如果从Git仓库）
git pull

# 重新构建并启动
docker-compose up --build -d
```

## 支持

如果遇到问题，请检查：

1. Docker 和 docker-compose 版本
2. 系统资源使用情况
3. 防火墙和网络配置
4. 应用日志文件

更多信息请参考主项目的 README.md 文件。