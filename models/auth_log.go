package models

import "time"

// AuthLog 认证日志数据结构
type AuthLog struct {
	ID           int       `json:"id" db:"id"`
	Username     string    `json:"username" db:"username"`           // 认证用户名 (Auth-User)
	IPAddress    string    `json:"ip_address" db:"ip_address"`       // 客户端IP地址
	Protocol     string    `json:"protocol" db:"protocol"`           // 认证协议 (pop3, imap, smtp)
	Result       string    `json:"result" db:"result"`               // 认证结果 (success, password_error, totp_error, user_not_found, user_disabled, etc.)
	ResultDetail string    `json:"result_detail" db:"result_detail"` // 认证结果详情
	ClientIP     string    `json:"client_ip" db:"client_ip"`         // 原始客户端IP (Client-IP header)
	UserAgent    string    `json:"user_agent" db:"user_agent"`       // 用户代理
	CreatedAt    time.Time `json:"created_at" db:"created_at"`       // 认证时间
}

// AuthLogSimple 认证日志简化结构（用于API返回）
type AuthLogSimple struct {
	Username     string `json:"username"`      // 认证用户名
	IPAddress    string `json:"ip_address"`    // IP地址
	Protocol     string `json:"protocol"`      // 认证协议
	Result       string `json:"result"`        // 认证结果
	ResultDetail string `json:"result_detail"` // 认证结果详情
	CreatedAt    string `json:"created_at"`    // 认证时间
}

// AuthLogGetRequest 认证日志查询请求
type AuthLogGetRequest struct {
	Page      int    `form:"page" binding:"min=1"`          // 页码
	Limit     int    `form:"limit" binding:"min=1,max=100"` // 每页数量
	Username  string `form:"username"`                      // 用户名筛选
	Protocol  string `form:"protocol"`                      // 协议筛选
	Result    string `form:"result"`                        // 结果筛选
	IPAddress string `form:"ip_address"`                    // IP地址筛选
	StartTime string `form:"start_time"`                    // 开始时间
	EndTime   string `form:"end_time"`                      // 结束时间
}

// AuthLogGetResponse 认证日志查询响应
type AuthLogGetResponse struct {
	Success bool         `json:"success"`
	Message string       `json:"message"`
	Data    *AuthLogData `json:"data,omitempty"`
}

// AuthLogData 认证日志数据
type AuthLogData struct {
	Logs       []AuthLogSimple `json:"logs"`
	Total      int             `json:"total"`
	Page       int             `json:"page"`
	Limit      int             `json:"limit"`
	TotalPages int             `json:"total_pages"`
}

// AuthLogStats 认证日志统计
type AuthLogStats struct {
	TotalAuth   int `json:"total_auth"`   // 总认证次数
	SuccessAuth int `json:"success_auth"` // 成功认证次数
	FailedAuth  int `json:"failed_auth"`  // 失败认证次数
	TodayAuth   int `json:"today_auth"`   // 今日认证次数
	UniqueUsers int `json:"unique_users"` // 唯一用户数
	UniqueIPs   int `json:"unique_ips"`   // 唯一IP数
}

// AuthLogStatsResponse 认证日志统计响应
type AuthLogStatsResponse struct {
	Success bool          `json:"success"`
	Message string        `json:"message"`
	Data    *AuthLogStats `json:"data,omitempty"`
}

// DashboardStats 仪表盘统计信息
type DashboardStats struct {
	TodayAuthCount     int     `json:"today_auth_count"`     // 今日认证总数
	TodayAuthChange    float64 `json:"today_auth_change"`    // 较昨日变化百分比
	TodaySuccessCount  int     `json:"today_success_count"`  // 今日成功认证数
	TodaySuccessChange float64 `json:"today_success_change"` // 较昨日变化百分比
	TodayFailedCount   int     `json:"today_failed_count"`   // 今日失败认证数
	TodayFailedChange  float64 `json:"today_failed_change"`  // 较昨日变化百分比
}

// DashboardStatsResponse 仪表盘统计响应
type DashboardStatsResponse struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    *DashboardStats `json:"data,omitempty"`
}
