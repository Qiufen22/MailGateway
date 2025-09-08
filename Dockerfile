# 多阶段构建 Dockerfile
# 第一阶段：构建阶段
FROM golang:1.24-alpine AS builder

# 设置工作目录
WORKDIR /app

# 设置Go代理为国内源
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=sum.golang.google.cn

# 安装必要的包
RUN apk add --no-cache git

# 复制 go mod 文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 整理依赖，确保 go.mod/go.sum 与源码一致
RUN go mod tidy

# 构建应用
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# 第二阶段：运行阶段
FROM alpine:3.20

# 安装必要的包
RUN apk --no-cache add ca-certificates tzdata

# 设置时区
RUN ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime
RUN echo 'Asia/Shanghai' > /etc/timezone

# 创建非root用户
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

# 设置工作目录
WORKDIR /app

# 从构建阶段复制二进制文件
COPY --from=builder /app/main .

# 复制配置文件和静态资源
COPY --from=builder /app/conf ./conf
COPY --from=builder /app/web ./web

# 创建日志目录
RUN mkdir -p logs

# 初始化日志目录与日志文件权限
RUN mkdir -p /app/logs && \
    touch /app/logs/app.log && \
    chown -R appuser:appgroup /app && \
    chmod 755 /app/logs && \
    chmod 664 /app/logs/app.log

# 切换到非root用户
USER appuser

# 暴露端口
EXPOSE 8089



# 启动脚本
CMD ["./main"]