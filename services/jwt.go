package services

import (
	"errors"
	"time"

	"MailGateway/config"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

// JWTService JWT服务
type JWTService struct {
	config *config.Config
	logger *zap.Logger
}

// Claims JWT声明
type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	UserType string `json:"user_type"` // 用户类型：admin/user
	jwt.RegisteredClaims
}

// NewJWTService 创建JWT服务实例
func NewJWTService(cfg *config.Config, logger *zap.Logger) *JWTService {
	return &JWTService{
		config: cfg,
		logger: logger,
	}
}

// GenerateToken 生成JWT token
func (j *JWTService) GenerateToken(userID, username, userType string) (string, error) {
	// 设置token过期时间
	expirationTime := time.Now().Add(j.getTokenExpiration())

	// 创建声明
	claims := &Claims{
		UserID:   userID,
		Username: username,
		UserType: userType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "MailGateway",
		},
	}

	// 创建token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// 使用密钥签名token
	tokenString, err := token.SignedString([]byte(j.getJWTSecret()))
	if err != nil {
		j.logger.Error("生成JWT token失败", zap.Error(err))
		return "", err
	}

	j.logger.Info("JWT token已生成",
		zap.String("username", username),
		zap.String("user_id", userID),
		zap.String("user_type", userType),
		zap.String("operation_type", "token_generate"))

	return tokenString, nil
}

// ValidateToken 验证JWT token
func (j *JWTService) ValidateToken(tokenString string) (*Claims, error) {
	// 解析token
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// 验证签名方法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("无效的签名方法")
		}
		return []byte(j.getJWTSecret()), nil
	})

	if err != nil {
		j.logger.Warn("JWT token验证失败", zap.Error(err))
		return nil, err
	}

	// 检查token是否有效
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		// 检查token是否过期
		if claims.ExpiresAt.Time.Before(time.Now()) {
			j.logger.Warn("JWT token已过期", zap.String("username", claims.Username))
			return nil, errors.New("token已过期")
		}

		j.logger.Debug("JWT token验证成功",
			zap.String("username", claims.Username),
			zap.String("user_id", claims.UserID))

		return claims, nil
	}

	j.logger.Warn("JWT token无效")
	return nil, errors.New("无效的token")
}

// GenerateRefreshToken 生成刷新token
func (j *JWTService) GenerateRefreshToken(userID, username, userType string) (string, error) {
	// 设置刷新token过期时间（通常比访问token更长）
	expirationTime := time.Now().Add(j.getRefreshTokenExpiration())

	// 创建声明
	claims := &Claims{
		UserID:   userID,
		Username: username,
		UserType: userType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "MailGateway-Refresh",
		},
	}

	// 创建token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// 使用密钥签名token
	tokenString, err := token.SignedString([]byte(j.getJWTSecret()))
	if err != nil {
		j.logger.Error("生成刷新token失败", zap.Error(err))
		return "", err
	}

	j.logger.Info("刷新token已生成",
		zap.String("username", username),
		zap.String("user_id", userID),
		zap.String("user_type", userType),
		zap.String("operation_type", "refresh_token_generate"))

	return tokenString, nil
}

// RefreshToken 刷新JWT token
func (j *JWTService) RefreshToken(tokenString string) (string, error) {
	// 验证当前token
	claims, err := j.ValidateToken(tokenString)
	if err != nil {
		return "", err
	}

	// 生成新token
	newToken, err := j.GenerateToken(claims.UserID, claims.Username, claims.UserType)
	if err != nil {
		return "", err
	}

	j.logger.Info("JWT token已刷新",
		zap.String("username", claims.Username),
		zap.String("user_id", claims.UserID),
		zap.String("operation_type", "token_refresh"))

	return newToken, nil
}

// getJWTSecret 获取JWT密钥
func (j *JWTService) getJWTSecret() string {
	// 从配置中获取JWT密钥，如果没有配置则使用默认值
	if j.config.JWT.Secret != "" {
		return j.config.JWT.Secret
	}
	// 默认密钥（生产环境中应该从环境变量或配置文件中获取）
	return "your-secret-key-change-this-in-production"
}

// getTokenExpiration 获取token过期时间
func (j *JWTService) getTokenExpiration() time.Duration {
	if j.config.JWT.Expiration > 0 {
		return j.config.JWT.Expiration
	}
	// 默认24小时
	return 24 * time.Hour
}

// getRefreshTokenExpiration 获取刷新token过期时间
func (j *JWTService) getRefreshTokenExpiration() time.Duration {
	if j.config.JWT.RefreshExpiration > 0 {
		return j.config.JWT.RefreshExpiration
	}
	// 默认7天
	return 7 * 24 * time.Hour
}
