# MailGateway - 邮件网关认证系统

## 项目简介

MailGateway 是一个基于 Go 语言开发的高性能邮件网关认证系统，专为 Nginx 邮件代理模块设计。该系统提供了完整的用户认证、管理和监控功能，支持 POP3、IMAP、SMTP 协议的统一认证，并集成了 TOTP 双因子认证、实时监控、日志审计等企业级功能。

## 界面预览

> 以下为系统的关键界面示例，便于快速了解产品外观与交互流程。

### 登录页

<img width="1658" height="876" alt="image" src="https://github.com/user-attachments/assets/6efd5485-1328-4979-abec-ed17e5b4afde" />


### 后台首页（系统设置）

<img width="1658" height="876" alt="image" src="https://github.com/user-attachments/assets/b24b252e-ce50-4e72-a71d-cce97ea1efae" />


> 如果图片未显示，请将截图文件放到仓库路径：docs/images/，并命名为 login.png 与 dashboard.png。

## 核心特性

### 🔐 认证功能
- **多协议支持**: 支持 POP3、IMAP、SMTP 协议认证
- **双因子认证**: 集成 TOTP (Time-based One-Time Password) 支持
- **密码缓存**: Redis 缓存机制提升认证性能
- **失败保护**: 自动锁定机制防止暴力破解
- **后端路由**: 智能后端服务器选择和负载均衡

### 📊 管理功能
- **用户管理**: 完整的用户 CRUD 操作
- **管理员管理**: 多级管理员权限控制
- **系统配置**: 灵活的配置管理系统
- **IP 黑名单**: 动态 IP 封禁管理
- **仪表盘**: 实时统计和数据可视化

### 📈 监控与日志
- **认证日志**: 详细的认证记录和审计
- **操作日志**: 管理操作的完整追踪
- **实时统计**: QPS、响应时间、活跃用户等指标
- **日志分析**: 支持多维度查询和统计

### 🛡️ 安全特性
- **JWT 认证**: 安全的 API 访问控制
- **密码加密**: bcrypt 加密存储
- **CORS 支持**: 跨域请求安全控制
- **请求限制**: 防止 API 滥用
- **安全头**: 完整的 HTTP 安全头设置

## 技术架构

### 技术栈
- **后端框架**: Gin (Go Web Framework)
- **数据库**: MySQL 8.0+
- **缓存**: Redis 6.0+
- **日志**: Zap (结构化日志)
- **配置**: Viper (配置管理)
- **认证**: JWT + TOTP
- **加密**: bcrypt

### 项目结构
```
MailGateway/
├── config/                 # 配置管理
│   └── config.go          # 配置结构和加载逻辑
├── conf/                   # 配置文件
│   └── config.yaml        # 主配置文件
├── handlers/               # HTTP 处理器
│   ├── auth.go            # 认证相关接口
│   ├── system.go          # 系统管理接口
│   └── user.go            # 用户管理接口
├── middleware/             # 中间件
│   ├── cors.go            # CORS 中间件
│   ├── error.go           # 错误处理中间件
│   ├── jwt.go             # JWT 认证中间件
│   └── logger.go          # 日志中间件
├── models/                 # 数据模型
│   ├── admin.go           # 管理员模型
│   ├── auth.go            # 认证模型
│   ├── auth_log.go        # 认证日志模型
│   └── user.go            # 用户模型
├── router/                 # 路由配置
│   └── router.go          # 路由注册和配置
├── services/               # 业务逻辑层
│   ├── auth.go            # 认证服务
│   ├── database.go        # 数据库服务
│   ├── jwt.go             # JWT 服务
│   ├── redis.go           # Redis 服务
│   └── totp.go            # TOTP 服务
├── logs/                   # 日志文件目录
├── main.go                 # 应用入口
├── go.mod                  # Go 模块文件
├── go.sum                  # 依赖校验文件
└── README.md               # 项目说明文档
```

### 数据库设计

#### 核心表结构
- **users**: 用户信息表（用户名、密码、TOTP 配置等）
- **admin**: 管理员表（管理员账户和权限）
- **auth_log**: 认证日志表（认证记录和审计）
- **operation_log**: 操作日志表（管理操作记录）
- **system_config**: 系统配置表（动态配置存储）
- **ip_blacklist**: IP 黑名单表（IP 封禁管理）

## 快速开始

### Docker 直接拉取镜像部署（不改配置）

适用场景：
- 不修改 conf/config.yaml（保留 auth_server.host=127.0.0.1）
- Linux 环境（使用 host 网络）

命令：
```bash
sudo docker pull qiufen22/mailgateway:latest

sudo mkdir -p /opt/mailgateway/conf/logs
# 请将你的 config.yaml 放到 /opt/mailgateway/conf/config.yaml
# sudo cp /path/to/config.yaml /opt/mailgateway/conf/config.yaml

sudo docker run -d --name mailgateway \
  --network host \
  -e GIN_MODE=release \
  -v /opt/mailgateway/conf:/app/conf:ro \
  -v /opt/mailgateway/conf/logs:/app/logs:rw \
  --restart unless-stopped \
  qiufen22/mailgateway:latest
```

验证：
- 管理端：http://0.0.0.0:8089
- 认证接口：http://127.0.0.1:8090/api/auth

### 环境要求
- Go 1.19+
- MySQL 8.0+
- Redis 6.0+

### 安装与部署

#### 方式一：本地编译安装部署（推荐用于开发环境）
- 前置要求：Go 1.24+、MySQL 8.0+、Redis 6.0+

1. 克隆项目
```bash
git clone <repository-url>
cd MailGateway
```

2. 安装依赖
```bash
go mod download
```

3. 配置应用
- 编辑 `conf/config.yaml`，按“配置说明”章节完善数据库和 Redis 参数

4. 编译与运行
```bash
# 编译
mkdir -p bin
go build -o bin/mailgateway .

# 运行（Linux/macOS）
./bin/mailgateway
# Windows
# .\\bin\\mailgateway.exe
```
应用启动后访问：`http://localhost:8089`

---

#### 方式二：Docker 安装部署（推荐用于快速试用/生产）
- 前置要求：Docker、Docker Compose

1. 使用脚本快速启动
```bash
chmod +x docker-start.sh
./docker-start.sh
```
或手动执行：
```bash
docker compose up --build -d
# 旧版本请使用：docker-compose up --build -d
```

2. 查看日志/管理服务
```bash
docker compose logs -f mailgateway
# 停止：docker compose down
# 重启：docker compose restart
```

3. 更多部署细节与故障排除
- 请参见项目根目录下的 DOCKER.md
- 如遇 `./logs` 目录写入权限问题，可在宿主机执行（Linux）：
```bash
sudo chown -R 1001:1001 logs
# 或临时方案（仅测试环境）：sudo chmod -R 777 logs
```

---

#### 方式三：二进制文件下载部署（无需本地编译）
- 前置要求：MySQL 8.0+、Redis 6.0+

1. 下载与解压
- 从项目的发布页下载与你平台匹配的压缩包（例如：`mailgateway_<version>_<os>_<arch>.tar.gz/zip`）
- 解压到目标目录

2. 配置应用
- 编辑 `conf/config.yaml`，按“配置说明”章节完善数据库和 Redis 参数

3. 直接运行
```bash
# Linux/macOS
chmod +x mailgateway
./mailgateway

# Windows
# .\\mailgateway.exe
```
应用启动后访问：`http://localhost:8089`

### 默认账户
- **管理员账户**: admin / admin123

## 配置说明

### 主要配置项

```yaml
# 服务器配置
server:
  host: "0.0.0.0"  # 监听地址: 127.0.0.1(本地) 或 0.0.0.0(所有接口)
  port: "8089"        # 管理服务监听端口
  mode: "debug"       # debug, release, test

# Nginx认证服务配置
auth_server:
  host: "127.0.0.1"   # 认证服务监听地址，强制本地访问
  port: "8090"        # 认证服务监听端口

# 数据库配置
database:
  host: "localhost"
  port: "3306"
  user: "root"
  password: "password"
  database: "mail"
  max_open_conns: 600         # 最大连接数
  max_idle_conns: 200         # 最大空闲连接数
  conn_max_lifetime: "1h"     # 连接最大生存时间

# Redis 配置
redis:
  host: "localhost"
  port: "6379"
  password: ""
  db: 0

# 认证配置
auth:
  max_fail_attempts: 5        # 最大失败尝试次数
  fail_ttl: "60s"            # 失败计数过期时间
  lock_ttl: "300s"           # 账户锁定时间

# JWT 配置
jwt:
  secret: "your-jwt-secret"   # JWT 密钥（生产环境请修改）
  expiration: "24h"          # 访问令牌过期时间
  refresh_expiration: "168h" # 刷新令牌过期时间

# TOTP 配置
totp:
  issuer: "MailGateway"       # TOTP 发行者名称
  secret_length: 16          # TOTP 密钥长度

# 日志配置
log:
  level: "info"              # 日志级别: debug/info/warn/error
  file_path: "logs/app.log"   # 日志文件路径
```

## 认证流程

1. **标准认证**: 用户名 + 密码
2. **TOTP 认证**: 用户名 + 密码 + 6位验证码（附加在密码末尾）
3. **失败处理**: 支持登录失败计数与锁定策略（由 system_config 控制）
4. **缓存机制**: 成功认证后缓存密码 5 分钟

### 管理员登录与安全策略

- 接口地址：`POST /login`
- 请求参数：`username`、`password`、`totp_code`（可选，开启TOTP后需提供）
- 错误信息：为防止信息泄露，触发TOTP后的所有错误均统一返回“登录失败”
- 失败计数：密码错误、TOTP验证码错误均计入失败次数
- 锁定策略（system_config 配置）：
  - `LOCKED`：是否启用账户锁定（true/false）
  - `LOCK_ERRORS`：触发锁定的失败次数阈值（默认 5 次）
  - `LOCK_TIME`：锁定时长（分钟，默认 30）
- 锁定期间：该管理员账户将被拒绝登录
- 成功登录：自动清除失败计数与锁定标记，并记录完整操作日志

## Nginx 集成

### Nginx 配置示例

```nginx
mail {
    server_name mail.example.com;
    auth_http http://127.0.0.1:8090/api/auth;
    auth_http_header User-Agent "Nginx";
    
    server {
        listen 110;
        protocol pop3;
        proxy on;
    }
    
    server {
        listen 143;
        protocol imap;
        proxy on;
    }
    
    server {
        listen 25;
        protocol smtp;
        proxy on;
    }
}
```

## 监控与日志

### 日志类型

1. **应用日志**: 系统运行日志（`logs/app.log`）
2. **认证日志**: 用户认证记录（数据库存储）
3. **操作日志**: 管理操作记录（数据库存储）
4. **HTTP 日志**: API 请求日志（结构化输出）

### 监控指标

- **QPS**: 每秒查询数
- **响应时间**: 平均响应时间
- **活跃用户**: 当前活跃用户数
- **认证成功率**: 认证成功/失败比例
- **错误率**: 系统错误统计

## 安全建议

### 生产环境配置

1. **修改默认密码**: 更改默认管理员密码
2. **更新 JWT 密钥**: 使用强随机密钥
3. **启用 HTTPS**: 配置 SSL/TLS 证书
4. **数据库安全**: 使用专用数据库用户和强密码
5. **防火墙配置**: 限制不必要的端口访问
6. **日志轮转**: 配置日志文件轮转和清理

### 安全特性

- **密码加密**: 使用 bcrypt 加密存储
- **会话管理**: JWT 令牌自动过期
- **IP 黑名单**: 动态 IP 封禁
- **请求限制**: 防止 API 滥用
- **审计日志**: 完整的操作记录

## 故障排除

### 常见问题

1. **数据库连接失败**
   - 检查数据库服务状态
   - 验证连接配置信息
   - 确认网络连通性

2. **Redis 连接失败**
   - 检查 Redis 服务状态
   - 验证密码和端口配置
   - 检查防火墙设置

3. **认证失败**
   - 检查用户账户状态
   - 验证 TOTP 配置
   - 查看认证日志

4. **性能问题**
   - 检查数据库连接池配置
   - 监控 Redis 缓存命中率
   - 分析慢查询日志

### 日志查看

```bash
# 查看应用日志
tail -f logs/app.log

# 查看错误日志
grep "ERROR" logs/app.log

# 查看认证日志
# 通过管理界面或 API 查询数据库记录
```

## 开发指南

### 代码结构

- **handlers**: HTTP 请求处理器，负责请求解析和响应
- **services**: 业务逻辑层，包含核心业务功能
- **models**: 数据模型定义，包含请求/响应结构
- **middleware**: 中间件，处理认证、日志、错误等
- **config**: 配置管理，支持文件和环境变量

### 添加新功能

1. 在 `models` 中定义数据结构
2. 在 `services` 中实现业务逻辑
3. 在 `handlers` 中添加 HTTP 处理器
4. 在 `router` 中注册路由
5. 更新配置和文档

## 许可证

本项目采用 MIT 许可证，详见 LICENSE 文件。

## 贡献

欢迎提交 Issue 和 Pull Request 来改进项目。

## 联系方式

如有问题或建议，请通过以下方式联系：
- 提交 GitHub Issue
- 发送邮件至项目维护者

---

**注意**: 本项目仅供学习和研究使用，生产环境部署前请进行充分的安全评估和测试。
