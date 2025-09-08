package services

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"time"

	"MailGateway/config"
	"MailGateway/models"

	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/skip2/go-qrcode"
	"go.uber.org/zap"
)

// TOTPService TOTP服务
type TOTPService struct {
	dbService *DatabaseService
	logger    *zap.Logger
	config    *config.Config
}

// NewTOTPService 创建TOTP服务实例
func NewTOTPService(dbService *DatabaseService, cfg *config.Config, logger *zap.Logger) *TOTPService {
	return &TOTPService{
		dbService: dbService,
		logger:    logger,
		config:    cfg,
	}
}

// getUserIDFromContext 从gin.Context中获取用户ID，如果没有则使用用户名
func (s *TOTPService) getUserIDFromContext(c *gin.Context, username string) string {
	if c != nil {
		if userID, exists := c.Get("user_id"); exists {
			if id, ok := userID.(string); ok {
				return id
			}
		}
	}
	// 如果没有JWT上下文，使用用户名作为user_id
	return username
}

// GenerateTOTPSecret 生成TOTP密钥
func (s *TOTPService) GenerateTOTPSecret() (string, error) {
	// 生成随机字节
	secretBytes := make([]byte, s.config.TOTP.SecretLength)
	_, err := rand.Read(secretBytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// 转换为Base32编码
	secret := base32.StdEncoding.EncodeToString(secretBytes)
	return secret, nil
}

// GenerateQRCode 生成TOTP二维码URL
func (s *TOTPService) GenerateQRCode(username, secret string) (string, error) {
	key, err := otp.NewKeyFromURL(fmt.Sprintf(
		"otpauth://totp/%s:%s?secret=%s&issuer=%s",
		s.config.TOTP.Issuer,
		username,
		secret,
		s.config.TOTP.Issuer,
	))
	if err != nil {
		return "", fmt.Errorf("failed to create TOTP key: %w", err)
	}

	return key.URL(), nil
}

// GenerateQRCodeImage 生成TOTP二维码图片的Base64编码
func (s *TOTPService) GenerateQRCodeImage(username, secret string) (string, error) {
	otpauthURL := fmt.Sprintf(
		"otpauth://totp/%s:%s?secret=%s&issuer=%s",
		s.config.TOTP.Issuer,
		username,
		secret,
		s.config.TOTP.Issuer,
	)

	// 生成二维码图片字节数据
	qrBytes, err := qrcode.Encode(otpauthURL, qrcode.Medium, 256)
	if err != nil {
		return "", fmt.Errorf("failed to generate QR code image: %w", err)
	}

	// 转换为Base64编码
	base64Image := base64.StdEncoding.EncodeToString(qrBytes)
	return fmt.Sprintf("data:image/png;base64,%s", base64Image), nil
}

// ValidateTOTPCode 验证TOTP代码
func (s *TOTPService) ValidateTOTPCode(username, code string) (bool, error) {
	// 获取用户信息
	user, err := s.dbService.GetUser(username)
	if err != nil {
		return false, fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return false, fmt.Errorf("user not found")
	}

	// 检查TOTP是否启用
	if !user.TOTPEnabled || user.TOTPSecret == "" {
		return false, fmt.Errorf("TOTP not enabled for user")
	}

	// 验证TOTP代码
	valid := totp.Validate(code, user.TOTPSecret)
	if !valid {
		// 允许时间偏差，检查前后30秒
		valid = totp.Validate(code, user.TOTPSecret) ||
			totp.Validate(code, user.TOTPSecret)
	}

	return valid, nil
}

// extractSecretFromQRCode 从二维码数据中提取TOTP密钥
func (s *TOTPService) extractSecretFromQRCode(qrData string) (string, error) {
	// 首先检查是否直接是otpauth URL
	if strings.HasPrefix(qrData, "otpauth://") {
		return s.parseSecretFromURL(qrData)
	}

	// 检查是否是data URL格式的base64图片
	if strings.HasPrefix(qrData, "data:image/") {
		// 提取base64部分
		parts := strings.Split(qrData, ",")
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid data URL format")
		}

		// 解码base64图片
		imageData, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return "", fmt.Errorf("failed to decode base64 image: %w", err)
		}

		// 尝试解析二维码图片
		return s.decodeQRCodeImage(imageData)
	}

	// 如果都不是，尝试直接作为base64解码
	imageData, err := base64.StdEncoding.DecodeString(qrData)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64 data: %w", err)
	}

	return s.decodeQRCodeImage(imageData)
}

// parseSecretFromURL 从otpauth URL中解析secret参数
func (s *TOTPService) parseSecretFromURL(otpauthURL string) (string, error) {
	parsedURL, err := url.Parse(otpauthURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse otpauth URL: %w", err)
	}

	queryParams := parsedURL.Query()
	secret := queryParams.Get("secret")
	if secret == "" {
		return "", fmt.Errorf("secret parameter not found in otpauth URL")
	}

	return secret, nil
}

// decodeQRCodeImage 解析二维码图片并提取URL
func (s *TOTPService) decodeQRCodeImage(imageData []byte) (string, error) {
	// 这里需要使用二维码解析库
	// 由于复杂性，我们先返回一个错误，建议使用totp_key字段
	return "", fmt.Errorf("QR code image parsing not implemented, please use totp_key field instead")
}

// ValidateAdminTOTPCode 验证管理员TOTP代码
func (s *TOTPService) ValidateAdminTOTPCode(username, code string) (bool, error) {
	// 获取管理员信息
	admin, err := s.dbService.GetAdmin(username)
	if err != nil {
		return false, fmt.Errorf("failed to get admin: %w", err)
	}
	if admin == nil {
		return false, fmt.Errorf("admin not found")
	}

	// 优先从totp_key获取密钥
	totpKey, err := s.dbService.GetSystemConfig("totp_key")
	if err != nil {
		s.logger.Error("获取系统TOTP密钥失败", zap.Error(err))
		return false, fmt.Errorf("获取系统TOTP密钥失败: %w", err)
	}

	if totpKey == "" {
		s.logger.Error("系统TOTP密钥未配置")
		return false, fmt.Errorf("系统TOTP密钥未配置")
	}

	// 验证TOTP代码，支持30秒的时间偏差
	valid, err := totp.ValidateCustom(code, totpKey, time.Now(), totp.ValidateOpts{
		Period:    30,
		Skew:      1,
		Digits:    6,
		Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		s.logger.Error("TOTP验证失败", zap.Error(err))
		return false, fmt.Errorf("TOTP验证失败: %w", err)
	}

	s.logger.Info("TOTP验证结果", zap.String("username", username), zap.Bool("valid", valid))
	return valid, nil
}

// GetCurrentTOTPCode 获取当前TOTP代码（用于测试）
func (s *TOTPService) GetCurrentTOTPCode(username string) (string, error) {
	// 获取用户信息
	user, err := s.dbService.GetUser(username)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return "", fmt.Errorf("user not found")
	}

	// 检查TOTP是否启用
	if !user.TOTPEnabled || user.TOTPSecret == "" {
		return "", fmt.Errorf("TOTP not enabled for user")
	}

	// 生成当前TOTP代码
	code, err := totp.GenerateCode(user.TOTPSecret, time.Now())
	if err != nil {
		return "", fmt.Errorf("failed to generate TOTP code: %w", err)
	}

	return code, nil
}

// EnableTOTPForUser 为用户启用TOTP
func (s *TOTPService) EnableTOTPForUser(c *gin.Context, username string) (string, string, error) {
	// 先获取用户信息，检查是否已有TOTP密钥
	user, err := s.dbService.GetUser(username)
	if err != nil {
		return "", "", fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return "", "", fmt.Errorf("用户不存在")
	}

	var secret string
	// 如果用户已有TOTP密钥且已启用，则保持不变
	if user.TOTPSecret != "" && user.TOTPEnabled {
		secret = user.TOTPSecret
		s.logger.Info("用户TOTP已启用，保持现有密钥",
			zap.String("username", username),
			zap.String("operation_type", "totp_enable"),
			zap.String("user_id", username))
	} else {
		// 如果没有密钥或未启用，则生成新密钥或使用现有密钥并启用
		if user.TOTPSecret != "" {
			// 有密钥但未启用，直接启用
			secret = user.TOTPSecret
			err = s.dbService.UpdateTOTPSecret(username, secret)
			if err != nil {
				return "", "", fmt.Errorf("failed to enable TOTP: %w", err)
			}
		} else {
			// 没有密钥，生成新的
			secret, err = s.GenerateTOTPSecret()
			if err != nil {
				return "", "", fmt.Errorf("failed to generate TOTP secret: %w", err)
			}
			// 更新数据库
			err = s.dbService.UpdateTOTPSecret(username, secret)
			if err != nil {
				return "", "", fmt.Errorf("failed to update TOTP secret: %w", err)
			}
		}
		s.logger.Info("用户TOTP已启用",
			zap.String("username", username),
			zap.String("operation_type", "totp_enable"),
			zap.String("user_id", s.getUserIDFromContext(c, username)))
	}

	// 生成二维码URL
	qrURL, err := s.GenerateQRCode(username, secret)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate QR code: %w", err)
	}

	return secret, qrURL, nil
}

// UpdateTOTPForUser 强制更新用户TOTP密钥
func (s *TOTPService) UpdateTOTPForUser(c *gin.Context, username string) (string, string, error) {
	// 先检查用户是否存在
	user, err := s.dbService.GetUser(username)
	if err != nil {
		return "", "", fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return "", "", fmt.Errorf("用户不存在")
	}

	// 生成新的TOTP密钥
	secret, err := s.GenerateTOTPSecret()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate TOTP secret: %w", err)
	}

	// 更新数据库（只更新密钥，不改变启用状态）
	err = s.dbService.UpdateTOTPSecretOnly(username, secret)
	if err != nil {
		return "", "", fmt.Errorf("failed to update TOTP secret: %w", err)
	}

	// 生成二维码URL
	qrURL, err := s.GenerateQRCode(username, secret)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate QR code: %w", err)
	}

	s.logger.Info("用户TOTP密钥已更新",
		zap.String("username", username),
		zap.String("operation_type", "totp_update"),
		zap.String("user_id", s.getUserIDFromContext(c, username)))
	return secret, qrURL, nil
}

// DisableTOTPForUser 为用户禁用TOTP
func (s *TOTPService) DisableTOTPForUser(c *gin.Context, username string) error {
	// 更新数据库，清空TOTP密钥并禁用
	falseValue := false
	req := &models.UserUpdateRequest{
		TOTPEnabled: &falseValue,
	}

	err := s.dbService.UpdateUser(username, req)
	if err != nil {
		return fmt.Errorf("failed to disable TOTP: %w", err)
	}

	s.logger.Info("用户TOTP已禁用",
		zap.String("username", username),
		zap.String("operation_type", "totp_disable"),
		zap.String("user_id", s.getUserIDFromContext(c, username)))
	return nil
}

// DeleteTOTPForUser 删除用户TOTP密钥
func (s *TOTPService) DeleteTOTPForUser(c *gin.Context, username string) error {
	// 先检查用户是否存在
	user, err := s.dbService.GetUser(username)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("用户不存在")
	}

	// 删除TOTP密钥并禁用
	err = s.dbService.DeleteTOTPSecret(username)
	if err != nil {
		return fmt.Errorf("failed to delete TOTP: %w", err)
	}

	s.logger.Info("用户TOTP已删除",
		zap.String("username", username),
		zap.String("operation_type", "totp_delete"),
		zap.String("user_id", s.getUserIDFromContext(c, username)))
	return nil
}

// InitializeAllUsersTOTP 为所有用户初始化TOTP（如果尚未启用）
func (s *TOTPService) InitializeAllUsersTOTP() error {
	// 获取所有用户
	users, err := s.dbService.GetAllUsers()
	if err != nil {
		return fmt.Errorf("failed to get all users: %w", err)
	}

	var initializedCount int
	for _, user := range users {
		// 如果用户还没有启用TOTP，则为其生成密钥
		if !user.TOTPEnabled || user.TOTPSecret == "" {
			secret, err := s.GenerateTOTPSecret()
			if err != nil {
				s.logger.Error("为用户生成TOTP密钥失败",
					zap.String("username", user.Username),
					zap.Error(err))
				continue
			}

			err = s.dbService.UpdateTOTPSecret(user.Username, secret)
			if err != nil {
				s.logger.Error("为用户更新TOTP密钥失败",
					zap.String("username", user.Username),
					zap.Error(err))
				continue
			}

			initializedCount++
			s.logger.Info("用户TOTP已初始化", zap.String("username", user.Username))
		}
	}

	s.logger.Info("TOTP初始化完成",
		zap.Int("total_users", len(users)),
		zap.Int("initialized_count", initializedCount))

	return nil
}

// GetTOTPStatus 获取用户TOTP状态
func (s *TOTPService) GetTOTPStatus(username string) (*models.TOTPResponse, error) {
	user, err := s.dbService.GetUser(username)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return &models.TOTPResponse{
			Valid:   false,
			Message: "User not found",
		}, nil
	}

	if !user.TOTPEnabled {
		return &models.TOTPResponse{
			Valid:   false,
			Message: "TOTP not enabled",
		}, nil
	}

	return &models.TOTPResponse{
		Valid:   true,
		Message: "TOTP enabled",
	}, nil
}

// VerifyTOTPAndUnlock 验证TOTP代码并解锁用户
func (s *TOTPService) VerifyTOTPAndUnlock(redisService *RedisService, username, code string) (*models.TOTPResponse, error) {
	// 验证TOTP代码
	valid, err := s.ValidateTOTPCode(username, code)
	if err != nil {
		return &models.TOTPResponse{
			Valid:   false,
			Message: fmt.Sprintf("TOTP validation error: %v", err),
		}, nil
	}

	if !valid {
		return &models.TOTPResponse{
			Valid:   false,
			Message: "Invalid TOTP code",
		}, nil
	}

	// 解锁用户
	err = redisService.UnlockUser(username)
	if err != nil {
		s.logger.Error("TOTP验证后解锁用户失败",
			zap.String("username", username),
			zap.Error(err))
		return &models.TOTPResponse{
			Valid:   false,
			Message: "Failed to unlock user",
		}, nil
	}

	s.logger.Info("用户通过TOTP解锁", zap.String("username", username))
	return &models.TOTPResponse{
		Valid:   true,
		Message: "User unlocked successfully",
	}, nil
}
