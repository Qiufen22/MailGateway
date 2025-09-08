package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ErrorResponse 错误响应结构
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// ErrorHandlerMiddleware 错误处理中间件
func ErrorHandlerMiddleware(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// 处理panic和错误
		if len(c.Errors) > 0 {
			err := c.Errors.Last()

			// 记录错误日志
			logger.Error("请求错误",
				zap.String("path", c.Request.URL.Path),
				zap.String("method", c.Request.Method),
				zap.String("client_ip", c.ClientIP()),
				zap.Error(err.Err),
				zap.String("request_id", getRequestID(c)),
			)

			// 根据错误类型返回不同的HTTP状态码
			statusCode := http.StatusInternalServerError
			errorMessage := "内部服务器错误"

			// 可以根据错误类型自定义状态码和消息
			switch err.Type {
			case gin.ErrorTypeBind:
				statusCode = http.StatusBadRequest
				errorMessage = "无效的请求参数"
			case gin.ErrorTypePublic:
				statusCode = http.StatusBadRequest
				errorMessage = err.Error()
			default:
				statusCode = http.StatusInternalServerError
				errorMessage = "内部服务器错误"
			}

			c.JSON(statusCode, ErrorResponse{
				Error:   errorMessage,
				Message: err.Error(),
				Code:    statusCode,
			})
			c.Abort()
		}
	}
}

// RecoveryMiddleware 恢复中间件，处理panic
func RecoveryMiddleware(logger *zap.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		// 记录panic日志
		logger.Error("恢复panic",
			zap.String("path", c.Request.URL.Path),
			zap.String("method", c.Request.Method),
			zap.String("client_ip", c.ClientIP()),
			zap.Any("panic", recovered),
			zap.String("request_id", getRequestID(c)),
		)

		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "内部服务器错误",
			Message: "发生了意外错误",
			Code:    http.StatusInternalServerError,
		})
		c.Abort()
	})
}

// getRequestID 从上下文获取请求ID
func getRequestID(c *gin.Context) string {
	if requestID, exists := c.Get("request_id"); exists {
		if id, ok := requestID.(string); ok {
			return id
		}
	}
	return "unknown"
}

// NotFoundHandler 404处理器
func NotFoundHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "未找到",
			Message: "请求的资源未找到",
			Code:    http.StatusNotFound,
		})
	}
}

// MethodNotAllowedHandler 405处理器
func MethodNotAllowedHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusMethodNotAllowed, ErrorResponse{
			Error:   "方法不允许",
			Message: "此资源不允许使用该请求方法",
			Code:    http.StatusMethodNotAllowed,
		})
	}
}
