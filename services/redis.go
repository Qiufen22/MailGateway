package services

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"MailGateway/config"
	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

// RedisService Redis服务
type RedisService struct {
	client *redis.Client
	logger *zap.Logger
	config *config.Config
}

// NewRedisService 创建Redis服务实例
func NewRedisService(cfg *config.Config, logger *zap.Logger) (*RedisService, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.GetRedisAddr(),
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	// 测试连接
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	logger.Info("Redis连接成功")

	return &RedisService{
		client: client,
		logger: logger,
		config: cfg,
	}, nil
}

// Close 关闭Redis连接
func (s *RedisService) Close() error {
	return s.client.Close()
}

// IsUserLocked 检查用户是否被锁定
func (s *RedisService) IsUserLocked(username string) (bool, error) {
	ctx := context.Background()
	lockKey := fmt.Sprintf("user_lock:%s", username)

	exists, err := s.client.Exists(ctx, lockKey).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check user lock: %w", err)
	}

	return exists > 0, nil
}

// RecordLoginFailure 记录登录失败
func (s *RedisService) RecordLoginFailure(username string) error {
	ctx := context.Background()
	failKey := fmt.Sprintf("login_fail:%s", username)
	lockKey := fmt.Sprintf("user_lock:%s", username)

	// 使用Lua脚本确保原子性
	luaScript := `
		local fail_key = KEYS[1]
		local lock_key = KEYS[2]
		local max_fails = tonumber(ARGV[1])
		local fail_ttl = tonumber(ARGV[2])
		local lock_ttl = tonumber(ARGV[3])
		
		-- 增加失败计数
		local current_fails = redis.call('INCR', fail_key)
		
		-- 设置失败计数的过期时间
		if current_fails == 1 then
			redis.call('EXPIRE', fail_key, fail_ttl)
		end
		
		-- 如果失败次数达到上限，锁定用户
		if current_fails >= max_fails then
			redis.call('SETEX', lock_key, lock_ttl, '1')
			return 1  -- 用户被锁定
		end
		
		return 0  -- 用户未被锁定
	`

	result, err := s.client.Eval(ctx, luaScript, []string{failKey, lockKey},
		s.config.Auth.MaxFailAttempts,
		int(s.config.Auth.FailTTL.Seconds()),
		int(s.config.Auth.LockTTL.Seconds())).Result()

	if err != nil {
		return fmt.Errorf("failed to record login failure: %w", err)
	}

	if result.(int64) == 1 {
		s.logger.Warn("用户因失败次数过多被锁定", zap.String("username", username))
	}

	return nil
}

// ClearLoginFailures 清除登录失败记录
func (s *RedisService) ClearLoginFailures(username string) error {
	ctx := context.Background()
	failKey := fmt.Sprintf("login_fail:%s", username)

	err := s.client.Del(ctx, failKey).Err()
	if err != nil {
		return fmt.Errorf("failed to clear login failures: %w", err)
	}

	return nil
}

// UnlockUser 解锁用户
func (s *RedisService) UnlockUser(username string) error {
	ctx := context.Background()
	lockKey := fmt.Sprintf("user_lock:%s", username)
	failKey := fmt.Sprintf("login_fail:%s", username)

	// 删除锁定和失败计数
	err := s.client.Del(ctx, lockKey, failKey).Err()
	if err != nil {
		return fmt.Errorf("failed to unlock user: %w", err)
	}

	s.logger.Info("用户已解锁", zap.String("username", username))
	return nil
}

// GetLoginFailureCount 获取登录失败次数
func (s *RedisService) GetLoginFailureCount(username string) (int, error) {
	ctx := context.Background()
	failKey := fmt.Sprintf("login_fail:%s", username)

	count, err := s.client.Get(ctx, failKey).Result()
	if err != nil {
		if err == redis.Nil {
			return 0, nil // 没有失败记录
		}
		return 0, fmt.Errorf("failed to get login failure count: %w", err)
	}

	failCount, err := strconv.Atoi(count)
	if err != nil {
		return 0, fmt.Errorf("failed to parse failure count: %w", err)
	}

	return failCount, nil
}

// GetLockedUsers 获取所有被锁定的用户
func (s *RedisService) GetLockedUsers() ([]string, error) {
	ctx := context.Background()
	pattern := "user_lock:*"

	keys, err := s.client.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get locked users: %w", err)
	}

	var users []string
	for _, key := range keys {
		// 提取用户名（去掉"user_lock:"前缀）
		username := key[10:] // len("user_lock:") = 10
		users = append(users, username)
	}

	return users, nil
}

// SetUserPasswordCache 缓存用户密码（用于性能优化）
func (s *RedisService) SetUserPasswordCache(username, password string, ttl time.Duration) error {
	ctx := context.Background()
	cacheKey := fmt.Sprintf("user_pwd:%s", username)

	err := s.client.Set(ctx, cacheKey, password, ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to cache user password: %w", err)
	}

	return nil
}

// GetUserPasswordCache 获取缓存的用户密码
func (s *RedisService) GetUserPasswordCache(username string) (string, error) {
	ctx := context.Background()
	cacheKey := fmt.Sprintf("user_pwd:%s", username)

	password, err := s.client.Get(ctx, cacheKey).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil // 缓存不存在
		}
		return "", fmt.Errorf("failed to get cached password: %w", err)
	}

	return password, nil
}

// DeleteUserPasswordCache 删除用户密码缓存
func (s *RedisService) DeleteUserPasswordCache(username string) error {
	ctx := context.Background()
	cacheKey := fmt.Sprintf("user_pwd:%s", username)

	err := s.client.Del(ctx, cacheKey).Err()
	if err != nil {
		return fmt.Errorf("failed to delete password cache: %w", err)
	}

	return nil
}

// IncrementQPSCounter 增加QPS计数器
func (s *RedisService) IncrementQPSCounter() error {
	ctx := context.Background()
	currentSecond := time.Now().Unix()
	key := fmt.Sprintf("qps:%d", currentSecond)

	// 增加计数并设置1分钟过期时间
	err := s.client.Incr(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to increment QPS counter: %w", err)
	}

	// 设置过期时间
	s.client.Expire(ctx, key, time.Minute)

	return nil
}

// GetCurrentQPS 获取当前QPS
func (s *RedisService) GetCurrentQPS() (float64, error) {
	ctx := context.Background()
	now := time.Now().Unix()

	// 获取最近10秒的请求数
	var totalRequests int64
	for i := int64(0); i < 10; i++ {
		key := fmt.Sprintf("qps:%d", now-i)
		count, err := s.client.Get(ctx, key).Result()
		if err != nil && err != redis.Nil {
			return 0, fmt.Errorf("failed to get QPS data: %w", err)
		}
		if err != redis.Nil {
			if c, parseErr := strconv.ParseInt(count, 10, 64); parseErr == nil {
				totalRequests += c
			}
		}
	}

	// 计算平均QPS（最近10秒）
	return float64(totalRequests) / 10.0, nil
}

// Set 设置键值对
func (s *RedisService) Set(key string, value interface{}, expiration time.Duration) error {
	ctx := context.Background()
	err := s.client.Set(ctx, key, value, expiration).Err()
	if err != nil {
		return fmt.Errorf("failed to set key %s: %w", key, err)
	}
	return nil
}

// Get 获取键值
func (s *RedisService) Get(key string) (string, error) {
	ctx := context.Background()
	value, err := s.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", fmt.Errorf("key %s not found", key)
		}
		return "", fmt.Errorf("failed to get key %s: %w", key, err)
	}
	return value, nil
}

// Delete 删除键（支持通配符模式）
func (s *RedisService) Delete(key string) error {
	ctx := context.Background()

	// 检查是否包含通配符
	if strings.Contains(key, "*") || strings.Contains(key, "?") || strings.Contains(key, "[") {
		// 使用KEYS命令查找匹配的键
		keys, err := s.client.Keys(ctx, key).Result()
		if err != nil {
			return fmt.Errorf("failed to find keys with pattern %s: %w", key, err)
		}

		// 如果找到匹配的键，批量删除
		if len(keys) > 0 {
			err = s.client.Del(ctx, keys...).Err()
			if err != nil {
				return fmt.Errorf("failed to delete keys with pattern %s: %w", key, err)
			}
			s.logger.Info("批量删除缓存键", zap.String("pattern", key), zap.Int("count", len(keys)))
		}
		return nil
	}

	// 普通键删除
	err := s.client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to delete key %s: %w", key, err)
	}
	return nil
}

// SetBlacklistCache 缓存黑名单数据
func (s *RedisService) SetBlacklistCache(page, limit int, search, data string, ttl time.Duration) error {
	ctx := context.Background()
	cacheKey := fmt.Sprintf("blacklist:%d:%d:%s", page, limit, search)

	err := s.client.Set(ctx, cacheKey, data, ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to cache blacklist data: %w", err)
	}

	return nil
}

// GetBlacklistCache 获取缓存的黑名单数据
func (s *RedisService) GetBlacklistCache(page, limit int, search string) (string, error) {
	ctx := context.Background()
	cacheKey := fmt.Sprintf("blacklist:%d:%d:%s", page, limit, search)

	data, err := s.client.Get(ctx, cacheKey).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil // 缓存不存在
		}
		return "", fmt.Errorf("failed to get cached blacklist data: %w", err)
	}

	return data, nil
}

// ClearBlacklistCache 清除黑名单缓存
func (s *RedisService) ClearBlacklistCache() error {
	ctx := context.Background()
	pattern := "blacklist:*"

	keys, err := s.client.Keys(ctx, pattern).Result()
	if err != nil {
		return fmt.Errorf("failed to get blacklist cache keys: %w", err)
	}

	if len(keys) > 0 {
		err = s.client.Del(ctx, keys...).Err()
		if err != nil {
			return fmt.Errorf("failed to clear blacklist cache: %w", err)
		}
	}

	return nil
}
