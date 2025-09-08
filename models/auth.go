package models

// AuthRequest Nginx认证请求
type AuthRequest struct {
	User     string `header:"Auth-User" binding:"required"`
	Pass     string `header:"Auth-Pass" binding:"required"`
	Protocol string `header:"Auth-Protocol" binding:"required,oneof=pop3 imap smtp"`
	ClientIP string `header:"Client-IP"`
	Method   string `header:"Auth-Method"`
}

// AuthResponse Nginx认证响应
type AuthResponse struct {
	Status string `header:"Auth-Status"`
	Server string `header:"Auth-Server,omitempty"`
	Port   string `header:"Auth-Port,omitempty"`
	Pass   string `header:"Auth-Pass,omitempty"`
}

// RealtimeStats 实时统计
type RealtimeStats struct {
	CurrentQPS      float64 `json:"current_qps"`
	AvgResponseTime float64 `json:"avg_response_time"`
	ActiveUsers     int64   `json:"active_users"`
	Timestamp       int64   `json:"timestamp"`
}

// TOTPRequest TOTP验证请求
type TOTPRequest struct {
	Username string `json:"username" binding:"required"`
	Code     string `json:"code" binding:"required,len=6"`
}

// TOTPResponse TOTP验证响应
type TOTPResponse struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message"`
}
