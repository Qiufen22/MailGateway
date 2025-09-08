package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// CORSConfig CORS配置
type CORSConfig struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	ExposeHeaders    []string
	AllowCredentials bool
	MaxAge           time.Duration
}

// DefaultCORSConfig 默认CORS配置
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Length", "Content-Type", "Authorization", "X-Requested-With", "X-Request-ID"},
		ExposeHeaders:    []string{"Content-Length", "X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}
}

// CORSMiddleware CORS中间件
func CORSMiddleware(config ...CORSConfig) gin.HandlerFunc {
	cfg := DefaultCORSConfig()
	if len(config) > 0 {
		cfg = config[0]
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// 检查是否允许该来源
		allowedOrigin := ""
		for _, allowOrigin := range cfg.AllowOrigins {
			if allowOrigin == "*" || allowOrigin == origin {
				allowedOrigin = allowOrigin
				break
			}
		}

		if allowedOrigin != "" {
			if allowedOrigin == "*" {
				c.Header("Access-Control-Allow-Origin", "*")
			} else {
				c.Header("Access-Control-Allow-Origin", origin)
			}
		}

		// 设置其他CORS头
		if len(cfg.AllowMethods) > 0 {
			c.Header("Access-Control-Allow-Methods", strings.Join(cfg.AllowMethods, ", "))
		}

		if len(cfg.AllowHeaders) > 0 {
			c.Header("Access-Control-Allow-Headers", strings.Join(cfg.AllowHeaders, ", "))
		}

		if len(cfg.ExposeHeaders) > 0 {
			c.Header("Access-Control-Expose-Headers", strings.Join(cfg.ExposeHeaders, ", "))
		}

		if cfg.AllowCredentials {
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		if cfg.MaxAge > 0 {
			c.Header("Access-Control-Max-Age", string(rune(int(cfg.MaxAge.Seconds()))))
		}

		// 处理预检请求
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// SecurityHeadersMiddleware 安全头中间件
func SecurityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 设置安全相关的HTTP头
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; img-src 'self' data:")

		c.Next()
	}
}

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	RequestsPerSecond int
	BurstSize         int
}

// RateLimitMiddleware 简单的限流中间件（基于内存）
// 注意：这是一个简化版本，生产环境建议使用Redis实现分布式限流
func RateLimitMiddleware(config RateLimitConfig) gin.HandlerFunc {
	// 这里使用简单的令牌桶算法
	// 实际生产环境应该使用更完善的限流库

	return func(c *gin.Context) {
		// 获取客户端IP
		clientIP := c.ClientIP()

		// 这里应该实现令牌桶或滑动窗口算法
		// 为了简化，这里只是一个占位符
		_ = clientIP

		// TODO: 实现真正的限流逻辑
		// 如果超过限制，返回429状态码
		// c.JSON(http.StatusTooManyRequests, ErrorResponse{
		//     Error:   "Too Many Requests",
		//     Message: "Rate limit exceeded",
		//     Code:    http.StatusTooManyRequests,
		// })
		// c.Abort()
		// return

		c.Next()
	}
}
