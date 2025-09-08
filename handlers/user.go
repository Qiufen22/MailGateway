package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"MailGateway/middleware"
	"MailGateway/models"
	"MailGateway/services"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// UserHandler 用户管理处理器
type UserHandler struct {
	dbService    *services.DatabaseService
	totpService  *services.TOTPService
	redisService *services.RedisService
	logger       *zap.Logger
}

// NewUserHandler 创建用户处理器
func NewUserHandler(dbService *services.DatabaseService, totpService *services.TOTPService, redisService *services.RedisService, logger *zap.Logger) *UserHandler {
	return &UserHandler{
		dbService:    dbService,
		totpService:  totpService,
		redisService: redisService,
		logger:       logger,
	}
}

// GetUser 获取用户信息
func (h *UserHandler) GetUser(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "用户名为必填项",
		})
		return
	}

	user, err := h.dbService.GetUser(username)
	if err != nil {
		h.logger.Error("获取用户失败",
			zap.String("username", username),
			zap.Error(err))

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get user",
		})
		return
	}

	if user == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "User not found",
		})
		return
	}

	// 如果用户开启了TOTP且有TOTP密钥，生成二维码图片
	if user.TOTPEnabled && user.TOTPSecret != "" {
		qrCodeImage, err := h.totpService.GenerateQRCodeImage(user.Username, user.TOTPSecret)
		if err != nil {
			h.logger.Warn("生成TOTP二维码图片失败",
				zap.String("username", username),
				zap.Error(err))
		} else {
			user.TOTPQRCode = qrCodeImage
		}
	}

	c.JSON(http.StatusOK, user)
}

// GetAllUsers 获取所有用户列表（支持缓存、分页和搜索）
func (h *UserHandler) GetAllUsers(c *gin.Context) {
	// 获取分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	search := strings.TrimSpace(c.DefaultQuery("search", ""))

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}

	// 构建缓存键
	cacheKey := fmt.Sprintf("users:page:%d:limit:%d:search:%s", page, limit, search)

	// 尝试从缓存获取
	if cachedData, err := h.redisService.Get(cacheKey); err == nil {
		var response models.UserListResponse
		if json.Unmarshal([]byte(cachedData), &response) == nil {
			h.logger.Debug("从缓存获取用户列表", zap.String("cache_key", cacheKey))
			c.JSON(http.StatusOK, response)
			return
		}
	}

	// 从数据库获取用户列表
	users, total, err := h.dbService.GetUsersWithPagination(page, limit, search)
	if err != nil {
		h.logger.Error("获取用户列表失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "获取用户列表失败",
		})
		return
	}

	// 为开启TOTP的用户生成二维码图片
	for i := range users {
		if users[i].TOTPEnabled && users[i].TOTPSecret != "" {
			qrCodeImage, err := h.totpService.GenerateQRCodeImage(users[i].Username, users[i].TOTPSecret)
			if err != nil {
				h.logger.Warn("生成TOTP二维码图片失败",
					zap.String("username", users[i].Username),
					zap.Error(err))
			} else {
				users[i].TOTPQRCode = qrCodeImage
			}
		}
	}

	// 构建响应
	response := models.UserListResponse{
		Users: users,
		Total: total,
		Page:  page,
		Limit: limit,
	}

	// 缓存结果（5分钟过期）
	if responseData, err := json.Marshal(response); err == nil {
		if err := h.redisService.Set(cacheKey, string(responseData), 5*time.Minute); err != nil {
			h.logger.Warn("缓存用户列表失败", zap.Error(err))
		}
	}

	h.logger.Info("获取用户列表成功",
		zap.Int("page", page),
		zap.Int("limit", limit),
		zap.String("search", search),
		zap.Int("total", total),
		zap.Int("returned", len(users)))

	c.JSON(http.StatusOK, response)
}

// clearUserListCache 清理用户列表缓存
func (h *UserHandler) clearUserListCache() {
	// 清理所有用户列表相关的缓存
	pattern := "users:page:*"
	if err := h.redisService.Delete(pattern); err != nil {
		h.logger.Warn("清理用户列表缓存失败", zap.Error(err))
	}
}

// CreateUser 创建用户
func (h *UserHandler) CreateUser(c *gin.Context) {
	var req models.UserCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "参数格式错误",
		})
		return
	}

	// 检查用户是否已存在
	existingUser, err := h.dbService.GetUser(req.Username)
	if err != nil {
		h.logger.Error("检查用户是否存在失败",
			zap.String("username", req.Username),
			zap.Error(err))

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "检查用户失败",
		})
		return
	}

	if existingUser != nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "用户已存在",
		})
		return
	}

	// 创建用户
	err = h.dbService.CreateUser(&req)
	if err != nil {
		h.logger.Error("创建用户失败",
			zap.String("username", req.Username),
			zap.Error(err))

		// 获取用户类型
		userType, _ := middleware.GetUserTypeFromContext(c)
		if userType == "" {
			userType = "unknown" // 默认值
		}

		// 记录创建用户失败日志
		h.dbService.AddOperationLog("", userType, "create_user", "user_management",
			fmt.Sprintf("创建用户失败: %s", req.Username), c.ClientIP(), c.GetHeader("User-Agent"), "failed")

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "创建用户失败",
		})
		return
	}

	// 如果启用了TOTP，为用户生成TOTP密钥
	if strings.ToLower(req.TOTPEnabled) == "true" || req.TOTPEnabled == "1" {
		// 生成TOTP密钥
		secret, err := h.totpService.GenerateTOTPSecret()
		if err != nil {
			h.logger.Error("生成TOTP密钥失败",
				zap.String("username", req.Username),
				zap.Error(err))
			// 不返回错误，只记录日志，用户创建成功但TOTP未启用
		} else {
			// 更新用户的TOTP密钥
			err = h.dbService.UpdateTOTPSecret(req.Username, secret)
			if err != nil {
				h.logger.Error("设置TOTP密钥失败",
					zap.String("username", req.Username),
					zap.Error(err))
				// 不返回错误，只记录日志
			} else {
				h.logger.Info("用户TOTP密钥生成成功",
					zap.String("username", req.Username))
			}
		}
	}

	// 获取操作者用户名和类型
	operatorUsername, _ := c.Get("username")
	var userType string
	if operatorUsername != nil {
		userType = h.dbService.GetUserTypeByUsername(operatorUsername.(string))
	} else {
		userType = "unknown"
	}

	// 记录创建用户成功日志
	h.dbService.AddOperationLog("", userType, "create_user", "user_management",
		fmt.Sprintf("创建用户: %s, 所有者: %s, TOTP: %s", req.Username, req.Owner, req.TOTPEnabled),
		c.ClientIP(), c.GetHeader("User-Agent"), "success")

	// 清理用户列表缓存
	h.clearUserListCache()

	h.logger.Info("用户创建成功",
		zap.String("username", req.Username),
		zap.String("owner", req.Owner),
		zap.String("totp_enabled", req.TOTPEnabled))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "用户创建成功",
	})
}

// UpdateUserPassword 修改用户密码
func (h *UserHandler) UpdateUserPassword(c *gin.Context) {
	// 验证JWT认证（确保是管理员身份）
	_, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	var updateData struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
		return
	}

	// 验证必填字段
	if updateData.Username == "" || updateData.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名和密码不能为空"})
		return
	}

	// 验证密码长度
	if len(updateData.Password) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "密码长度至少6位"})
		return
	}

	// 检查要修改的用户是否存在
	targetUser, err := h.dbService.GetUser(updateData.Username)
	if err != nil {
		h.logger.Error("获取用户信息失败", zap.String("username", updateData.Username), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库查询失败"})
		return
	}
	if targetUser == nil {
		h.logger.Warn("要修改的用户不存在", zap.String("username", updateData.Username))
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}

	// 更新用户密码
	req := &models.UserUpdateRequest{
		Password: updateData.Password,
	}
	err = h.dbService.UpdateUser(updateData.Username, req)
	if err != nil {
		h.logger.Error("修改密码失败",
			zap.String("username", updateData.Username),
			zap.Error(err))

		// 获取用户类型
		userType, _ := middleware.GetUserTypeFromContext(c)
		if userType == "" {
			userType = "unknown" // 默认值
		}

		// 记录修改密码失败日志
		h.dbService.AddOperationLog("", userType, "update_password", "user_management",
			fmt.Sprintf("修改用户密码失败: %s", updateData.Username), c.ClientIP(), c.GetHeader("User-Agent"), "failed")

		c.JSON(http.StatusInternalServerError, gin.H{"error": "修改密码失败"})
		return
	}

	// 获取用户类型
	userType := h.dbService.GetUserTypeByUsername(updateData.Username)

	// 记录修改密码成功日志
	h.dbService.AddOperationLog("", userType, "update_password", "user_management",
		fmt.Sprintf("修改用户密码: %s", updateData.Username), c.ClientIP(), c.GetHeader("User-Agent"), "success")

	// 清理用户列表缓存
	h.clearUserListCache()

	h.logger.Info("密码修改成功", zap.String("username", updateData.Username))
	c.JSON(http.StatusOK, gin.H{"message": "密码修改成功", "success": true})
}

// DeleteUser 删除用户
func (h *UserHandler) DeleteUser(c *gin.Context) {
	// 验证JWT认证（确保是管理员身份）
	_, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	var deleteData struct {
		Username string `json:"username"`
	}

	if err := c.ShouldBindJSON(&deleteData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
		return
	}

	if deleteData.Username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名不能为空"})
		return
	}

	// 检查用户是否存在
	existingUser, err := h.dbService.GetUser(deleteData.Username)
	if err != nil {
		h.logger.Error("检查用户是否存在失败",
			zap.String("username", deleteData.Username),
			zap.Error(err))

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "数据库查询失败",
		})
		return
	}

	if existingUser == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "用户不存在",
		})
		return
	}

	// 删除用户
	err = h.dbService.DeleteUser(deleteData.Username)
	if err != nil {
		h.logger.Error("删除用户失败",
			zap.String("username", deleteData.Username),
			zap.Error(err))

		// 获取用户类型
		userType, _ := middleware.GetUserTypeFromContext(c)
		if userType == "" {
			userType = "admin" // 默认值
		}

		// 记录删除用户失败日志
		h.dbService.AddOperationLog("", userType, "delete_user", "user_management",
			fmt.Sprintf("删除用户失败: %s", deleteData.Username), c.ClientIP(), c.GetHeader("User-Agent"), "failed")

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "删除用户失败",
		})
		return
	}

	// 获取用户类型
	userType := h.dbService.GetUserTypeByUsername(deleteData.Username)

	// 记录删除用户成功日志
	h.dbService.AddOperationLog("", userType, "用户管理", "user_management",
		fmt.Sprintf("删除用户: %s", deleteData.Username), c.ClientIP(), c.GetHeader("User-Agent"), "success")

	// 清理用户列表缓存
	h.clearUserListCache()

	h.logger.Info("用户删除成功", zap.String("username", deleteData.Username))
	c.JSON(http.StatusOK, gin.H{
		"message": "用户删除成功",
		"success": true,
	})
}

// EnableTOTP 为用户启用TOTP

// GetTOTPStatus 获取用户TOTP状态
func (h *UserHandler) GetTOTPStatus(c *gin.Context) {
	username := c.Query("username")
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Username is required",
		})
		return
	}

	status, err := h.totpService.GetTOTPStatus(username)
	if err != nil {
		h.logger.Error("获取TOTP状态失败",
			zap.String("username", username),
			zap.Error(err))

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取TOTP状态失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"username": username,
		"status":   status.Valid,
	})
}

// ManageTOTP 管理用户TOTP（开启/关闭）
func (h *UserHandler) ManageTOTP(c *gin.Context) {
	var req struct {
		Username    string `json:"username" binding:"required"`
		TOTPEnabled bool   `json:"totp_enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "用户名为必填项",
		})
		return
	}

	if req.TOTPEnabled {
		// 开启TOTP
		_, _, err := h.totpService.EnableTOTPForUser(c, req.Username)
		if err != nil {
			h.logger.Error("启用TOTP失败",
				zap.String("username", req.Username),
				zap.Error(err))

			// 获取操作者信息
			_, operatorUsername, _ := middleware.GetUserFromContext(c)
			userType, _ := middleware.GetUserTypeFromContext(c)
			if userType == "" {
				userType = "admin" // 默认值
			}

			// 记录开启TOTP失败日志
			h.dbService.AddOperationLog(operatorUsername, userType, "enable_totp", "user_management",
				fmt.Sprintf("开启用户TOTP失败: %s", req.Username), c.ClientIP(), c.GetHeader("User-Agent"), "failed")

			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "TOTP开启失败",
			})
			return
		}

		// 获取操作者信息
		_, operatorUsername, _ := middleware.GetUserFromContext(c)
		userType, _ := middleware.GetUserTypeFromContext(c)
		if userType == "" {
			userType = "admin" // 默认值
		}

		// 记录开启TOTP成功日志
		h.dbService.AddOperationLog(operatorUsername, userType, "enable_totp", "user_management",
			fmt.Sprintf("开启用户TOTP: %s", req.Username), c.ClientIP(), c.GetHeader("User-Agent"), "success")

		// 清理用户列表缓存
		h.clearUserListCache()

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "TOTP开启成功",
		})
	} else {
		// 关闭TOTP
		err := h.totpService.DisableTOTPForUser(c, req.Username)
		if err != nil {
			h.logger.Error("禁用TOTP失败",
				zap.String("username", req.Username),
				zap.Error(err))

			// 获取操作者信息
			_, operatorUsername, _ := middleware.GetUserFromContext(c)
			userType, _ := middleware.GetUserTypeFromContext(c)
			if userType == "" {
				userType = "admin" // 默认值
			}

			// 记录关闭TOTP失败日志
			h.dbService.AddOperationLog(operatorUsername, userType, "disable_totp", "user_management",
				fmt.Sprintf("关闭用户TOTP失败: %s", req.Username), c.ClientIP(), c.GetHeader("User-Agent"), "failed")

			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "TOTP关闭失败",
			})
			return
		}

		// 获取操作者信息
		_, operatorUsername, _ := middleware.GetUserFromContext(c)
		userType, _ := middleware.GetUserTypeFromContext(c)
		if userType == "" {
			userType = "admin" // 默认值
		}

		// 记录关闭TOTP成功日志
		h.dbService.AddOperationLog(operatorUsername, userType, "disable_totp", "user_management",
			fmt.Sprintf("关闭用户TOTP: %s", req.Username), c.ClientIP(), c.GetHeader("User-Agent"), "success")

		// 清理用户列表缓存
		h.clearUserListCache()

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "TOTP关闭成功",
		})
	}
}

// ResetTOTP 重置用户TOTP
func (h *UserHandler) ResetTOTP(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "用户名为必填项",
		})
		return
	}

	// 先检查用户TOTP状态
	totpStatus, err := h.totpService.GetTOTPStatus(req.Username)
	if err != nil {
		h.logger.Error("获取TOTP状态失败",
			zap.String("username", req.Username),
			zap.Error(err))

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取TOTP状态失败",
		})
		return
	}

	// 如果TOTP未开启，返回false
	if !totpStatus.Valid {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "用户未开启TOTP",
		})
		return
	}

	// 重置TOTP secret
	_, _, err = h.totpService.UpdateTOTPForUser(c, req.Username)
	if err != nil {
		h.logger.Error("重置TOTP失败",
			zap.String("username", req.Username),
			zap.Error(err))

		// 获取操作者信息
		_, operatorUsername, _ := middleware.GetUserFromContext(c)
		userType, _ := middleware.GetUserTypeFromContext(c)
		if userType == "" {
			userType = "admin" // 默认值
		}

		// 记录重置TOTP失败日志
		h.dbService.AddOperationLog(operatorUsername, userType, "reset_totp", "user_management",
			fmt.Sprintf("重置用户TOTP失败: %s", req.Username), c.ClientIP(), c.GetHeader("User-Agent"), "failed")

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "重置TOTP失败",
		})
		return
	}

	// 获取操作者信息
	_, operatorUsername, _ := middleware.GetUserFromContext(c)
	userType, _ := middleware.GetUserTypeFromContext(c)
	if userType == "" {
		userType = "admin" // 默认值
	}

	// 记录重置TOTP成功日志
	h.dbService.AddOperationLog(operatorUsername, userType, "reset_totp", "user_management",
		fmt.Sprintf("重置用户TOTP: %s", req.Username), c.ClientIP(), c.GetHeader("User-Agent"), "success")

	// 清理用户列表缓存
	h.clearUserListCache()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "TOTP重置成功",
	})
}
