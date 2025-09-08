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

// SystemHandler 系统账户处理器
type SystemHandler struct {
	authService  *services.AuthService
	redisService *services.RedisService
	dbService    *services.DatabaseService
	totpService  *services.TOTPService
	logger       *zap.Logger
}

// NewSystemHandler 创建系统账户处理器
func NewSystemHandler(authService *services.AuthService, redisService *services.RedisService, dbService *services.DatabaseService, totpService *services.TOTPService, logger *zap.Logger) *SystemHandler {
	return &SystemHandler{
		authService:  authService,
		redisService: redisService,
		dbService:    dbService,
		totpService:  totpService,
		logger:       logger,
	}
}

// ModifyPassword 修改系统账户密码（专门用于超级管理员ID=1的密码修改）
func (h *SystemHandler) ModifyPassword(c *gin.Context) {
	var req models.SystemPasswordModifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error("参数绑定失败", zap.Error(err))
		c.JSON(http.StatusBadRequest, models.SystemAccountResponse{
			Success: false,
			Message: "参数格式错误: " + err.Error(),
		})
		return
	}

	// 验证新密码和重复密码是否一致
	if req.NewPassword != req.RepeatPassword {
		c.JSON(http.StatusBadRequest, models.SystemAccountResponse{
			Success: false,
			Message: "新密码和重复密码不一致",
		})
		return
	}

	// 验证密码强度
	if len(req.NewPassword) < 6 {
		c.JSON(http.StatusBadRequest, models.SystemAccountResponse{
			Success: false,
			Message: "新密码长度不能少于6位",
		})
		return
	}

	// 从JWT中获取当前用户信息
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.SystemAccountResponse{
			Success: false,
			Message: "未认证",
		})
		return
	}

	// 获取当前管理员信息，验证是否为超级管理员
	admin, err := h.authService.GetAdminByUsername(username.(string))
	if err != nil {
		h.logger.Error("获取管理员信息失败", zap.Error(err), zap.String("username", username.(string)))
		c.JSON(http.StatusBadRequest, models.SystemAccountResponse{
			Success: false,
			Message: "获取管理员信息失败",
		})
		return
	}

	// 验证是否为超级管理员（ID=1）
	if admin.ID != 1 {
		h.logger.Warn("非超级管理员尝试修改系统密码", zap.String("username", username.(string)), zap.Int("admin_id", admin.ID))
		c.JSON(http.StatusForbidden, models.SystemAccountResponse{
			Success: false,
			Message: "只有超级管理员才能使用此接口修改密码",
		})
		return
	}

	// 修改密码
	err = h.authService.ModifyAdminPassword(username.(string), req.OldPassword, req.NewPassword)
	if err != nil {
		h.logger.Error("超级管理员密码修改失败", zap.Error(err), zap.String("username", username.(string)))

		// 获取用户类型
		userType, _ := middleware.GetUserTypeFromContext(c)
		if userType == "" {
			userType = "admin" // 默认值
		}

		// 记录失败的操作日志
		h.dbService.AddOperationLog(
			username.(string),
			userType,
			"密码修改",
			"系统账户",
			fmt.Sprintf("超级管理员密码修改失败: %s", err.Error()),
			c.ClientIP(),
			c.Request.UserAgent(),
			"failed",
		)

		c.JSON(http.StatusBadRequest, models.SystemAccountResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	h.logger.Info("超级管理员密码修改成功", zap.String("username", username.(string)))

	// 获取用户类型
	userType, _ := middleware.GetUserTypeFromContext(c)
	if userType == "" {
		userType = "admin" // 默认值
	}

	// 记录成功的操作日志
	h.dbService.AddOperationLog(
		username.(string),
		userType,
		"密码修改",
		"系统账户",
		"超级管理员密码修改成功",
		c.ClientIP(),
		c.Request.UserAgent(),
		"success",
	)

	c.JSON(http.StatusOK, models.SystemAccountResponse{
		Success: true,
		Message: "超级管理员密码修改成功",
	})
}

// EditAccount 编辑系统账户
func (h *SystemHandler) EditAccount(c *gin.Context) {
	var req models.SystemAccountEditRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error("参数绑定失败", zap.Error(err))
		c.JSON(http.StatusBadRequest, models.SystemAccountResponse{
			Success: false,
			Message: "参数格式错误: " + err.Error(),
		})
		return
	}

	// 验证操作类型
	validActions := []string{"add", "modify", "delete", "disable"}
	validAction := false
	for _, action := range validActions {
		if req.Action == action {
			validAction = true
			break
		}
	}
	if !validAction {
		c.JSON(http.StatusBadRequest, models.SystemAccountResponse{
			Success: false,
			Message: "无效的操作类型，支持的操作: add, modify, delete, disable",
		})
		return
	}

	// 验证用户名
	if strings.TrimSpace(req.Username) == "" {
		c.JSON(http.StatusBadRequest, models.SystemAccountResponse{
			Success: false,
			Message: "用户名不能为空",
		})
		return
	}

	// 根据操作类型执行相应操作
	var result *models.Admin
	var err error

	switch req.Action {
	case "add":
		if req.Password == "" {
			c.JSON(http.StatusBadRequest, models.SystemAccountResponse{
				Success: false,
				Message: "添加账户时密码不能为空",
			})
			return
		}
		if len(req.Password) < 6 {
			c.JSON(http.StatusBadRequest, models.SystemAccountResponse{
				Success: false,
				Message: "密码长度不能少于6位",
			})
			return
		}
		result, err = h.authService.CreateAdmin(req.Username, req.Password, req.Enabled)
	case "modify":
		if req.Password != "" && len(req.Password) < 6 {
			c.JSON(http.StatusBadRequest, models.SystemAccountResponse{
				Success: false,
				Message: "密码长度不能少于6位",
			})
			return
		}
		result, err = h.authService.UpdateAdmin(req.Username, req.Password, req.Enabled)
	case "delete":
		err = h.authService.DeleteAdmin(req.Username)
	case "disable":
		enabled := false
		result, err = h.authService.UpdateAdmin(req.Username, "", &enabled)
	}

	if err != nil {
		h.logger.Error("账户操作失败", zap.Error(err), zap.String("action", req.Action), zap.String("username", req.Username))
		c.JSON(http.StatusBadRequest, models.SystemAccountResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	var message string
	switch req.Action {
	case "add":
		message = "账户添加成功"
	case "modify":
		message = "账户修改成功"
	case "delete":
		message = "账户删除成功"
	case "disable":
		message = "账户禁用成功"
	}

	h.logger.Info("账户操作成功", zap.String("action", req.Action), zap.String("username", req.Username))
	c.JSON(http.StatusOK, models.SystemAccountResponse{
		Success: true,
		Message: message,
		Data:    result,
	})
}

// SaveConfiguration 保存系统配置到Redis
func (h *SystemHandler) SaveConfiguration(c *gin.Context) {
	var req models.SystemConfigSaveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error("参数绑定失败", zap.Error(err))
		c.JSON(http.StatusBadRequest, models.SystemConfigSaveResponse{
			Success: false,
			Message: "参数格式错误: " + err.Error(),
		})
		return
	}

	// 验证配置参数
	if strings.TrimSpace(req.HTTPHTTPS) == "" {
		c.JSON(http.StatusBadRequest, models.SystemConfigSaveResponse{
			Success: false,
			Message: "HTTP/HTTPS配置不能为空",
		})
		return
	}

	if strings.TrimSpace(req.POP3) == "" {
		c.JSON(http.StatusBadRequest, models.SystemConfigSaveResponse{
			Success: false,
			Message: "POP3配置不能为空",
		})
		return
	}

	if strings.TrimSpace(req.IMAP) == "" {
		c.JSON(http.StatusBadRequest, models.SystemConfigSaveResponse{
			Success: false,
			Message: "IMAP配置不能为空",
		})
		return
	}

	// 将配置转换为JSON格式保存到Redis
	configData := map[string]string{
		"HTTP/HTTPS": req.HTTPHTTPS,
		"POP3":       req.POP3,
		"IMAP":       req.IMAP,
		"updated_at": time.Now().Format(time.RFC3339),
	}

	configJSON, err := json.Marshal(configData)
	if err != nil {
		h.logger.Error("配置序列化失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.SystemConfigSaveResponse{
			Success: false,
			Message: "配置序列化失败",
		})
		return
	}

	// 保存到Redis，设置永不过期
	err = h.redisService.Set("system:configuration", string(configJSON), 0)
	if err != nil {
		h.logger.Error("保存系统配置到Redis失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.SystemConfigSaveResponse{
			Success: false,
			Message: "保存配置到Redis失败: " + err.Error(),
		})
		return
	}

	// 同时保存到MySQL数据库
	err = h.dbService.SaveSystemConfig("HTTP/HTTPS", req.HTTPHTTPS)
	if err != nil {
		h.logger.Error("保存HTTP/HTTPS配置到数据库失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.SystemConfigSaveResponse{
			Success: false,
			Message: "保存HTTP/HTTPS配置到数据库失败: " + err.Error(),
		})
		return
	}

	err = h.dbService.SaveSystemConfig("POP3", req.POP3)
	if err != nil {
		h.logger.Error("保存POP3配置到数据库失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.SystemConfigSaveResponse{
			Success: false,
			Message: "保存POP3配置到数据库失败: " + err.Error(),
		})
		return
	}

	err = h.dbService.SaveSystemConfig("IMAP", req.IMAP)
	if err != nil {
		h.logger.Error("保存IMAP配置到数据库失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.SystemConfigSaveResponse{
			Success: false,
			Message: "保存IMAP配置到数据库失败: " + err.Error(),
		})
		return
	}

	h.logger.Info("系统配置保存成功",
		zap.String("HTTP/HTTPS", req.HTTPHTTPS),
		zap.String("POP3", req.POP3),
		zap.String("IMAP", req.IMAP))

	// 记录操作日志
	operatorUsername, _ := c.Get("username")
	if operatorUsername != nil {
		userType, _ := middleware.GetUserTypeFromContext(c)
		if userType == "" {
			userType = "unknown"
		}
		h.dbService.AddOperationLog(
			operatorUsername.(string),
			userType,
			"保存系统配置",
			"系统配置",
			"保存HTTP/HTTPS、POP3、IMAP配置",
			c.ClientIP(),
			c.Request.UserAgent(),
			"成功",
		)
	}

	c.JSON(http.StatusOK, models.SystemConfigSaveResponse{
		Success: true,
		Message: "系统配置保存成功",
	})
}

// GetConfiguration 获取系统配置信息
func (h *SystemHandler) GetConfiguration(c *gin.Context) {
	// 记录请求日志
	h.logger.Info("收到获取系统配置请求")

	// 从数据库获取配置信息
	configs, err := h.dbService.GetAllSystemConfigs()
	if err != nil {
		h.logger.Error("获取系统配置失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.AuthConfigGetResponse{
			Code:    500,
			Message: "获取系统配置失败",
		})
		return
	}

	// 将配置转换为分组格式，与POST请求保持一致
	configData := map[string]string{
		"HTTP/HTTPS": "",
		"POP3":       "",
		"IMAP":       "",
	}

	// 从数据库配置中填充对应的值
	for _, config := range configs {
		switch config.ConfigKey {
		case "HTTP/HTTPS":
			configData["HTTP/HTTPS"] = config.ConfigValue
		case "POP3":
			configData["POP3"] = config.ConfigValue
		case "IMAP":
			configData["IMAP"] = config.ConfigValue
		}
	}

	h.logger.Info("系统配置获取成功")
	c.JSON(http.StatusOK, models.AuthConfigGetResponse{
		Code:    200,
		Message: "获取系统配置成功",
		Data:    configData,
	})
}

// AddAdmin 添加管理员用户
func (h *SystemHandler) AddAdmin(c *gin.Context) {
	// 权限检查：只有超级管理员（ID为1）才能添加管理员
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.AdminAddResponse{
			Success: false,
			Message: "未认证",
		})
		return
	}

	// 检查是否为超级管理员
	if userID != "1" {
		h.logger.Warn("非超级管理员尝试添加管理员", zap.String("user_id", userID.(string)))
		c.JSON(http.StatusForbidden, models.AdminAddResponse{
			Success: false,
			Message: "权限不足",
		})
		return
	}

	var req models.AdminAddRequest

	// 绑定请求参数
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error("添加管理员请求参数绑定失败", zap.Error(err))
		c.JSON(http.StatusBadRequest, models.AdminAddResponse{
			Success: false,
			Message: "请求参数格式错误",
		})
		return
	}

	// 验证用户名和密码
	if strings.TrimSpace(req.Username) == "" {
		c.JSON(http.StatusBadRequest, models.AdminAddResponse{
			Success: false,
			Message: "用户名不能为空",
		})
		return
	}

	if strings.TrimSpace(req.Password) == "" {
		c.JSON(http.StatusBadRequest, models.AdminAddResponse{
			Success: false,
			Message: "密码不能为空",
		})
		return
	}

	// 检查用户名是否已存在
	existingAdmin, _ := h.dbService.GetAdminByUsername(req.Username)
	if existingAdmin != nil {
		c.JSON(http.StatusConflict, models.AdminAddResponse{
			Success: false,
			Message: "用户名已存在",
		})
		return
	}

	// 创建管理员
	adminReq := &models.AdminCreateRequest{
		Username: req.Username,
		Password: req.Password,
		Enabled:  &[]bool{true}[0],
	}
	err := h.dbService.CreateAdmin(adminReq)
	if err != nil {
		h.logger.Error("创建管理员失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.AdminAddResponse{
			Success: false,
			Message: "创建管理员失败: " + err.Error(),
		})
		return
	}

	// 获取创建的管理员信息
	admin, err := h.dbService.GetAdminByUsername(req.Username)
	if err != nil {
		h.logger.Error("获取创建的管理员信息失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.AdminAddResponse{
			Success: false,
			Message: "获取管理员信息失败",
		})
		return
	}

	h.logger.Info("管理员创建成功",
		zap.String("username", req.Username),
		zap.Bool("enabled", admin.Enabled))

	// 记录操作日志
	operatorUsername, _ := c.Get("username")
	userType, _ := middleware.GetUserTypeFromContext(c)
	if userType == "" {
		userType = "admin"
	}
	h.dbService.AddOperationLog(
		operatorUsername.(string),
		userType,
		"添加管理员",
		"管理员账户",
		fmt.Sprintf("添加管理员: %s", req.Username),
		c.ClientIP(),
		c.Request.UserAgent(),
		"成功",
	)

	c.JSON(http.StatusOK, models.AdminAddResponse{
		Success: true,
		Message: "创建成功",
	})
}

// DisableAdmin 禁用/启用管理员用户
func (h *SystemHandler) DisableAdmin(c *gin.Context) {
	// 权限检查：只有超级管理员（ID为1）才能禁用管理员
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.AdminDisableResponse{
			Success: false,
			Message: "未认证",
		})
		return
	}

	// 检查是否为超级管理员
	if userID != "1" {
		h.logger.Warn("非超级管理员尝试禁用管理员", zap.String("user_id", userID.(string)))
		c.JSON(http.StatusForbidden, models.AdminDisableResponse{
			Success: false,
			Message: "权限不足",
		})
		return
	}

	var req models.AdminDisableRequest

	// 绑定请求参数
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error("禁用管理员请求参数绑定失败", zap.Error(err))
		c.JSON(http.StatusBadRequest, models.AdminDisableResponse{
			Success: false,
			Message: "请求参数格式错误",
		})
		return
	}

	// 验证用户名
	if strings.TrimSpace(req.Username) == "" {
		c.JSON(http.StatusBadRequest, models.AdminDisableResponse{
			Success: false,
			Message: "用户名不能为空",
		})
		return
	}

	// 检查管理员是否存在
	existingAdmin, err := h.dbService.GetAdminByUsername(req.Username)
	if err != nil || existingAdmin == nil {
		c.JSON(http.StatusNotFound, models.AdminDisableResponse{
			Success: false,
			Message: "管理员不存在",
		})
		return
	}

	// 防止禁用超级管理员自己
	if existingAdmin.ID == 1 {
		c.JSON(http.StatusBadRequest, models.AdminDisableResponse{
			Success: false,
			Message: "不能禁用超级管理员",
		})
		return
	}

	// 转换enabled参数
	enabled := req.Enabled == "true"

	// 更新管理员状态
	updateReq := &models.AdminUpdateRequest{
		Enabled: &enabled,
	}
	err = h.dbService.UpdateAdmin(req.Username, updateReq)
	if err != nil {
		h.logger.Error("更新管理员状态失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.AdminDisableResponse{
			Success: false,
			Message: "更新管理员状态失败: " + err.Error(),
		})
		return
	}

	action := "启用"
	if !enabled {
		action = "禁用"
	}

	h.logger.Info("管理员状态更新成功",
		zap.String("username", req.Username),
		zap.Bool("enabled", enabled),
		zap.String("action", action))

	// 记录操作日志
	operatorUsername, _ := c.Get("username")
	userType, _ := middleware.GetUserTypeFromContext(c)
	if userType == "" {
		userType = "admin"
	}
	h.dbService.AddOperationLog(
		operatorUsername.(string),
		userType,
		action+"管理员",
		"管理员账户",
		fmt.Sprintf("%s管理员: %s", action, req.Username),
		c.ClientIP(),
		c.Request.UserAgent(),
		"成功",
	)

	c.JSON(http.StatusOK, models.AdminDisableResponse{
		Success: true,
		Message: action + "成功",
	})
}

// DeleteAdmin 删除管理员用户
func (h *SystemHandler) DeleteAdmin(c *gin.Context) {
	// 权限检查：只有超级管理员（ID为1）才能删除管理员
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.AdminDeleteResponse{
			Success: false,
			Message: "未认证",
		})
		return
	}

	// 检查是否为超级管理员
	if userID != "1" {
		h.logger.Warn("非超级管理员尝试删除管理员", zap.String("user_id", userID.(string)))
		c.JSON(http.StatusForbidden, models.AdminDeleteResponse{
			Success: false,
			Message: "权限不足",
		})
		return
	}

	var req models.AdminDeleteRequest

	// 绑定请求参数
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error("删除管理员请求参数绑定失败", zap.Error(err))
		c.JSON(http.StatusBadRequest, models.AdminDeleteResponse{
			Success: false,
			Message: "请求参数格式错误",
		})
		return
	}

	// 验证用户名
	if strings.TrimSpace(req.Username) == "" {
		c.JSON(http.StatusBadRequest, models.AdminDeleteResponse{
			Success: false,
			Message: "用户名不能为空",
		})
		return
	}

	// 检查管理员是否存在
	existingAdmin, err := h.dbService.GetAdminByUsername(req.Username)
	if err != nil || existingAdmin == nil {
		c.JSON(http.StatusNotFound, models.AdminDeleteResponse{
			Success: false,
			Message: "管理员不存在",
		})
		return
	}

	// 防止删除超级管理员自己
	if existingAdmin.ID == 1 {
		c.JSON(http.StatusBadRequest, models.AdminDeleteResponse{
			Success: false,
			Message: "不能删除超级管理员",
		})
		return
	}

	// 删除管理员
	err = h.dbService.DeleteAdmin(req.Username)
	if err != nil {
		h.logger.Error("删除管理员失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.AdminDeleteResponse{
			Success: false,
			Message: "删除管理员失败: " + err.Error(),
		})
		return
	}

	h.logger.Info("管理员删除成功",
		zap.String("username", req.Username),
		zap.String("operator", userID.(string)))

	// 记录操作日志
	operatorUsername, _ := c.Get("username")
	userType, _ := middleware.GetUserTypeFromContext(c)
	if userType == "" {
		userType = "admin"
	}
	h.dbService.AddOperationLog(
		operatorUsername.(string),
		userType,
		"删除管理员",
		"管理员账户",
		fmt.Sprintf("删除管理员: %s", req.Username),
		c.ClientIP(),
		c.Request.UserAgent(),
		"成功",
	)

	c.JSON(http.StatusOK, models.AdminDeleteResponse{
		Success: true,
		Message: "删除成功",
	})
}

// GetAdminList 获取管理员列表
func (h *SystemHandler) GetAdminList(c *gin.Context) {
	// 权限检查：只有超级管理员（ID为1）才能获取管理员列表
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.AdminListResponse{
			Success: false,
			Message: "未认证",
		})
		return
	}

	// 检查是否为超级管理员
	if userID != "1" {
		h.logger.Warn("非超级管理员尝试获取管理员列表", zap.String("user_id", userID.(string)))
		c.JSON(http.StatusForbidden, models.AdminListResponse{
			Success: false,
			Message: "权限不足",
		})
		return
	}

	// 获取所有管理员
	admins, err := h.dbService.GetAllAdmins()
	if err != nil {
		h.logger.Error("获取管理员列表失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.AdminListResponse{
			Success: false,
			Message: "获取列表失败: " + err.Error(),
		})
		return
	}

	// 清除密码字段和更新时间字段，不返回给前端
	for i := range admins {
		admins[i].Password = ""
		admins[i].UpdatedAt = time.Time{}
	}

	h.logger.Info("获取管理员列表成功", zap.String("operator", userID.(string)))
	c.JSON(http.StatusOK, models.AdminListResponse{
		Success: true,
		Message: "获取成功",
		Data:    admins,
	})
}

// EditAdmin 修改管理员信息
func (h *SystemHandler) EditAdmin(c *gin.Context) {
	// 权限检查：只有超级管理员（ID为1）才能修改管理员信息
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.AdminEditResponse{
			Success: false,
			Message: "未认证",
		})
		return
	}

	// 检查是否为超级管理员
	if userID != "1" {
		h.logger.Warn("非超级管理员尝试修改管理员信息", zap.String("user_id", userID.(string)))
		c.JSON(http.StatusForbidden, models.AdminEditResponse{
			Success: false,
			Message: "权限不足",
		})
		return
	}

	// 绑定请求参数
	var req models.AdminEditRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("修改管理员信息参数绑定失败", zap.Error(err))
		c.JSON(http.StatusBadRequest, models.AdminEditResponse{
			Success: false,
			Message: "参数格式错误: " + err.Error(),
		})
		return
	}

	// 验证参数
	if strings.TrimSpace(req.Username) == "" {
		c.JSON(http.StatusBadRequest, models.AdminEditResponse{
			Success: false,
			Message: "用户名不能为空",
		})
		return
	}

	if strings.TrimSpace(req.Password) == "" {
		c.JSON(http.StatusBadRequest, models.AdminEditResponse{
			Success: false,
			Message: "密码不能为空",
		})
		return
	}

	// 检查目标管理员是否存在
	_, err := h.dbService.GetAdmin(req.Username)
	if err != nil {
		h.logger.Warn("要修改的管理员不存在", zap.String("username", req.Username), zap.Error(err))
		c.JSON(http.StatusNotFound, models.AdminEditResponse{
			Success: false,
			Message: "管理员不存在",
		})
		return
	}

	// 更新管理员密码
	err = h.dbService.UpdateAdminPassword(req.Username, req.Password)
	if err != nil {
		h.logger.Error("修改管理员密码失败", zap.String("username", req.Username), zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.AdminEditResponse{
			Success: false,
			Message: "修改失败: " + err.Error(),
		})
		return
	}

	h.logger.Info("修改管理员密码成功", zap.String("operator", userID.(string)), zap.String("target_username", req.Username))

	// 记录操作日志
	operatorUsername, _ := c.Get("username")
	userType, _ := middleware.GetUserTypeFromContext(c)
	if userType == "" {
		userType = "admin"
	}
	h.dbService.AddOperationLog(
		operatorUsername.(string),
		userType,
		"修改管理员密码",
		"管理员账户",
		fmt.Sprintf("修改管理员密码: %s", req.Username),
		c.ClientIP(),
		c.Request.UserAgent(),
		"成功",
	)

	c.JSON(http.StatusOK, models.AdminEditResponse{
		Success: true,
		Message: "修改成功",
	})
}

// GetBlacklist 获取IP黑名单列表（支持分页和缓存）
func (h *SystemHandler) GetBlacklist(c *gin.Context) {
	// 权限检查：需要管理员权限
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.BlacklistPaginationResponse{
			Success: false,
			Message: "未认证",
		})
		return
	}

	// 检查是否为管理员（已通过JWT中间件验证）
	if userID == nil || userID == "" {
		h.logger.Warn("无效用户ID尝试查看黑名单", zap.Any("user_id", userID))
		c.JSON(http.StatusForbidden, models.BlacklistPaginationResponse{
			Success: false,
			Message: "权限不足",
		})
		return
	}

	// 获取分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	search := c.Query("search")

	// 参数验证
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}

	// 尝试从缓存获取数据
	cachedData, err := h.redisService.GetBlacklistCache(page, limit, search)
	if err == nil && cachedData != "" {
		h.logger.Debug("从缓存获取黑名单数据", zap.Int("page", page), zap.Int("limit", limit))
		c.Header("Content-Type", "application/json")
		c.String(http.StatusOK, cachedData)
		return
	}

	// 从数据库获取黑名单数据
	blacklists, total, err := h.dbService.GetBlacklistWithPagination(page, limit, search)
	if err != nil {
		h.logger.Error("获取黑名单失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.BlacklistPaginationResponse{
			Success: false,
			Message: "获取黑名单失败",
		})
		return
	}

	// 计算总页数
	pages := (total + limit - 1) / limit

	// 构建响应
	response := models.BlacklistPaginationResponse{
		Success: true,
		Message: "获取黑名单成功",
		Data:    blacklists,
		Total:   total,
		Page:    page,
		Limit:   limit,
		Pages:   pages,
	}

	// 缓存响应数据（5分钟）
	responseJSON, _ := json.Marshal(response)
	h.redisService.SetBlacklistCache(page, limit, search, string(responseJSON), 5*time.Minute)

	h.logger.Info("获取黑名单成功", zap.String("operator", userID.(string)), zap.Int("total", total))
	c.JSON(http.StatusOK, response)
}

// AddBlacklist 添加IP黑名单
func (h *SystemHandler) AddBlacklist(c *gin.Context) {
	// 权限检查：需要管理员权限
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.BlacklistAddResponse{
			Success: false,
			Message: "未认证",
		})
		return
	}

	// 检查是否为管理员（已通过JWT中间件验证）
	if userID == nil || userID == "" {
		h.logger.Warn("无效用户ID尝试添加黑名单", zap.Any("user_id", userID))
		c.JSON(http.StatusForbidden, models.BlacklistAddResponse{
			Success: false,
			Message: "权限不足",
		})
		return
	}

	// 绑定请求参数
	var req models.BlacklistAddRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error("添加黑名单请求参数绑定失败", zap.Error(err))
		c.JSON(http.StatusBadRequest, models.BlacklistAddResponse{
			Success: false,
			Message: "请求参数格式错误",
		})
		return
	}

	// 验证IP地址
	if strings.TrimSpace(req.IPAddress) == "" {
		c.JSON(http.StatusBadRequest, models.BlacklistAddResponse{
			Success: false,
			Message: "IP地址不能为空",
		})
		return
	}

	// 验证封禁时长
	if req.BanDuration <= 0 {
		c.JSON(http.StatusBadRequest, models.BlacklistAddResponse{
			Success: false,
			Message: "封禁时长必须大于0",
		})
		return
	}

	// 验证事件类型
	if strings.TrimSpace(req.EventType) == "" {
		c.JSON(http.StatusBadRequest, models.BlacklistAddResponse{
			Success: false,
			Message: "事件类型不能为空",
		})
		return
	}

	// 检查IP是否已在黑名单中
	exists, err := h.dbService.CheckBlacklistExists(req.IPAddress)
	if err != nil {
		h.logger.Error("检查黑名单是否存在失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.BlacklistAddResponse{
			Success: false,
			Message: "检查黑名单失败",
		})
		return
	}

	if exists {
		c.JSON(http.StatusConflict, models.BlacklistAddResponse{
			Success: false,
			Message: "IP地址已在黑名单中",
		})
		return
	}

	// 添加黑名单记录
	err = h.dbService.AddBlacklist(req.IPAddress, req.BanDuration, req.EventType)
	if err != nil {
		h.logger.Error("添加黑名单失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.BlacklistAddResponse{
			Success: false,
			Message: "添加黑名单失败: " + err.Error(),
		})
		return
	}

	// 清除相关缓存
	h.redisService.ClearBlacklistCache()

	h.logger.Info("添加黑名单成功",
		zap.String("operator", userID.(string)),
		zap.String("ip_address", req.IPAddress),
		zap.String("event_type", req.EventType),
		zap.Int("ban_duration", req.BanDuration))

	// 记录操作日志
	operatorUsername, _ := c.Get("username")
	if operatorUsername != nil {
		userType, _ := middleware.GetUserTypeFromContext(c)
		if userType == "" {
			userType = "admin"
		}
		h.dbService.AddOperationLog(
			operatorUsername.(string),
			userType,
			"添加黑名单",
			"IP黑名单",
			fmt.Sprintf("添加IP黑名单: %s, 事件类型: %s", req.IPAddress, req.EventType),
			c.ClientIP(),
			c.Request.UserAgent(),
			"成功",
		)
	}

	c.JSON(http.StatusOK, models.BlacklistAddResponse{
		Success: true,
		Message: "添加黑名单成功",
	})
}

// DeleteBlacklist 删除IP黑名单
func (h *SystemHandler) DeleteBlacklist(c *gin.Context) {
	// 权限检查：需要管理员权限
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "未授权访问",
		})
		return
	}

	// 验证管理员权限
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "未授权访问",
		})
		return
	}

	// 获取管理员信息
	admin, err := h.authService.GetAdminByUsername(username.(string))
	if err != nil {
		h.logger.Error("获取管理员信息失败", zap.Error(err), zap.String("username", username.(string)))
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "权限验证失败",
		})
		return
	}

	// 验证是否为管理员
	if admin.ID == 0 {
		h.logger.Warn("非管理员尝试删除黑名单", zap.String("username", username.(string)))
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "需要管理员权限",
		})
		return
	}

	// 解析请求数据
	var req models.DeleteBlacklistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 数据验证
	if req.IPAddress == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "IP地址不能为空",
		})
		return
	}

	// 删除黑名单记录
	err = h.dbService.DeleteBlacklist(req.IPAddress)
	if err != nil {
		h.logger.Error("删除黑名单失败", zap.Error(err))
		if strings.Contains(err.Error(), "IP地址不在黑名单中") {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "IP地址不在黑名单中",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "删除黑名单失败",
			})
		}
		return
	}

	// 清除相关缓存
	h.redisService.ClearBlacklistCache()

	// 记录操作日志
	h.logger.Info("删除黑名单成功",
		zap.String("operator", userID.(string)),
		zap.String("ip_address", req.IPAddress))

	// 记录操作日志到数据库
	operatorUsername, _ := c.Get("username")
	if operatorUsername != nil {
		userType, _ := middleware.GetUserTypeFromContext(c)
		if userType == "" {
			userType = "admin"
		}
		h.dbService.AddOperationLog(
			operatorUsername.(string),
			userType,
			"删除黑名单",
			"IP黑名单",
			fmt.Sprintf("删除IP黑名单: %s", req.IPAddress),
			c.ClientIP(),
			c.Request.UserAgent(),
			"成功",
		)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "删除黑名单成功",
	})
}

// GetSecurityConfiguration 获取系统安全配置
func (h *SystemHandler) GetSecurityConfiguration(c *gin.Context) {
	// 记录请求日志
	h.logger.Info("收到获取安全配置请求")

	// 尝试从Redis缓存获取配置
	cacheKey := "system:security_config"
	cachedData, err := h.redisService.Get(cacheKey)
	if err == nil && cachedData != "" {
		// 缓存命中，解析JSON数据
		var response models.SecurityConfigGetResponse
		if err := json.Unmarshal([]byte(cachedData), &response); err == nil {
			h.logger.Info("从缓存获取安全配置成功")
			c.JSON(http.StatusOK, response)
			return
		}
		h.logger.Warn("缓存数据解析失败，从数据库获取", zap.Error(err))
	}

	// 缓存未命中或解析失败，从数据库获取安全配置
	totpConfig, err := h.dbService.GetSystemConfig("TOTP")
	if err != nil {
		h.logger.Error("获取TOTP配置失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.SecurityConfigGetResponse{
			Success: false,
			Message: "获取安全配置失败: " + err.Error(),
		})
		return
	}

	lockedConfig, err := h.dbService.GetSystemConfig("LOCKED")
	if err != nil {
		h.logger.Error("获取LOCKED配置失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.SecurityConfigGetResponse{
			Success: false,
			Message: "获取安全配置失败: " + err.Error(),
		})
		return
	}

	// 构建配置数据
	configData := &models.SecurityConfigData{
		TOTP:   totpConfig,
		Locked: lockedConfig,
	}

	// 如果TOTP启用，获取相关配置
	if totpConfig == "true" {
		countConfig, err := h.dbService.GetSystemConfig("TOTP_COUNT")
		if err == nil {
			configData.Count = countConfig
		}

		// 获取totp_secret
		totpSecret, err := h.dbService.GetSystemConfig("totp_secret")
		if err == nil && totpSecret != "" {
			configData.TOTPSecret = totpSecret
		}

		// 获取totp_key
		totpKey, err := h.dbService.GetSystemConfig("totp_key")
		if err == nil && totpKey != "" {
			configData.TOTPKey = totpKey
		}
	}

	// 如果锁定功能启用，获取相关配置
	if lockedConfig == "true" {
		errorsConfig, err := h.dbService.GetSystemConfig("LOCK_ERRORS")
		if err == nil {
			configData.Errors = errorsConfig
		}

		lockTimeConfig, err := h.dbService.GetSystemConfig("LOCK_TIME")
		if err == nil {
			configData.LockTime = lockTimeConfig
		}
	}

	// 构建响应数据
	response := models.SecurityConfigGetResponse{
		Success: true,
		Message: "获取安全配置成功",
		Data:    configData,
	}

	// 将响应数据保存到Redis缓存，设置5分钟过期时间
	responseJSON, err := json.Marshal(response)
	if err == nil {
		err = h.redisService.Set(cacheKey, string(responseJSON), 5*time.Minute)
		if err != nil {
			h.logger.Warn("保存安全配置到缓存失败", zap.Error(err))
		} else {
			h.logger.Info("安全配置已保存到缓存")
		}
	} else {
		h.logger.Warn("序列化响应数据失败", zap.Error(err))
	}

	h.logger.Info("安全配置获取成功")
	c.JSON(http.StatusOK, response)
}

// GetSMTPConfiguration 获取SMTP配置
func (h *SystemHandler) GetSMTPConfiguration(c *gin.Context) {
	// 记录请求日志
	h.logger.Info("收到获取SMTP配置请求")

	// 从数据库获取SMTP配置
	smtpServer, err := h.dbService.GetSystemConfig("smtp_server")
	if err != nil {
		h.logger.Error("获取SMTP服务器配置失败", zap.Error(err))
		smtpServer = ""
	}

	smtpPort, err := h.dbService.GetSystemConfig("smtp_port")
	if err != nil {
		h.logger.Error("获取SMTP端口配置失败", zap.Error(err))
		smtpPort = ""
	}

	account, err := h.dbService.GetSystemConfig("smtp_account")
	if err != nil {
		h.logger.Error("获取SMTP账户配置失败", zap.Error(err))
		account = ""
	}

	password, err := h.dbService.GetSystemConfig("smtp_password")
	if err != nil {
		h.logger.Error("获取SMTP密码配置失败", zap.Error(err))
		password = ""
	}

	authEnable, err := h.dbService.GetSystemConfig("smtp_auth_enable")
	if err != nil {
		h.logger.Error("获取SMTP认证配置失败", zap.Error(err))
		authEnable = "false"
	}

	// 构建响应数据
	response := models.SMTPConfigGetResponse{
		Success:    true,
		Message:    "获取SMTP配置成功",
		SMTPServer: smtpServer,
		SMTPPort:   smtpPort,
		Account:    account,
		Password:   password,
		AuthEnable: authEnable,
	}

	h.logger.Info("SMTP配置获取成功")
	c.JSON(http.StatusOK, response)
}

// SaveSMTPConfiguration 保存SMTP配置
func (h *SystemHandler) SaveSMTPConfiguration(c *gin.Context) {
	// 记录请求日志
	h.logger.Info("收到保存SMTP配置请求")

	// 解析请求参数
	var req models.SMTPConfigSaveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error("请求参数格式错误", zap.Error(err))
		c.JSON(http.StatusBadRequest, models.SMTPConfigSaveResponse{
			Success: false,
			Message: "请求参数格式错误",
		})
		return
	}

	// 验证auth_enable参数
	if req.AuthEnable != "true" && req.AuthEnable != "false" {
		h.logger.Error("auth_enable参数值无效", zap.String("auth_enable", req.AuthEnable))
		c.JSON(http.StatusBadRequest, models.SMTPConfigSaveResponse{
			Success: false,
			Message: "auth_enable参数只能为true或false",
		})
		return
	}

	// 验证端口号
	if _, err := strconv.Atoi(req.SMTPPort); err != nil {
		h.logger.Error("SMTP端口格式错误", zap.String("port", req.SMTPPort), zap.Error(err))
		c.JSON(http.StatusBadRequest, models.SMTPConfigSaveResponse{
			Success: false,
			Message: "SMTP端口必须为数字",
		})
		return
	}

	// 保存配置到数据库
	configs := map[string]string{
		"smtp_server":      req.SMTPServer,
		"smtp_port":        req.SMTPPort,
		"smtp_account":     req.Account,
		"smtp_password":    req.Password,
		"smtp_auth_enable": req.AuthEnable,
	}

	for key, value := range configs {
		if err := h.dbService.SaveSystemConfig(key, value); err != nil {
			h.logger.Error("保存SMTP配置到数据库失败", zap.String("key", key), zap.Error(err))
			c.JSON(http.StatusInternalServerError, models.SMTPConfigSaveResponse{
				Success: false,
				Message: fmt.Sprintf("保存%s配置到数据库失败: %v", key, err),
			})
			return
		}
	}

	h.logger.Info("SMTP配置保存成功")

	// 记录操作日志
	operatorUsername, _ := c.Get("username")
	if operatorUsername != nil {
		userType, _ := middleware.GetUserTypeFromContext(c)
		if userType == "" {
			userType = "admin"
		}
		h.dbService.AddOperationLog(
			operatorUsername.(string),
			userType,
			"保存SMTP配置",
			"SMTP配置",
			fmt.Sprintf("保存SMTP配置: 服务器=%s, 端口=%s", req.SMTPServer, req.SMTPPort),
			c.ClientIP(),
			c.Request.UserAgent(),
			"成功",
		)
	}

	c.JSON(http.StatusOK, models.SMTPConfigSaveResponse{
		Success: true,
		Message: "SMTP配置保存成功",
	})
}

// GetOperationLogs 获取用户操作日志
func (h *SystemHandler) GetOperationLogs(c *gin.Context) {
	// 解析请求参数
	var req models.OperationLogGetRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		h.logger.Error("解析操作日志请求参数失败", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "请求参数格式错误",
		})
		return
	}

	// 设置默认值
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Limit <= 0 {
		req.Limit = 20
	}
	if req.Limit > 100 {
		req.Limit = 100 // 限制最大每页数量
	}

	// 从数据库获取操作日志
	logs, total, err := h.dbService.GetOperationLogsWithPagination(
		req.Page,
		req.Limit,
		req.Username,
		req.UserType,
		req.Operation,
		req.StartTime,
		req.EndTime,
	)
	if err != nil {
		h.logger.Error("获取操作日志失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "获取操作日志失败",
		})
		return
	}

	// 转换为简化格式
	simpleLogs := make([]models.OperationLogSimple, len(logs))
	for i, log := range logs {
		simpleLogs[i] = models.OperationLogSimple{
			Operator:       log.Username,
			OperationType:  h.getOperationTypeChinese(log.Operation),
			OperationEvent: log.Details,
			IPAddress:      log.IPAddress,
			CreatedAt:      log.CreatedAt,
		}
	}

	// 计算分页信息
	totalPages := (total + req.Limit - 1) / req.Limit

	// 构建响应
	response := models.OperationLogGetResponse{
		Success: true,
		Message: "获取操作日志成功",
		Data: &models.OperationLogData{
			Logs:       simpleLogs,
			Total:      total,
			Page:       req.Page,
			Limit:      req.Limit,
			TotalPages: totalPages,
		},
	}

	h.logger.Info("获取操作日志成功",
		zap.Int("total", total),
		zap.Int("page", req.Page),
		zap.Int("limit", req.Limit),
	)

	c.JSON(http.StatusOK, response)
}

// getOperationTypeChinese 获取操作类型的中文描述
func (h *SystemHandler) getOperationTypeChinese(operation string) string {
	// 如果已经是中文，直接返回
	switch operation {
	case "系统登录", "用户管理", "密码修改", "安全设置", "邮件认证", "保存系统配置":
		return operation
	// 兼容旧的英文字段
	case "admin_login":
		return "系统登录"
	case "user_create", "user_update", "user_delete":
		return "用户管理"
	case "password_modify":
		return "密码修改"
	case "totp_reset", "enable_totp", "disable_totp":
		return "安全设置"
	case "login":
		return "邮件认证"
	default:
		return operation
	}
}

// getOperationEventChinese 获取操作事件的中文描述
func (h *SystemHandler) getOperationEventChinese(operation, details, status string) string {
	if status == "success" {
		switch operation {
		// 中文operation字段
		case "系统登录":
			return "登录成功"
		case "用户管理":
			if strings.Contains(details, "创建") {
				return "用户创建成功"
			} else if strings.Contains(details, "更新") {
				return "用户更新成功"
			} else if strings.Contains(details, "删除") {
				return "用户删除成功"
			} else {
				return "用户管理成功"
			}
		case "密码修改":
			return "密码修改成功"
		case "安全设置":
			return "安全设置成功"
		case "邮件认证":
			return "认证成功"
		case "保存系统配置":
			return "配置保存成功"
		// 兼容旧的英文字段
		case "admin_login":
			return "登录成功"
		case "user_create":
			return "用户创建成功"
		case "user_update":
			return "用户更新成功"
		case "user_delete":
			return "用户删除成功"
		case "password_modify":
			return "密码修改成功"
		case "totp_reset":
			return "TOTP重置成功"
		case "enable_totp":
			return "TOTP开启成功"
		case "disable_totp":
			return "TOTP关闭成功"
		case "login":
			return "认证成功"
		default:
			return "操作成功"
		}
	} else {
		switch operation {
		// 中文operation字段
		case "系统登录":
			if strings.Contains(details, "参数错误") {
				return "登录失败（参数错误）"
			} else if strings.Contains(details, "管理员不存在") {
				return "登录失败（用户不存在）"
			} else if strings.Contains(details, "账户已禁用") {
				return "登录失败（账户已禁用）"
			} else if strings.Contains(details, "密码错误") {
				return "登录失败（密码错误）"
			} else {
				return "登录失败"
			}
		case "用户管理":
			if strings.Contains(details, "创建") {
				return "用户创建失败"
			} else if strings.Contains(details, "更新") {
				return "用户更新失败"
			} else if strings.Contains(details, "删除") {
				return "用户删除失败"
			} else {
				return "用户管理失败"
			}
		case "密码修改":
			return "密码修改失败"
		case "安全设置":
			return "安全设置失败"
		case "邮件认证":
			return "认证失败"
		case "保存系统配置":
			return "配置保存失败"
		// 兼容旧的英文字段
		case "admin_login":
			if strings.Contains(details, "参数错误") {
				return "登录失败（参数错误）"
			} else if strings.Contains(details, "管理员不存在") {
				return "登录失败（用户不存在）"
			} else if strings.Contains(details, "账户已禁用") {
				return "登录失败（账户已禁用）"
			} else if strings.Contains(details, "密码错误") {
				return "登录失败（密码错误）"
			} else {
				return "登录失败"
			}
		case "user_create":
			return "用户创建失败"
		case "user_update":
			return "用户更新失败"
		case "user_delete":
			return "用户删除失败"
		case "password_modify":
			return "密码修改失败"
		case "totp_reset":
			return "TOTP重置失败"
		case "enable_totp":
			return "TOTP开启失败"
		case "disable_totp":
			return "TOTP关闭失败"
		case "login":
			return "认证失败"
		default:
			return "操作失败"
		}
	}
}

// SaveSecurityConfiguration 保存系统安全配置
func (h *SystemHandler) SaveSecurityConfiguration(c *gin.Context) {
	var req models.SecurityConfigSaveRequest

	// 绑定请求参数
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error("安全配置请求参数绑定失败", zap.Error(err))
		c.JSON(http.StatusBadRequest, models.SecurityConfigSaveResponse{
			Success: false,
			Message: "请求参数格式错误",
		})
		return
	}

	// 验证TOTP相关参数
	if req.TOTP == "true" {
		if req.Count == "" {
			c.JSON(http.StatusBadRequest, models.SecurityConfigSaveResponse{
				Success: false,
				Message: "当TOTP启用时，count参数不能为空",
			})
			return
		}
		// 验证count是否为数字且不为0
		count, err := strconv.Atoi(req.Count)
		if err != nil || count <= 0 {
			c.JSON(http.StatusBadRequest, models.SecurityConfigSaveResponse{
				Success: false,
				Message: "count参数必须为大于0的数字",
			})
			return
		}
	}

	// 验证锁定相关参数
	if req.Locked == "true" {
		if req.Errors == "" {
			c.JSON(http.StatusBadRequest, models.SecurityConfigSaveResponse{
				Success: false,
				Message: "当锁定功能启用时，errors参数不能为空",
			})
			return
		}
		if req.LockTime == "" {
			c.JSON(http.StatusBadRequest, models.SecurityConfigSaveResponse{
				Success: false,
				Message: "当锁定功能启用时，locktime参数不能为空",
			})
			return
		}
		// 验证errors是否为数字且不为0
		errors, err := strconv.Atoi(req.Errors)
		if err != nil || errors <= 0 {
			c.JSON(http.StatusBadRequest, models.SecurityConfigSaveResponse{
				Success: false,
				Message: "errors参数必须为大于0的数字",
			})
			return
		}
		// 验证locktime是否为数字且不为0
		lockTime, err := strconv.Atoi(req.LockTime)
		if err != nil || lockTime <= 0 {
			c.JSON(http.StatusBadRequest, models.SecurityConfigSaveResponse{
				Success: false,
				Message: "locktime参数必须为大于0的数字",
			})
			return
		}
	}

	// 构建配置JSON
	configData := map[string]interface{}{
		"totp":     req.TOTP,
		"locked":   req.Locked,
		"updateAt": time.Now().Unix(),
	}

	// 添加条件参数
	if req.TOTP == "true" {
		configData["count"] = req.Count
	}
	if req.Locked == "true" {
		configData["errors"] = req.Errors
		configData["locktime"] = req.LockTime
	}

	configJSON, err := json.Marshal(configData)
	if err != nil {
		h.logger.Error("安全配置JSON序列化失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.SecurityConfigSaveResponse{
			Success: false,
			Message: "配置数据处理失败",
		})
		return
	}

	// 保存到Redis
	err = h.redisService.Set("system:security_configuration", string(configJSON), 0)
	if err != nil {
		h.logger.Error("保存安全配置到Redis失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.SecurityConfigSaveResponse{
			Success: false,
			Message: "保存配置到Redis失败: " + err.Error(),
		})
		return
	}

	// 同时保存到MySQL数据库
	err = h.dbService.SaveSystemConfig("TOTP", req.TOTP)
	if err != nil {
		h.logger.Error("保存TOTP配置到数据库失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.SecurityConfigSaveResponse{
			Success: false,
			Message: "保存TOTP配置到数据库失败: " + err.Error(),
		})
		return
	}

	if req.TOTP == "true" {
		err = h.dbService.SaveSystemConfig("TOTP_COUNT", req.Count)
		if err != nil {
			h.logger.Error("保存TOTP_COUNT配置到数据库失败", zap.Error(err))
			c.JSON(http.StatusInternalServerError, models.SecurityConfigSaveResponse{
				Success: false,
				Message: "保存TOTP_COUNT配置到数据库失败: " + err.Error(),
			})
			return
		}

		// 检查是否已存在totp_key配置
		existingKey, err := h.dbService.GetSystemConfig("totp_key")
		if err != nil || existingKey == "" {
			// 生成新的TOTP密钥
			secret, err := h.totpService.GenerateTOTPSecret()
			if err != nil {
				h.logger.Error("生成TOTP密钥失败", zap.Error(err))
				c.JSON(http.StatusInternalServerError, models.SecurityConfigSaveResponse{
					Success: false,
					Message: "生成TOTP密钥失败: " + err.Error(),
				})
				return
			}

			// 保存实际的TOTP密钥到数据库
			err = h.dbService.SaveSystemConfig("totp_key", secret)
			if err != nil {
				h.logger.Error("保存TOTP密钥到数据库失败", zap.Error(err))
				c.JSON(http.StatusInternalServerError, models.SecurityConfigSaveResponse{
					Success: false,
					Message: "保存TOTP密钥到数据库失败: " + err.Error(),
				})
				return
			}

			// 生成二维码Base64图片
			qrCodeBase64, err := h.totpService.GenerateQRCodeImage("system", secret)
			if err != nil {
				h.logger.Error("生成TOTP二维码失败", zap.Error(err))
				c.JSON(http.StatusInternalServerError, models.SecurityConfigSaveResponse{
					Success: false,
					Message: "生成TOTP二维码失败: " + err.Error(),
				})
				return
			}

			// 保存totp_secret到数据库（二维码图片）
			err = h.dbService.SaveSystemConfig("totp_secret", qrCodeBase64)
			if err != nil {
				h.logger.Error("保存TOTP二维码到数据库失败", zap.Error(err))
				c.JSON(http.StatusInternalServerError, models.SecurityConfigSaveResponse{
					Success: false,
					Message: "保存TOTP二维码到数据库失败: " + err.Error(),
				})
				return
			}

			h.logger.Info("TOTP密钥和二维码生成成功", zap.String("secret_length", fmt.Sprintf("%d", len(secret))))
		}
	}

	err = h.dbService.SaveSystemConfig("LOCKED", req.Locked)
	if err != nil {
		h.logger.Error("保存LOCKED配置到数据库失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.SecurityConfigSaveResponse{
			Success: false,
			Message: "保存LOCKED配置到数据库失败: " + err.Error(),
		})
		return
	}

	if req.Locked == "true" {
		err = h.dbService.SaveSystemConfig("LOCK_ERRORS", req.Errors)
		if err != nil {
			h.logger.Error("保存LOCK_ERRORS配置到数据库失败", zap.Error(err))
			c.JSON(http.StatusInternalServerError, models.SecurityConfigSaveResponse{
				Success: false,
				Message: "保存LOCK_ERRORS配置到数据库失败: " + err.Error(),
			})
			return
		}

		err = h.dbService.SaveSystemConfig("LOCK_TIME", req.LockTime)
		if err != nil {
			h.logger.Error("保存LOCK_TIME配置到数据库失败", zap.Error(err))
			c.JSON(http.StatusInternalServerError, models.SecurityConfigSaveResponse{
				Success: false,
				Message: "保存LOCK_TIME配置到数据库失败: " + err.Error(),
			})
			return
		}
	}

	h.logger.Info("安全配置保存成功",
		zap.String("TOTP", req.TOTP),
		zap.String("Count", req.Count),
		zap.String("Locked", req.Locked),
		zap.String("Errors", req.Errors),
		zap.String("LockTime", req.LockTime))

	// 清除安全配置缓存，确保下次获取时使用最新数据
	err = h.redisService.Delete("system:security_config")
	if err != nil {
		h.logger.Warn("清除安全配置缓存失败", zap.Error(err))
	} else {
		h.logger.Info("安全配置缓存已清除")
	}

	// 记录操作日志
	operatorUsername, _ := c.Get("username")
	if operatorUsername != nil {
		userType, _ := middleware.GetUserTypeFromContext(c)
		if userType == "" {
			userType = "admin"
		}
		h.dbService.AddOperationLog(
			operatorUsername.(string),
			userType,
			"保存安全配置",
			"安全配置",
			fmt.Sprintf("保存安全配置: TOTP=%s, 锁定=%s", req.TOTP, req.Locked),
			c.ClientIP(),
			c.Request.UserAgent(),
			"成功",
		)
	}

	c.JSON(http.StatusOK, models.SecurityConfigSaveResponse{
		Success: true,
		Message: "安全配置保存成功",
	})
}
