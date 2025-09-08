package handlers

import (
	"fmt"
	"net/http"

	"MailGateway/middleware"
	"MailGateway/models"
	"MailGateway/services"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AuthHandler 认证处理器
type AuthHandler struct {
	authService *services.AuthService
	dbService   *services.DatabaseService
	logger      *zap.Logger
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler(authService *services.AuthService, dbService *services.DatabaseService, logger *zap.Logger) *AuthHandler {
	return &AuthHandler{
		authService: authService,
		dbService:   dbService,
		logger:      logger,
	}
}

// NginxAuth Nginx认证接口
func (h *AuthHandler) NginxAuth(c *gin.Context) {
	// 从请求头获取认证信息
	authReq := &models.AuthRequest{
		User:     c.GetHeader("Auth-User"),
		Pass:     c.GetHeader("Auth-Pass"),
		Protocol: c.GetHeader("Auth-Protocol"),
		ClientIP: c.GetHeader("Client-IP"),
		Method:   c.GetHeader("Auth-Method"),
	}

	// 如果Client-IP为空，使用真实IP
	if authReq.ClientIP == "" {
		authReq.ClientIP = c.ClientIP()
	}

	// 验证请求参数
	if err := h.authService.ValidateAuthRequest(authReq); err != nil {
		h.logger.Warn("无效的认证请求",
			zap.String("error", err.Error()),
			zap.String("client_ip", authReq.ClientIP))

		c.Header("Auth-Status", "Invalid request")
		c.Status(http.StatusBadRequest)
		return
	}

	// 执行认证
	authResp, err := h.authService.AuthenticateUser(authReq)
	if err != nil {
		h.logger.Error("认证错误",
			zap.String("username", authReq.User),
			zap.String("client_ip", authReq.ClientIP),
			zap.Error(err))

		c.Header("Auth-Status", "Internal error")
		c.Status(http.StatusInternalServerError)
		return
	}

	// 设置响应头
	c.Header("Auth-Status", authResp.Status)
	if authResp.Server != "" {
		c.Header("Auth-Server", authResp.Server)
	}
	if authResp.Port != "" {
		c.Header("Auth-Port", authResp.Port)
	}
	if authResp.Pass != "" {
		c.Header("Auth-Pass", authResp.Pass)
	}

	// 记录认证日志
	result := "success"
	var resultDetail string
	switch authResp.Status {
	case "OK":
		// 检查用户是否启用了TOTP来确定认证类型
		user, err := h.dbService.GetUser(authReq.User)
		if err == nil && user != nil && user.TOTPEnabled {
			resultDetail = "认证成功 (密码+TOTP)"
		} else {
			resultDetail = "认证成功"
		}
	case "Invalid credentials":
		result = "failed"
		resultDetail = "认证失败: 用户名、密码或TOTP码错误"
	case "User locked":
		result = "failed"
		resultDetail = "认证失败: 用户已被锁定"
	case "Internal error":
		result = "failed"
		resultDetail = "认证失败: 内部错误"
	default:
		result = "failed"
		resultDetail = fmt.Sprintf("认证失败: %s", authResp.Status)
	}

	// 添加认证日志记录
	err = h.dbService.AddAuthLog(
		authReq.User,
		authReq.ClientIP,
		authReq.Protocol,
		result,
		resultDetail,
		c.GetHeader("Client-IP"),
		c.GetHeader("User-Agent"),
	)
	if err != nil {
		h.logger.Error("添加认证日志失败",
			zap.String("username", authReq.User),
			zap.String("client_ip", authReq.ClientIP),
			zap.Error(err))
	}

	// 根据认证结果返回状态码
	if authResp.Status == "OK" {
		c.Status(http.StatusOK)
	} else {
		c.Status(http.StatusForbidden)
	}
}

// GetUserStatus 获取用户状态
func (h *AuthHandler) GetUserStatus(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Username is required",
		})
		return
	}

	status, err := h.authService.GetUserStatus(username)
	if err != nil {
		h.logger.Error("获取用户状态失败",
			zap.String("username", username),
			zap.Error(err))

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get user status",
		})
		return
	}

	c.JSON(http.StatusOK, status)
}

// UnlockUser 解锁用户（使用TOTP）
func (h *AuthHandler) UnlockUser(c *gin.Context) {
	var req models.TOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request parameters",
			"details": err.Error(),
		})
		return
	}

	resp, err := h.authService.UnlockUserWithTOTP(req.Username, req.Code)
	if err != nil {
		h.logger.Error("解锁用户失败",
			zap.String("username", req.Username),
			zap.Error(err))

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to unlock user",
		})
		return
	}

	// 获取用户类型
	userType, _ := middleware.GetUserTypeFromContext(c)
	if userType == "" {
		userType = "user" // 默认值
	}

	// 记录解锁用户操作日志
	status := "success"
	if !resp.Valid {
		status = "failed"
	}
	h.dbService.AddOperationLog(req.Username, userType, "unlock_user", "totp_unlock",
		"使用TOTP解锁用户", c.ClientIP(), c.GetHeader("User-Agent"), status)

	if resp.Valid {
		c.JSON(http.StatusOK, resp)
	} else {
		c.JSON(http.StatusBadRequest, resp)
	}
}

// Dashboard 主页接口
func (h *AuthHandler) Dashboard(c *gin.Context) {
	// 从JWT中获取用户信息
	userID, userIDExists := c.Get("user_id")
	username, usernameExists := c.Get("username")
	if !userIDExists || !usernameExists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "未认证",
		})
		return
	}

	// 获取实时统计信息
	stats, err := h.authService.GetRealtimeStats()
	if err != nil {
		h.logger.Error("获取实时统计失败", zap.Error(err))
		stats = nil
	}

	// 返回主页数据
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "欢迎访问邮件网关管理系统",
		"user": gin.H{
			"id":       userID,
			"username": username,
		},
		"stats": stats,
	})
}

// GetAuthLogs 获取认证日志列表
func (h *AuthHandler) GetAuthLogs(c *gin.Context) {
	var req models.AuthLogGetRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "参数错误",
			"details": err.Error(),
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
		req.Limit = 100
	}

	// 获取认证日志
	logs, total, err := h.dbService.GetAuthLogsWithPagination(
		req.Page, req.Limit, req.Username, req.Protocol, req.Result,
		req.IPAddress, req.StartTime, req.EndTime)
	if err != nil {
		h.logger.Error("获取认证日志失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取认证日志失败",
		})
		return
	}

	// 转换为简化格式
	var simpleLogs []models.AuthLogSimple
	for _, log := range logs {
		simpleLogs = append(simpleLogs, models.AuthLogSimple{
			Username:     log.Username,
			IPAddress:    log.IPAddress,
			Protocol:     log.Protocol,
			Result:       log.Result,
			ResultDetail: log.ResultDetail,
			CreatedAt:    log.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	// 计算总页数
	totalPages := (total + req.Limit - 1) / req.Limit

	// 构造响应
	resp := models.AuthLogGetResponse{
		Success: true,
		Message: "获取成功",
		Data: &models.AuthLogData{
			Logs:       simpleLogs,
			Total:      total,
			Page:       req.Page,
			Limit:      req.Limit,
			TotalPages: totalPages,
		},
	}

	c.JSON(http.StatusOK, resp)
}

// GetAuthLogStats 获取认证日志统计信息
func (h *AuthHandler) GetAuthLogStats(c *gin.Context) {
	// 获取统计信息
	stats, err := h.dbService.GetAuthLogStats()
	if err != nil {
		h.logger.Error("获取认证日志统计失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取统计信息失败",
		})
		return
	}

	// 构造响应
	resp := models.AuthLogStatsResponse{
		Success: true,
		Message: "获取成功",
		Data:    stats,
	}

	c.JSON(http.StatusOK, resp)
}

// GetDashboardInformation 获取仪表盘信息
func (h *AuthHandler) GetDashboardInformation(c *gin.Context) {
	stats, err := h.dbService.GetDashboardStats()
	if err != nil {
		h.logger.Error("获取仪表盘统计失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, &models.DashboardStatsResponse{
			Success: false,
			Message: "获取仪表盘统计失败",
		})
		return
	}

	c.JSON(http.StatusOK, &models.DashboardStatsResponse{
		Success: true,
		Message: "获取仪表盘统计成功",
		Data:    stats,
	})
}

// GetAuthInformation 获取认证日志信息给前端展示
func (h *AuthHandler) GetAuthInformation(c *gin.Context) {
	var req models.AuthLogGetRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "参数错误",
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
		req.Limit = 100
	}

	// 获取认证日志
	logs, total, err := h.dbService.GetAuthLogsWithPagination(
		req.Page, req.Limit, req.Username, req.Protocol, req.Result,
		req.IPAddress, req.StartTime, req.EndTime)
	if err != nil {
		h.logger.Error("获取认证日志失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "获取认证日志失败",
		})
		return
	}

	// 转换为前端需要的格式
	type AuthInformation struct {
		Username     string `json:"username"`
		IP           string `json:"ip"`
		Protocol     string `json:"protocol"`
		ResultDetail string `json:"result_detail"`
		CreatedAt    string `json:"created_at"`
	}

	var authInfos []AuthInformation
	for _, log := range logs {
		authInfos = append(authInfos, AuthInformation{
			Username:     log.Username,
			IP:           log.IPAddress,
			Protocol:     log.Protocol,
			ResultDetail: log.ResultDetail,
			CreatedAt:    log.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	// 计算总页数
	totalPages := (total + req.Limit - 1) / req.Limit

	// 构造响应
	response := gin.H{
		"success": true,
		"message": "获取认证日志成功",
		"data": gin.H{
			"logs":       authInfos,
			"total":      total,
			"page":       req.Page,
			"limit":      req.Limit,
			"totalPages": totalPages,
		},
	}

	c.JSON(http.StatusOK, response)
}
