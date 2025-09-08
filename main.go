package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"MailGateway/config"
	"MailGateway/router"
	"MailGateway/services"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// App 应用结构
type App struct {
	config       *config.Config
	logger       *zap.Logger
	dbService    *services.DatabaseService
	redisService *services.RedisService
	totpService  *services.TOTPService
	authService  *services.AuthService
	jwtService   *services.JWTService
	server       *http.Server
	authServer   *http.Server
}

// NewApp 创建应用实例
func NewApp() (*App, error) {
	// 加载配置
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// 初始化日志
	logger, err := initLogger(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	// 初始化数据库服务
	dbService, err := services.NewDatabaseService(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database service: %w", err)
	}

	// 初始化Redis服务
	redisService, err := services.NewRedisService(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize redis service: %w", err)
	}

	// 初始化TOTP服务
	totpService := services.NewTOTPService(dbService, cfg, logger)

	// 初始化JWT服务
	jwtService := services.NewJWTService(cfg, logger)

	// 初始化认证服务
	authService := services.NewAuthService(dbService, redisService, totpService, cfg, logger)

	return &App{
		config:       cfg,
		logger:       logger,
		dbService:    dbService,
		redisService: redisService,
		totpService:  totpService,
		authService:  authService,
		jwtService:   jwtService,
	}, nil
}

// initLogger 初始化日志
func initLogger(cfg *config.Config) (*zap.Logger, error) {
	// 配置日志级别
	level := zapcore.InfoLevel
	switch cfg.Log.Level {
	case "debug":
		level = zapcore.DebugLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	}

	// 配置日志输出
	config := zap.Config{
		Level:       zap.NewAtomicLevelAt(level),
		Development: cfg.Server.Mode == "debug",
		Encoding:    "json",
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:        "timestamp",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.LowercaseLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		},
		OutputPaths:      []string{"stdout", cfg.Log.FilePath},
		ErrorOutputPaths: []string{"stderr"},
	}

	return config.Build()
}

// setupRoutes 设置管理服务路由
func (app *App) setupRoutes() *gin.Engine {
	// 设置Gin模式
	gin.SetMode(app.config.Server.Mode)

	// 使用router包设置路由
	return router.SetupRoutes(
		app.dbService,
		app.totpService,
		app.authService,
		app.jwtService,
		app.redisService,
		app.logger,
	)
}

// setupAuthRoutes 设置认证服务路由
func (app *App) setupAuthRoutes() *gin.Engine {
	// 设置Gin模式
	gin.SetMode(app.config.Server.Mode)

	// 使用router包设置认证路由
	return router.SetupAuthRoutes(
		app.dbService,
		app.authService,
		app.logger,
	)
}

// Start 启动应用
func (app *App) Start() error {
	// 设置管理服务路由
	mainRouter := app.setupRoutes()

	// 创建管理服务HTTP服务器
	app.server = &http.Server{
		Addr:           app.config.GetServerAddr(),
		Handler:        mainRouter,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB
	}

	// 设置认证服务路由
	authRouter := app.setupAuthRoutes()

	// 创建认证服务HTTP服务器
	app.authServer = &http.Server{
		Addr:           app.config.GetAuthServerAddr(),
		Handler:        authRouter,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB
	}

	// 启动管理服务器
	app.logger.Info("启动邮件网关管理服务器",
		zap.String("host", app.config.Server.Host),
		zap.String("port", app.config.Server.Port),
		zap.String("mode", app.config.Server.Mode))

	go func() {
		if err := app.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			app.logger.Fatal("启动管理服务器失败", zap.Error(err))
		}
	}()

	// 启动认证服务器
	app.logger.Info("启动邮件网关认证服务器",
		zap.String("host", app.config.AuthServer.Host),
		zap.String("port", app.config.AuthServer.Port),
		zap.String("mode", app.config.Server.Mode))

	go func() {
		if err := app.authServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			app.logger.Fatal("启动认证服务器失败", zap.Error(err))
		}
	}()

	return nil
}

// Stop 停止应用
func (app *App) Stop() error {
	app.logger.Info("正在关闭邮件网关服务器...")

	// 创建超时上下文
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 优雅关闭管理服务HTTP服务器
	if app.server != nil {
		app.logger.Info("正在关闭管理服务器...")
		if err := app.server.Shutdown(ctx); err != nil {
			app.logger.Error("管理服务器关闭错误", zap.Error(err))
			return err
		}
	}

	// 优雅关闭认证服务HTTP服务器
	if app.authServer != nil {
		app.logger.Info("正在关闭认证服务器...")
		if err := app.authServer.Shutdown(ctx); err != nil {
			app.logger.Error("认证服务器关闭错误", zap.Error(err))
			return err
		}
	}

	// 关闭数据库连接
	if app.dbService != nil {
		if err := app.dbService.Close(); err != nil {
			app.logger.Error("数据库关闭错误", zap.Error(err))
		}
	}

	// 关闭Redis连接
	if app.redisService != nil {
		if err := app.redisService.Close(); err != nil {
			app.logger.Error("Redis关闭错误", zap.Error(err))
		}
	}

	app.logger.Info("邮件网关服务器已停止")
	return nil
}

func main() {
	// 创建应用实例
	app, err := NewApp()
	if err != nil {
		fmt.Printf("Failed to create app: %v\n", err)
		os.Exit(1)
	}

	// 启动应用
	if err := app.Start(); err != nil {
		app.logger.Fatal("启动应用失败", zap.Error(err))
	}

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// 停止应用
	if err := app.Stop(); err != nil {
		app.logger.Error("优雅停止应用失败", zap.Error(err))
		os.Exit(1)
	}
}
