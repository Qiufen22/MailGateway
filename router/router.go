package router

import (
	"MailGateway/handlers"
	"MailGateway/middleware"
	"MailGateway/services"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// SetupRoutes 设置所有路由
func SetupRoutes(
	dbService *services.DatabaseService,
	totpService *services.TOTPService,
	authService *services.AuthService,
	jwtService *services.JWTService,
	redisService *services.RedisService,
	logger *zap.Logger,
) *gin.Engine {
	// 创建Gin引擎
	r := gin.New()

	// 添加中间件
	r.Use(middleware.RequestIDMiddleware())
	r.Use(middleware.ZapLogger(logger))
	r.Use(middleware.RecoveryMiddleware(logger))
	r.Use(middleware.ErrorHandlerMiddleware(logger))
	r.Use(middleware.CORSMiddleware())
	r.Use(middleware.SecurityHeadersMiddleware())

	// 创建处理器
	authHandler := handlers.NewAuthHandler(authService, dbService, logger)
	userHandler := handlers.NewUserHandler(dbService, totpService, redisService, logger)
	systemHandler := handlers.NewSystemHandler(authService, redisService, dbService, totpService, logger)
	jwtHandler := middleware.NewJWTHandler(jwtService, redisService, dbService, totpService, logger)

	// 注册API路由（不包含Nginx认证路由）
	registerAPIRoutes(r, authHandler, userHandler, systemHandler, jwtHandler)

	// 设置404和405处理器
	r.NoRoute(middleware.NotFoundHandler())
	r.NoMethod(middleware.MethodNotAllowedHandler())

	return r
}

// SetupAuthRoutes 设置独立的认证服务路由
func SetupAuthRoutes(
	dbService *services.DatabaseService,
	authService *services.AuthService,
	logger *zap.Logger,
) *gin.Engine {
	// 创建Gin引擎
	r := gin.New()

	// 添加中间件
	r.Use(middleware.RequestIDMiddleware())
	r.Use(middleware.ZapLogger(logger))
	r.Use(middleware.RecoveryMiddleware(logger))
	r.Use(middleware.ErrorHandlerMiddleware(logger))

	// 创建认证处理器
	authHandler := handlers.NewAuthHandler(authService, dbService, logger)

	// 注册Nginx认证路由
	registerNginxAuthRoutes(r, authHandler)

	// 设置404和405处理器
	r.NoRoute(middleware.NotFoundHandler())
	r.NoMethod(middleware.MethodNotAllowedHandler())

	return r
}

// registerNginxAuthRoutes 注册Nginx认证路由
func registerNginxAuthRoutes(r *gin.Engine, authHandler *handlers.AuthHandler) {
	// Nginx认证接口（移到api前缀下）
	r.GET("/api/auth", authHandler.NginxAuth)
	r.POST("/api/auth", authHandler.NginxAuth)
}

// registerAPIRoutes 注册API管理路由
func registerAPIRoutes(r *gin.Engine, authHandler *handlers.AuthHandler, userHandler *handlers.UserHandler, systemHandler *handlers.SystemHandler, jwtHandler *middleware.JWTHandler) {
	// 登录接口（无需认证，直接注册到根路径）
	r.POST("/login", jwtHandler.AdminLogin)

	// 刷新token接口（无需认证，直接注册到根路径）
	r.POST("/refresh", jwtHandler.RefreshToken)

	// API路由组
	api := r.Group("/api")
	{

		// 需要认证的路由组
		protected := api.Group("/")
		protected.Use(jwtHandler.JWTAuthMiddleware())
		{
			// 主页接口
			protected.GET("/dashboard", authHandler.Dashboard)

			// 登出接口（需要认证）
			protected.POST("/logout", jwtHandler.Logout)

			// 认证相关路由
			registerAuthRoutes(protected, authHandler)

			// 用户管理路由
			registerUserRoutes(protected, userHandler)

			// 系统管理路由
			registerSystemRoutes(protected, systemHandler)
		}

		// 静态文件服务
		r.Static("/static", "./web/static")
		r.StaticFile("/favicon.ico", "./web/favicon.ico")
		r.StaticFile("/logo.svg", "./web/logo.svg")
		r.StaticFile("/asset-manifest.json", "./web/asset-manifest.json")

		// 根路径重定向到登录页面
		r.GET("/", func(c *gin.Context) {
			c.Redirect(302, "/login")
		})

		// 前端路由处理
		r.GET("/login", func(c *gin.Context) {
			c.File("./web/index.html")
		})
		r.GET("/dashboard", func(c *gin.Context) {
			c.File("./web/index.html")
		})
		r.GET("/users", func(c *gin.Context) {
			c.File("./web/index.html")
		})
		r.GET("/system", func(c *gin.Context) {
			c.File("./web/index.html")
		})
	}
}

// registerAdminAuthRoutes 注册管理员认证相关路由（无需认证）
// 注意：此函数已废弃，相关路由已移至registerAPIRoutes中
func registerAdminAuthRoutes(api *gin.RouterGroup, jwtHandler *middleware.JWTHandler) {
	// 此函数已废弃，路由已重新组织
}

// registerAuthRoutes 注册认证相关路由（需要认证）
func registerAuthRoutes(api *gin.RouterGroup, authHandler *handlers.AuthHandler) {
	auth := api.Group("/auth")
	{
		// 管理接口（需要认证）
		auth.POST("/unlock", authHandler.UnlockUser)
		auth.GET("/user/:username/status", authHandler.GetUserStatus)

		// 认证日志相关路由
		auth.GET("/logs", authHandler.GetAuthLogs)
		auth.GET("/logs/stats", authHandler.GetAuthLogStats)
		auth.GET("/information", authHandler.GetAuthInformation)
	}

	// 仪表盘相关路由
	dashboard := api.Group("/dashboard")
	{
		dashboard.GET("/information", authHandler.GetDashboardInformation)
	}
}

// registerUserRoutes 注册用户管理路由
func registerUserRoutes(api *gin.RouterGroup, userHandler *handlers.UserHandler) {
	users := api.Group("/users")
	{
		// 基础用户操作
		users.GET("", userHandler.GetAllUsers)
		users.POST("", userHandler.CreateUser)
		users.PUT("", userHandler.UpdateUserPassword)
		users.GET("/:username", userHandler.GetUser)
		users.DELETE("", userHandler.DeleteUser)

		// TOTP相关路由
		registerTOTPRoutes(users, userHandler)
	}
}

// registerTOTPRoutes 注册TOTP相关路由
func registerTOTPRoutes(users *gin.RouterGroup, userHandler *handlers.UserHandler) {
	users.GET("/totp", userHandler.GetTOTPStatus)
	users.POST("/totp", userHandler.ManageTOTP)
	users.PUT("/totp", userHandler.ResetTOTP)
}

// registerSystemRoutes 注册系统管理路由
func registerSystemRoutes(api *gin.RouterGroup, systemHandler *handlers.SystemHandler) {
	sys := api.Group("/sys")
	{
		// 系统账户管理
		sys.POST("/modify", systemHandler.ModifyPassword)
		sys.POST("/edit", systemHandler.EditAccount)

		// 系统配置管理
		sys.GET("/authconfiguration", systemHandler.GetConfiguration)
		sys.POST("/authconfiguration", systemHandler.SaveConfiguration)
		sys.GET("/securityconfiguration", systemHandler.GetSecurityConfiguration)
		sys.POST("/securityconfiguration", systemHandler.SaveSecurityConfiguration)
		sys.GET("/smtp", systemHandler.GetSMTPConfiguration)
		sys.POST("/smtp", systemHandler.SaveSMTPConfiguration)
		// 管理员管理相关路由
		sys.POST("/add", systemHandler.AddAdmin)
		sys.POST("/disable", systemHandler.DisableAdmin)
		sys.DELETE("/delete", systemHandler.DeleteAdmin)
		sys.GET("/adminlist", systemHandler.GetAdminList)
		sys.PUT("/edit", systemHandler.EditAdmin)
		// IP黑名单管理
		sys.GET("/blacklist", systemHandler.GetBlacklist)
		sys.POST("/blacklist", systemHandler.AddBlacklist)
		sys.DELETE("/blacklist", systemHandler.DeleteBlacklist)
		// 操作日志管理
		sys.GET("/operationlogs", systemHandler.GetOperationLogs)
	}
}
