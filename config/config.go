package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config 应用配置结构
type Config struct {
	Server     ServerConfig     `mapstructure:"server"`
	AuthServer AuthServerConfig `mapstructure:"auth_server"`
	Database   DatabaseConfig   `mapstructure:"database"`
	Redis      RedisConfig      `mapstructure:"redis"`
	Auth       AuthConfig       `mapstructure:"auth"`
	JWT        JWTConfig        `mapstructure:"jwt"`
	TOTP       TOTPConfig       `mapstructure:"totp"`
	Admin      AdminConfig      `mapstructure:"admin"`
	Log        LogConfig        `mapstructure:"log"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port string `mapstructure:"port"`
	Mode string `mapstructure:"mode"` // debug, release, test
}

// AuthServerConfig 认证服务器配置
type AuthServerConfig struct {
	Host string `mapstructure:"host"`
	Port string `mapstructure:"port"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Host            string        `mapstructure:"host"`
	Port            string        `mapstructure:"port"`
	User            string        `mapstructure:"user"`
	Password        string        `mapstructure:"password"`
	Database        string        `mapstructure:"database"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

// RedisConfig Redis配置
type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     string `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// JWTConfig JWT配置
type JWTConfig struct {
	Secret            string        `mapstructure:"secret"`             // JWT密钥
	Expiration        time.Duration `mapstructure:"expiration"`         // 访问token过期时间
	RefreshExpiration time.Duration `mapstructure:"refresh_expiration"` // 刷新token过期时间
}

// AuthConfig 认证配置
type AuthConfig struct {
	MaxFailAttempts int           `mapstructure:"max_fail_attempts"` // 最大失败次数
	FailTTL         time.Duration `mapstructure:"fail_ttl"`          // 失败计数过期时间
	LockTTL         time.Duration `mapstructure:"lock_ttl"`          // 锁定时间
}

// TOTPConfig TOTP配置
type TOTPConfig struct {
	Issuer       string `mapstructure:"issuer"`
	SecretLength int    `mapstructure:"secret_length"`
}

// AdminConfig 管理员配置
type AdminConfig struct {
	DefaultUsername string `mapstructure:"default_username"`
	DefaultPassword string `mapstructure:"default_password"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level    string `mapstructure:"level"`
	FilePath string `mapstructure:"file_path"`
}

// LoadConfig 使用Viper加载配置
// 支持配置文件、环境变量和默认值
func LoadConfig() (*Config, error) {
	// 初始化Viper
	v := viper.New()

	// 设置配置文件名和类型
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./conf")
	v.AddConfigPath("./config")

	// 设置环境变量
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// 设置默认值
	setDefaults(v)

	// 读取配置文件（如果存在）
	if err := v.ReadInConfig(); err != nil {
		// 如果配置文件不存在，使用默认值和环境变量
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// 解析配置到结构体
	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// 手动处理时间类型（Viper对time.Duration的支持有限）
	config.Database.ConnMaxLifetime = v.GetDuration("database.conn_max_lifetime")
	config.Auth.FailTTL = v.GetDuration("auth.fail_ttl")
	config.Auth.LockTTL = v.GetDuration("auth.lock_ttl")

	// 验证必要的配置项
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &config, nil
}

// setDefaults 设置默认配置值
func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.host", "127.0.0.1")
	v.SetDefault("server.port", "8089")
	v.SetDefault("server.mode", "debug")

	// Auth Server defaults
	v.SetDefault("auth_server.host", "127.0.0.1")
	v.SetDefault("auth_server.port", "8090")

	// Database defaults - 用户必须在配置文件中提供数据库连接信息
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", "3306")
	v.SetDefault("database.user", "")
	v.SetDefault("database.password", "")
	v.SetDefault("database.database", "")
	v.SetDefault("database.max_open_conns", 100)
	v.SetDefault("database.max_idle_conns", 10)
	v.SetDefault("database.conn_max_lifetime", "3m")

	// Redis defaults
	v.SetDefault("redis.host", "localhost")
	v.SetDefault("redis.port", "6379")
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)

	// Auth defaults
	v.SetDefault("auth.max_fail_attempts", 5)
	v.SetDefault("auth.fail_ttl", "60s")
	v.SetDefault("auth.lock_ttl", "5m")

	// JWT defaults
	v.SetDefault("jwt.secret", "your-secret-key-change-this-in-production")
	v.SetDefault("jwt.expiration", "24h")
	v.SetDefault("jwt.refresh_expiration", "168h") // 7天

	// TOTP defaults
	v.SetDefault("totp.issuer", "mail-auth")
	v.SetDefault("totp.secret_length", 16)

	// Admin defaults
	v.SetDefault("admin.default_username", "admin")
	v.SetDefault("admin.default_password", "admin123")

	// Log defaults
	v.SetDefault("log.level", "info")
	v.SetDefault("log.file_path", "logs/app.log")
}

// GetDSN 获取数据库连接字符串
func (c *Config) GetDSN() string {
	return c.Database.User + ":" + c.Database.Password + "@tcp(" + c.Database.Host + ":" + c.Database.Port + ")/" + c.Database.Database + "?charset=utf8mb4&parseTime=true&loc=Local"
}

// GetRedisAddr 获取Redis地址
func (c *Config) GetRedisAddr() string {
	return fmt.Sprintf("%s:%s", c.Redis.Host, c.Redis.Port)
}

// GetServerAddr 获取服务器监听地址
func (c *Config) GetServerAddr() string {
	return c.Server.Host + ":" + c.Server.Port
}

// GetAuthServerAddr 获取认证服务器监听地址
func (c *Config) GetAuthServerAddr() string {
	return c.AuthServer.Host + ":" + c.AuthServer.Port
}

// validateConfig 验证配置的必要项
func validateConfig(config *Config) error {
	// 验证数据库配置
	if config.Database.User == "" {
		return fmt.Errorf("database user is required")
	}
	if config.Database.Password == "" {
		return fmt.Errorf("database password is required")
	}
	if config.Database.Database == "" {
		return fmt.Errorf("database name is required")
	}
	if config.Database.Host == "" {
		return fmt.Errorf("database host is required")
	}

	// 验证服务器配置
	if config.Server.Port == "" {
		return fmt.Errorf("server port is required")
	}

	return nil
}
