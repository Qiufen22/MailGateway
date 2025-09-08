package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"MailGateway/models"
	"MailGateway/services"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// JWTHandler JWT认证处理器
type JWTHandler struct {
	jwtService   *services.JWTService
	redisService *services.RedisService
	dbService    *services.DatabaseService
	totpService  *services.TOTPService
	logger       *zap.Logger
}

// NewJWTHandler 创建JWT处理器
func NewJWTHandler(jwtService *services.JWTService, redisService *services.RedisService, dbService *services.DatabaseService, totpService *services.TOTPService, logger *zap.Logger) *JWTHandler {
	return &JWTHandler{
		jwtService:   jwtService,
		redisService: redisService,
		dbService:    dbService,
		totpService:  totpService,
		logger:       logger,
	}
}

// AdminLogin 管理员登录
func (h *JWTHandler) AdminLogin(c *gin.Context) {
	var req models.AdminLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("登录请求参数错误", zap.Error(err))
		// 记录登录失败日志
		h.dbService.AddOperationLog("unknown", "admin", "系统登录", "login", "参数错误", c.ClientIP(), c.GetHeader("User-Agent"), "failed")
		c.JSON(http.StatusBadRequest, models.AdminLoginResponse{
			Success: false,
			Message: "请求参数错误",
		})
		return
	}

	// 获取管理员信息
	admin, err := h.dbService.GetAdmin(req.Username)
	if err != nil {
		h.logger.Error("获取管理员信息失败", zap.Error(err), zap.String("username", req.Username))
		c.JSON(http.StatusInternalServerError, models.AdminLoginResponse{
			Success: false,
			Message: "登录失败",
		})
		return
	}

	if admin == nil {
		h.logger.Warn("管理员不存在", zap.String("username", req.Username))
		// 记录登录失败日志
		h.dbService.AddOperationLog(req.Username, "admin", "系统登录", "login", "管理员不存在", c.ClientIP(), c.GetHeader("User-Agent"), "failed")
		c.JSON(http.StatusUnauthorized, models.AdminLoginResponse{
			Success: false,
			Message: "用户名或密码错误",
		})
		return
	}

	// 检查登录锁定策略
	lockEnabled, _ := h.dbService.GetSystemConfig("LOCKED")
	if lockEnabled == "true" {
		// 检查用户是否被锁定
		lockKey := "admin_lock:" + req.Username
		lockTimeStr, _ := h.redisService.Get(lockKey)
		if lockTimeStr != "" {
			// 用户被锁定，直接拒绝登录
			h.logger.Warn("管理员账户被锁定", zap.String("username", req.Username))
			h.dbService.AddOperationLog(req.Username, "admin", "系统登录", "login", "账户被锁定，拒绝登录", c.ClientIP(), c.GetHeader("User-Agent"), "failed")
			c.JSON(http.StatusUnauthorized, models.AdminLoginResponse{
				Success: false,
				Message: "登录失败",
			})
			return
		}
	}

	if !admin.Enabled {
		h.logger.Warn("管理员账户已禁用", zap.String("username", req.Username))
		// 记录登录失败日志
		h.dbService.AddOperationLog(req.Username, "admin", "系统登录", "login", "账户已禁用", c.ClientIP(), c.GetHeader("User-Agent"), "failed")
		c.JSON(http.StatusUnauthorized, models.AdminLoginResponse{
			Success: false,
			Message: "账户已禁用",
		})
		return
	}

	// 检查TOTP是否启用
	totpEnabledValue, _ := h.dbService.GetSystemConfig("TOTP")
	totpEnabled := totpEnabledValue == "true"

	// 只有在TOTP启用时才检查失败次数和触发TOTP验证
	if totpEnabled {
		// 检查是否已触发TOTP验证（在密码验证之前）
		failKey := "login_fail:" + req.Username
		failCountStr, _ := h.redisService.Get(failKey)
		failCount := 0
		if failCountStr != "" {
			if count, err := strconv.Atoi(failCountStr); err == nil {
				failCount = count
			}
		}

		// 获取TOTP失败次数阈值配置
		totpCountValue, _ := h.dbService.GetSystemConfig("TOTP_COUNT")
		totpThreshold := 3 // 默认值
		if totpCountValue != "" {
			if threshold, err := strconv.Atoi(totpCountValue); err == nil {
				totpThreshold = threshold
			}
		}

		// 如果已触发TOTP验证但未提供TOTP码，要求提供TOTP码
		if failCount >= totpThreshold && req.TOTPCode == "" {
			h.logger.Warn("用户已触发TOTP验证，需要提供TOTP验证码", zap.String("username", req.Username))
			// 记录登录失败日志
			h.dbService.AddOperationLog(req.Username, "admin", "系统登录", "login", "已触发TOTP验证，需要提供TOTP验证码", c.ClientIP(), c.GetHeader("User-Agent"), "failed")
			c.JSON(http.StatusUnauthorized, models.AdminLoginResponse{
				Success:     false,
				Message:     "请输入TOTP验证码",
				RequireTOTP: true,
			})
			return
		}
	}

	// 检查是否已触发TOTP验证（需要在密码验证前检查）
	failKey := "login_fail:" + req.Username
	failCountStr, _ := h.redisService.Get(failKey)
	failCount := 0
	if failCountStr != "" {
		if count, err := strconv.Atoi(failCountStr); err == nil {
			failCount = count
		}
	}

	// 获取TOTP失败次数阈值配置
	totpCountValue, _ := h.dbService.GetSystemConfig("TOTP_COUNT")
	totpThreshold := 3 // 默认值
	if totpCountValue != "" {
		if threshold, err := strconv.Atoi(totpCountValue); err == nil {
			totpThreshold = threshold
		}
	}

	// 判断是否已触发TOTP验证
	totpTriggered := totpEnabled && failCount >= totpThreshold

	// 验证密码
	err = bcrypt.CompareHashAndPassword([]byte(admin.Password), []byte(req.Password))
	passwordValid := err == nil

	// 如果已触发TOTP验证
	if totpTriggered {
		// 如果未提供TOTP码
		if req.TOTPCode == "" {
			h.logger.Warn("TOTP验证触发，需要提供验证码", zap.String("username", req.Username))
			h.dbService.AddOperationLog(req.Username, "admin", "系统登录", "login", "需要TOTP验证", c.ClientIP(), c.GetHeader("User-Agent"), "failed")
			c.JSON(http.StatusUnauthorized, models.AdminLoginResponse{
				Success:     false,
				Message:     "登录失败",
				RequireTOTP: true,
			})
			return
		}

		// 如果密码错误或TOTP验证失败，统一返回错误信息
		if !passwordValid {
			h.logger.Warn("TOTP验证状态下密码错误", zap.String("username", req.Username))
			h.dbService.AddOperationLog(req.Username, "admin", "系统登录", "login", "TOTP验证状态下密码错误", c.ClientIP(), c.GetHeader("User-Agent"), "failed")
			// 处理登录失败次数和锁定策略
			h.handleLoginFailure(req.Username, c)
			c.JSON(http.StatusUnauthorized, models.AdminLoginResponse{
				Success: false,
				Message: "登录失败",
			})
			return
		}

		// 验证TOTP代码
		valid, err := h.totpService.ValidateAdminTOTPCode(req.Username, req.TOTPCode)
		if err != nil || !valid {
			h.logger.Warn("TOTP验证失败", zap.String("username", req.Username))
			h.dbService.AddOperationLog(req.Username, "admin", "系统登录", "login", "TOTP验证失败", c.ClientIP(), c.GetHeader("User-Agent"), "failed")
			// 处理登录失败次数和锁定策略
			h.handleLoginFailure(req.Username, c)
			c.JSON(http.StatusUnauthorized, models.AdminLoginResponse{
				Success: false,
				Message: "登录失败",
			})
			return
		}

		// TOTP验证成功，清除失败次数
		h.redisService.Delete(failKey)
	} else {
		// 未触发TOTP验证的情况
		if !passwordValid {
			h.logger.Warn("密码验证失败",
				zap.String("username", req.Username),
				zap.String("client_ip", c.ClientIP()),
				zap.String("operation_type", "admin_login_failed"))

			// 记录登录失败日志
			h.dbService.AddOperationLog(req.Username, "admin", "系统登录", "login", "密码错误", c.ClientIP(), c.GetHeader("User-Agent"), "failed")

			// 处理登录失败次数和锁定策略
			h.handleLoginFailure(req.Username, c)

			// 只有在TOTP启用时才记录失败次数
			if totpEnabled {
				// 增加失败次数
				failCount++
				h.redisService.Set(failKey, strconv.Itoa(failCount), 24*time.Hour) // 24小时过期

				// 检查是否达到TOTP触发阈值
				if failCount >= totpThreshold {
					// 记录触发TOTP验证的日志
					h.dbService.AddOperationLog(req.Username, "admin", "系统登录", "login", "密码错误，触发TOTP验证", c.ClientIP(), c.GetHeader("User-Agent"), "failed")
					c.JSON(http.StatusUnauthorized, models.AdminLoginResponse{
						Success:     false,
						Message:     "需要TOTP验证",
						RequireTOTP: true,
					})
					return
				}
			}

			c.JSON(http.StatusUnauthorized, models.AdminLoginResponse{
				Success: false,
				Message: "用户名或密码错误",
			})
			return
		}
	}

	// 登录成功，清除失败次数记录
	h.redisService.Delete(failKey)
	// 清除锁定失败次数记录
	failCountKey := "login_fail_count:" + req.Username
	h.redisService.Delete(failCountKey)
	// 清除锁定记录
	lockKey := "admin_lock:" + req.Username
	h.redisService.Delete(lockKey)

	// 生成JWT token
	adminID := strconv.Itoa(admin.ID)
	accessToken, err := h.jwtService.GenerateToken(adminID, admin.Username, "admin")
	if err != nil {
		h.logger.Error("生成JWT token失败", zap.Error(err), zap.String("username", req.Username))
		c.JSON(http.StatusInternalServerError, models.AdminLoginResponse{
			Success: false,
			Message: "登录失败",
		})
		return
	}

	// 生成刷新token
	refreshToken, err := h.jwtService.GenerateRefreshToken(adminID, admin.Username, "admin")
	if err != nil {
		h.logger.Error("生成刷新token失败", zap.Error(err), zap.String("username", req.Username))
		c.JSON(http.StatusInternalServerError, models.AdminLoginResponse{
			Success: false,
			Message: "登录失败",
		})
		return
	}

	// 将token存储到Redis中
	tokenKey := "admin_token:" + adminID
	err = h.redisService.Set(tokenKey, accessToken, 24*time.Hour)
	if err != nil {
		h.logger.Error("存储token到Redis失败", zap.Error(err), zap.String("username", req.Username))
		c.JSON(http.StatusInternalServerError, models.AdminLoginResponse{
			Success: false,
			Message: "登录失败",
		})
		return
	}

	// 存储刷新token
	refreshKey := "admin_refresh:" + adminID
	err = h.redisService.Set(refreshKey, refreshToken, 7*24*time.Hour)
	if err != nil {
		h.logger.Error("存储刷新token到Redis失败", zap.Error(err), zap.String("username", req.Username))
		c.JSON(http.StatusInternalServerError, models.AdminLoginResponse{
			Success: false,
			Message: "登录失败",
		})
		return
	}

	// 记录登录成功日志
	h.logger.Info("管理员登录成功",
		zap.String("username", admin.Username),
		zap.String("admin_id", adminID),
		zap.String("client_ip", c.ClientIP()),
		zap.String("operation_type", "admin_login_success"))
	// 记录操作日志到数据库
	userType := h.dbService.GetUserTypeByUsername(admin.Username)
	h.dbService.AddOperationLog(admin.Username, userType, "系统登录", "login", "登录成功", c.ClientIP(), c.GetHeader("User-Agent"), "success")

	// 返回登录响应（只返回必要字段）
	c.JSON(http.StatusOK, models.AdminLoginResponse{
		Success:      true,
		Message:      "登录成功",
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	})
}

// RefreshToken 刷新JWT token
func (h *JWTHandler) RefreshToken(c *gin.Context) {
	// 从请求体获取刷新token
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "缺少刷新token",
		})
		return
	}

	// 验证刷新token
	claims, err := h.jwtService.ValidateToken(req.RefreshToken)
	if err != nil {
		h.logger.Warn("刷新token验证失败", zap.Error(err))
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "无效的刷新token",
		})
		return
	}

	// 检查Redis中的刷新token
	refreshKey := "admin_refresh:" + claims.UserID
	storedToken, err := h.redisService.Get(refreshKey)
	if err != nil || storedToken != req.RefreshToken {
		h.logger.Warn("刷新token不存在或已过期", zap.String("user_id", claims.UserID))
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "刷新token已过期",
		})
		return
	}

	// 生成新的访问token
	newAccessToken, err := h.jwtService.GenerateToken(claims.UserID, claims.Username, claims.UserType)
	if err != nil {
		h.logger.Error("生成新访问token失败", zap.Error(err), zap.String("user_id", claims.UserID))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "刷新token失败",
		})
		return
	}

	// 更新Redis中的访问token
	tokenKey := "admin_token:" + claims.UserID
	err = h.redisService.Set(tokenKey, newAccessToken, 24*time.Hour)
	if err != nil {
		h.logger.Error("更新Redis中的token失败", zap.Error(err), zap.String("user_id", claims.UserID))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "刷新token失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"message":      "token刷新成功",
		"access_token": newAccessToken,
	})
}

// JWTAuthMiddleware JWT认证中间件
func (h *JWTHandler) JWTAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从Authorization头获取token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			h.logger.Warn("缺少Authorization头", zap.String("path", c.Request.URL.Path))
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "缺少认证信息",
				"code":  "MISSING_AUTH",
			})
			c.Abort()
			return
		}

		// 检查Bearer前缀
		if !strings.HasPrefix(authHeader, "Bearer ") {
			h.logger.Warn("无效的Authorization格式", zap.String("path", c.Request.URL.Path))
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "无效的认证格式",
				"code":  "INVALID_AUTH_FORMAT",
			})
			c.Abort()
			return
		}

		// 提取token
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == "" {
			h.logger.Warn("空的token", zap.String("path", c.Request.URL.Path))
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "空的认证token",
				"code":  "EMPTY_TOKEN",
			})
			c.Abort()
			return
		}

		// 验证token
		claims, err := h.jwtService.ValidateToken(tokenString)
		if err != nil {
			h.logger.Warn("token验证失败",
				zap.Error(err),
				zap.String("path", c.Request.URL.Path))
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "无效的认证token",
				"code":  "INVALID_TOKEN",
			})
			c.Abort()
			return
		}

		// 检查Redis中的token
		tokenKey := "admin_token:" + claims.UserID
		storedToken, err := h.redisService.Get(tokenKey)
		if err != nil || storedToken != tokenString {
			h.logger.Warn("token不存在或已过期", zap.String("user_id", claims.UserID))
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "token已过期",
				"code":  "TOKEN_EXPIRED",
			})
			c.Abort()
			return
		}

		// 将用户信息存储到上下文中
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("userType", claims.UserType)
		c.Set("claims", claims)

		// 记录认证成功日志
		h.logger.Debug("用户认证成功",
			zap.String("username", claims.Username),
			zap.String("user_id", claims.UserID),
			zap.String("path", c.Request.URL.Path),
			zap.String("operation_type", "auth_success"))

		c.Next()
	}
}

// Logout 登出
func (h *JWTHandler) Logout(c *gin.Context) {
	// 从上下文获取用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "未认证",
		})
		return
	}

	userIDStr := userID.(string)

	// 删除Redis中的token
	tokenKey := "admin_token:" + userIDStr
	refreshKey := "admin_refresh:" + userIDStr

	err := h.redisService.Delete(tokenKey)
	if err != nil {
		h.logger.Error("删除访问token失败", zap.Error(err), zap.String("user_id", userIDStr))
	}

	err = h.redisService.Delete(refreshKey)
	if err != nil {
		h.logger.Error("删除刷新token失败", zap.Error(err), zap.String("user_id", userIDStr))
	}

	// 记录登出日志
	h.logger.Info("管理员登出成功",
		zap.String("user_id", userIDStr),
		zap.String("client_ip", c.ClientIP()),
		zap.String("operation_type", "admin_logout"))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "登出成功",
	})
}

// GetUserFromContext 从上下文中获取用户信息
func GetUserFromContext(c *gin.Context) (userID, username string, exists bool) {
	userIDInterface, userIDExists := c.Get("user_id")
	usernameInterface, usernameExists := c.Get("username")

	if !userIDExists || !usernameExists {
		return "", "", false
	}

	userID, userIDOk := userIDInterface.(string)
	username, usernameOk := usernameInterface.(string)

	if !userIDOk || !usernameOk {
		return "", "", false
	}

	return userID, username, true
}

// GetClaimsFromContext 从context中获取Claims
func GetClaimsFromContext(c *gin.Context) (*services.Claims, bool) {
	claims, exists := c.Get("claims")
	if !exists {
		return nil, false
	}
	claimsObj, ok := claims.(*services.Claims)
	return claimsObj, ok
}

// GetUserTypeFromContext 从context中获取用户类型
func GetUserTypeFromContext(c *gin.Context) (string, bool) {
	userType, exists := c.Get("userType")
	if !exists {
		return "", false
	}
	userTypeStr, ok := userType.(string)
	return userTypeStr, ok
}

// handleLoginFailure 处理登录失败次数记录和锁定机制
func (h *JWTHandler) handleLoginFailure(username string, c *gin.Context) {
	// 检查登录锁定策略是否启用
	lockEnabled, _ := h.dbService.GetSystemConfig("LOCKED")
	if lockEnabled != "true" {
		return
	}

	// 获取失败次数阈值配置
	lockErrorsValue, _ := h.dbService.GetSystemConfig("LOCK_ERRORS")
	failThreshold := 5 // 默认值
	if lockErrorsValue != "" {
		if threshold, err := strconv.Atoi(lockErrorsValue); err == nil {
			failThreshold = threshold
		}
	}

	// 获取锁定时间配置
	lockTimeValue, _ := h.dbService.GetSystemConfig("LOCK_TIME")
	lockDuration := 30 * time.Minute // 默认30分钟
	if lockTimeValue != "" {
		if minutes, err := strconv.Atoi(lockTimeValue); err == nil {
			lockDuration = time.Duration(minutes) * time.Minute
		}
	}

	// 记录失败次数
	failKey := "login_fail_count:" + username
	failCountStr, _ := h.redisService.Get(failKey)
	failCount := 1
	if failCountStr != "" {
		if count, err := strconv.Atoi(failCountStr); err == nil {
			failCount = count + 1
		}
	}

	// 设置失败次数，24小时过期
	h.redisService.Set(failKey, strconv.Itoa(failCount), 24*time.Hour)

	// 检查是否达到锁定阈值
	if failCount >= failThreshold {
		// 锁定用户
		lockKey := "admin_lock:" + username
		h.redisService.Set(lockKey, "locked", lockDuration)

		h.logger.Warn("管理员账户因多次登录失败被锁定",
			zap.String("username", username),
			zap.Int("fail_count", failCount),
			zap.Duration("lock_duration", lockDuration))

		// 记录锁定日志
		h.dbService.AddOperationLog(username, "admin", "系统登录", "login",
			fmt.Sprintf("登录失败%d次，账户被锁定%d分钟", failCount, int(lockDuration.Minutes())),
			c.ClientIP(), c.GetHeader("User-Agent"), "failed")
	}
}
