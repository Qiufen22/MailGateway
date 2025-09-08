package services

import (
	"fmt"
	"strings"
	"time"

	"MailGateway/config"
	"MailGateway/models"
	"go.uber.org/zap"
)

// AuthService 认证服务
type AuthService struct {
	dbService    *DatabaseService
	redisService *RedisService
	totpService  *TOTPService
	logger       *zap.Logger
	config       *config.Config
}

// NewAuthService 创建认证服务实例
func NewAuthService(dbService *DatabaseService, redisService *RedisService, totpService *TOTPService, cfg *config.Config, logger *zap.Logger) *AuthService {
	return &AuthService{
		dbService:    dbService,
		redisService: redisService,
		totpService:  totpService,
		logger:       logger,
		config:       cfg,
	}
}

// AuthenticateUser 用户认证主逻辑
func (s *AuthService) AuthenticateUser(req *models.AuthRequest) (*models.AuthResponse, error) {
	// 记录QPS
	s.redisService.IncrementQPSCounter()

	// 检查用户是否被锁定
	locked, err := s.redisService.IsUserLocked(req.User)
	if err != nil {
		s.logger.Error("检查用户锁定状态失败",
			zap.String("username", req.User),
			zap.Error(err))
		return &models.AuthResponse{Status: "Internal error"}, err
	}

	if locked {
		s.logger.Warn("被锁定用户尝试认证",
			zap.String("username", req.User),
			zap.String("client_ip", req.ClientIP))

		return &models.AuthResponse{Status: "User locked"}, nil
	}

	// 获取用户信息以检查TOTP状态
	user, err := s.dbService.GetUser(req.User)
	if err != nil {
		s.logger.Error("获取用户信息失败",
			zap.String("username", req.User),
			zap.Error(err))
		return &models.AuthResponse{Status: "Internal error"}, err
	}
	if user == nil {
		s.logger.Warn("用户不存在",
			zap.String("username", req.User))
		return &models.AuthResponse{Status: "Invalid credentials"}, nil
	}

	// 处理TOTP验证
	var actualPassword string
	var totpCode string
	if user.TOTPEnabled && user.TOTPSecret != "" {
		// 如果启用了TOTP，从密码中分离出TOTP码（最后6位）
		if len(req.Pass) < 6 {
			s.logger.Warn("密码长度不足，无法提取TOTP码",
				zap.String("username", req.User))
			return &models.AuthResponse{Status: "Invalid credentials"}, nil
		}
		actualPassword = req.Pass[:len(req.Pass)-6]
		totpCode = req.Pass[len(req.Pass)-6:]
	} else {
		actualPassword = req.Pass
	}

	// 首先尝试从缓存获取密码
	cachedPassword, err := s.redisService.GetUserPasswordCache(req.User)
	if err != nil {
		s.logger.Error("获取缓存密码失败",
			zap.String("username", req.User),
			zap.Error(err))
	}

	var passwordValid bool
	if cachedPassword != "" {
		// 使用缓存的密码验证
		passwordValid = (cachedPassword == actualPassword)
	} else {
		// 从数据库验证密码
		passwordValid, err = s.dbService.ValidatePassword(req.User, actualPassword)
		if err != nil {
			s.logger.Error("验证密码失败",
				zap.String("username", req.User),
				zap.Error(err))
			return &models.AuthResponse{Status: "Internal error"}, err
		}

		// 如果密码正确，缓存密码（5分钟）
		if passwordValid {
			s.redisService.SetUserPasswordCache(req.User, actualPassword, 5*time.Minute)
		}
	}

	if !passwordValid {
		// 密码错误，记录失败
		err = s.redisService.RecordLoginFailure(req.User)
		if err != nil {
			s.logger.Error("记录登录失败失败",
				zap.String("username", req.User),
				zap.Error(err))
		}

		s.logger.Warn("认证失败 - 密码无效",
			zap.String("username", req.User),
			zap.String("client_ip", req.ClientIP))

		return &models.AuthResponse{Status: "Invalid credentials"}, nil
	}

	// 如果启用了TOTP，验证TOTP码
	if user.TOTPEnabled && user.TOTPSecret != "" {
		totpValid, err := s.totpService.ValidateTOTPCode(req.User, totpCode)
		if err != nil {
			s.logger.Error("TOTP验证失败",
				zap.String("username", req.User),
				zap.Error(err))
			return &models.AuthResponse{Status: "Internal error"}, err
		}
		if !totpValid {
			// TOTP码错误，记录失败
			err = s.redisService.RecordLoginFailure(req.User)
			if err != nil {
				s.logger.Error("记录登录失败失败",
					zap.String("username", req.User),
					zap.Error(err))
			}

			s.logger.Warn("认证失败 - TOTP码无效",
				zap.String("username", req.User),
				zap.String("client_ip", req.ClientIP))

			return &models.AuthResponse{Status: "Invalid credentials"}, nil
		}
	}

	// 密码和TOTP验证都通过，清除失败记录
	err = s.redisService.ClearLoginFailures(req.User)
	if err != nil {
		s.logger.Error("清除登录失败记录失败",
			zap.String("username", req.User),
			zap.Error(err))
	}

	// 获取后端服务器信息
	backendServer, backendPort := s.getBackendServer(req.Protocol)

	s.logger.Info("认证成功",
		zap.String("username", req.User),
		zap.String("protocol", req.Protocol),
		zap.String("client_ip", req.ClientIP),
		zap.String("backend", fmt.Sprintf("%s:%s", backendServer, backendPort)))

	return &models.AuthResponse{
		Status: "OK",
		Server: backendServer,
		Port:   backendPort,
		Pass:   actualPassword,
	}, nil
}

// getBackendServer 获取后端服务器信息
func (s *AuthService) getBackendServer(protocol string) (string, string) {
	// 这里可以根据协议和负载均衡策略选择后端服务器
	// 目前使用固定配置
	switch strings.ToLower(protocol) {
	case "pop3":
		return "10.251.65.150", "110"
	case "imap":
		return "10.251.65.150", "143"
	case "smtp":
		return "10.251.65.150", "25"
	default:
		return "10.251.65.150", "110" // 默认POP3
	}
}

// GetUserStatus 获取用户状态
func (s *AuthService) GetUserStatus(username string) (map[string]interface{}, error) {
	// 检查用户是否存在
	user, err := s.dbService.GetUser(username)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return map[string]interface{}{
			"exists": false,
		}, nil
	}

	// 检查是否被锁定
	locked, err := s.redisService.IsUserLocked(username)
	if err != nil {
		return nil, fmt.Errorf("failed to check lock status: %w", err)
	}

	// 获取失败次数
	failCount, err := s.redisService.GetLoginFailureCount(username)
	if err != nil {
		return nil, fmt.Errorf("failed to get failure count: %w", err)
	}

	return map[string]interface{}{
		"exists":       true,
		"username":     user.Username,
		"totp_enabled": user.TOTPEnabled,
		"locked":       locked,
		"fail_count":   failCount,
		"created_at":   user.CreatedAt,
		"updated_at":   user.UpdatedAt,
	}, nil
}

// UnlockUserWithTOTP 使用TOTP解锁用户
func (s *AuthService) UnlockUserWithTOTP(username, totpCode string) (*models.TOTPResponse, error) {
	// 检查用户是否被锁定
	locked, err := s.redisService.IsUserLocked(username)
	if err != nil {
		return &models.TOTPResponse{
			Valid:   false,
			Message: "Failed to check lock status",
		}, err
	}

	if !locked {
		return &models.TOTPResponse{
			Valid:   false,
			Message: "User is not locked",
		}, nil
	}

	// 验证TOTP并解锁
	return s.totpService.VerifyTOTPAndUnlock(s.redisService, username, totpCode)
}

// GetRealtimeStats 获取实时统计信息
func (s *AuthService) GetRealtimeStats() (*models.RealtimeStats, error) {
	// 获取当前QPS
	currentQPS, err := s.redisService.GetCurrentQPS()
	if err != nil {
		s.logger.Error("获取当前QPS失败", zap.Error(err))
		currentQPS = 0
	}

	// 获取活跃用户数（这里简化处理，实际可以通过Redis统计最近活跃的用户）
	activeUsers := int64(0) // TODO: 实现活跃用户统计

	return &models.RealtimeStats{
		CurrentQPS:      currentQPS,
		AvgResponseTime: 0, // TODO: 实现响应时间统计
		ActiveUsers:     activeUsers,
		Timestamp:       time.Now().Unix(),
	}, nil
}

// ValidateAuthRequest 验证认证请求参数
func (s *AuthService) ValidateAuthRequest(req *models.AuthRequest) error {
	if req.User == "" {
		return fmt.Errorf("username is required")
	}
	if req.Pass == "" {
		return fmt.Errorf("password is required")
	}
	if req.Protocol == "" {
		return fmt.Errorf("protocol is required")
	}

	// 验证协议类型
	validProtocols := map[string]bool{
		"pop3": true,
		"imap": true,
		"smtp": true,
	}
	if !validProtocols[strings.ToLower(req.Protocol)] {
		return fmt.Errorf("invalid protocol: %s", req.Protocol)
	}

	return nil
}

// ModifyAdminPassword 修改管理员密码
func (s *AuthService) ModifyAdminPassword(username, oldPassword, newPassword string) error {
	// 验证旧密码
	admin, err := s.dbService.GetAdminByUsername(username)
	if err != nil {
		return fmt.Errorf("获取管理员信息失败: %v", err)
	}

	if !s.dbService.CheckAdminPassword(admin, oldPassword) {
		return fmt.Errorf("旧密码错误")
	}

	// 更新密码
	err = s.dbService.UpdateAdminPassword(username, newPassword)
	if err != nil {
		return fmt.Errorf("更新密码失败: %v", err)
	}

	return nil
}

// CreateAdmin 创建管理员账户
func (s *AuthService) CreateAdmin(username, password string, enabled *bool) (*models.Admin, error) {
	// 检查用户名是否已存在
	existingAdmin, _ := s.dbService.GetAdminByUsername(username)
	if existingAdmin != nil {
		return nil, fmt.Errorf("用户名已存在")
	}

	// 创建管理员请求
	req := &models.AdminCreateRequest{
		Username: username,
		Password: password,
		Enabled:  enabled,
	}

	err := s.dbService.CreateAdmin(req)
	if err != nil {
		return nil, fmt.Errorf("创建管理员失败: %v", err)
	}

	// 返回创建的管理员信息
	admin, err := s.dbService.GetAdminByUsername(username)
	if err != nil {
		return nil, fmt.Errorf("获取创建的管理员信息失败: %v", err)
	}

	return admin, nil
}

// UpdateAdmin 更新管理员账户
func (s *AuthService) UpdateAdmin(username, password string, enabled *bool) (*models.Admin, error) {
	// 检查管理员是否存在
	_, err := s.dbService.GetAdminByUsername(username)
	if err != nil {
		return nil, fmt.Errorf("管理员不存在")
	}

	// 创建更新请求
	req := &models.AdminUpdateRequest{}
	if password != "" {
		req.Password = &password
	}
	if enabled != nil {
		req.Enabled = enabled
	}

	err = s.dbService.UpdateAdmin(username, req)
	if err != nil {
		return nil, fmt.Errorf("更新管理员失败: %v", err)
	}

	// 返回更新后的管理员信息
	admin, err := s.dbService.GetAdminByUsername(username)
	if err != nil {
		return nil, fmt.Errorf("获取更新后的管理员信息失败: %v", err)
	}

	return admin, nil
}

// DeleteAdmin 删除管理员账户
func (s *AuthService) DeleteAdmin(username string) error {
	// 检查管理员是否存在
	_, err := s.dbService.GetAdminByUsername(username)
	if err != nil {
		return fmt.Errorf("管理员不存在")
	}

	err = s.dbService.DeleteAdmin(username)
	if err != nil {
		return fmt.Errorf("删除管理员失败: %v", err)
	}

	return nil
}

// GetAdminByUsername 根据用户名获取管理员信息
func (s *AuthService) GetAdminByUsername(username string) (*models.Admin, error) {
	return s.dbService.GetAdminByUsername(username)
}
