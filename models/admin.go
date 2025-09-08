package models

import "time"

// Admin 管理员用户模型
type Admin struct {
	ID         int       `json:"id" db:"id"`
	Username   string    `json:"username" db:"username"`
	Password   string    `json:"-" db:"password"` // 不在JSON中显示密码
	Enabled    bool      `json:"enabled" db:"enabled"`
	TOTPSecret string    `json:"-" db:"totp_secret"` // 不在JSON中显示TOTP密钥
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at,omitempty" db:"updated_at"`
}

// AdminLoginRequest 管理员登录请求
type AdminLoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	TOTPCode string `json:"totp_code,omitempty"` // 可选：TOTP验证码
}

// AdminLoginResponse 管理员登录响应
type AdminLoginResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	RequireTOTP  bool   `json:"require_totp,omitempty"` // 是否需要TOTP验证
}

// AdminCreateRequest 创建管理员请求
type AdminCreateRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Enabled  *bool  `json:"enabled"`
}

// AdminUpdateRequest 更新管理员请求
type AdminUpdateRequest struct {
	Password *string `json:"password,omitempty"`
	Enabled  *bool   `json:"enabled,omitempty"`
}

// AdminAddRequest 添加管理员请求
type AdminAddRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// AdminAddResponse 添加管理员响应
type AdminAddResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    *Admin `json:"data,omitempty"`
}

// AdminDisableRequest 禁用管理员请求
type AdminDisableRequest struct {
	Username string `json:"username" binding:"required"`
	Enabled  string `json:"enabled" binding:"required,oneof=true false"`
}

// AdminDisableResponse 禁用管理员响应
type AdminDisableResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// AdminDeleteRequest 删除管理员请求
type AdminDeleteRequest struct {
	Username string `json:"username" binding:"required"`
}

// AdminDeleteResponse 删除管理员响应
type AdminDeleteResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// AdminListResponse 获取管理员列表响应
type AdminListResponse struct {
	Success bool    `json:"success"`
	Message string  `json:"message"`
	Data    []Admin `json:"data,omitempty"`
}

// AdminEditRequest 修改管理员信息请求
type AdminEditRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// AdminEditResponse 修改管理员信息响应
type AdminEditResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// SystemPasswordModifyRequest 修改系统账户密码请求
type SystemPasswordModifyRequest struct {
	OldPassword    string `json:"old_password" binding:"required"`
	NewPassword    string `json:"new_password" binding:"required"`
	RepeatPassword string `json:"repeat_password" binding:"required"`
}

// SystemAccountEditRequest 编辑系统账户请求
type SystemAccountEditRequest struct {
	Action   string `json:"action" binding:"required"` // add, modify, delete, disable
	Username string `json:"username" binding:"required"`
	Password string `json:"password,omitempty"`
	Enabled  *bool  `json:"enabled,omitempty"`
}

// SystemAccountResponse 系统账户操作响应
type SystemAccountResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    *Admin `json:"data,omitempty"`
}

// SystemAccountListResponse 系统账户列表响应
type SystemAccountListResponse struct {
	Success bool    `json:"success"`
	Message string  `json:"message"`
	Data    []Admin `json:"data"`
	Total   int     `json:"total"`
}

// SystemConfigSaveRequest 系统配置保存请求
type SystemConfigSaveRequest struct {
	HTTPHTTPS string `json:"HTTP/HTTPS" binding:"required"`
	POP3      string `json:"POP3" binding:"required"`
	IMAP      string `json:"IMAP" binding:"required"`
}

// SystemConfigSaveResponse 系统配置保存响应
type SystemConfigSaveResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// SecurityConfigSaveRequest 安全配置保存请求
type SecurityConfigSaveRequest struct {
	TOTP     string `json:"totp" binding:"required,oneof=true false"`
	Count    string `json:"count,omitempty"`
	Locked   string `json:"locked" binding:"required,oneof=true false"`
	Errors   string `json:"errors,omitempty"`
	LockTime string `json:"locktime,omitempty"`
}

// SecurityConfigSaveResponse 安全配置保存响应
type SecurityConfigSaveResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// SecurityConfigData 安全配置数据
type SecurityConfigData struct {
	TOTP       string `json:"totp"`
	Count      string `json:"count,omitempty"`
	Locked     string `json:"locked"`
	Errors     string `json:"errors,omitempty"`
	LockTime   string `json:"locktime,omitempty"`
	TOTPSecret string `json:"totp_secret,omitempty"`
	TOTPKey    string `json:"totp_key,omitempty"`
}

// SecurityConfigGetResponse 安全配置获取响应
type SecurityConfigGetResponse struct {
	Success bool                `json:"success"`
	Message string              `json:"message"`
	Data    *SecurityConfigData `json:"data,omitempty"`
}

// SMTPConfigSaveRequest SMTP配置保存请求
type SMTPConfigSaveRequest struct {
	SMTPServer string `json:"smtp_server" binding:"required"`
	SMTPPort   string `json:"smtp_port" binding:"required"`
	Account    string `json:"account" binding:"required"`
	Password   string `json:"password" binding:"required"`
	AuthEnable string `json:"auth_enable" binding:"required"`
}

// SMTPConfigSaveResponse SMTP配置保存响应
type SMTPConfigSaveResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// SMTPConfigGetResponse SMTP配置获取响应
type SMTPConfigGetResponse struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	SMTPServer string `json:"smtp_server"`
	SMTPPort   string `json:"smtp_port"`
	Account    string `json:"account"`
	Password   string `json:"password"`
	AuthEnable string `json:"auth_enable"`
}

// OperationLog 操作日志数据结构
type OperationLog struct {
	ID        int    `json:"id" db:"id"`
	Username  string `json:"username" db:"username"`
	UserType  string `json:"user_type" db:"user_type"`   // admin 或 user
	Operation string `json:"operation" db:"operation"`   // 操作类型：登录、修改密码、添加用户等
	Resource  string `json:"resource" db:"resource"`     // 操作的资源：用户管理、系统配置等
	Details   string `json:"details" db:"details"`       // 操作详情
	IPAddress string `json:"ip_address" db:"ip_address"` // 客户端IP
	UserAgent string `json:"user_agent" db:"user_agent"` // 用户代理
	Status    string `json:"status" db:"status"`         // 操作状态：success、failed
	CreatedAt string `json:"created_at" db:"created_at"` // 操作时间
}

// OperationLogSimple 简化的操作日志结构（用于前端显示）
type OperationLogSimple struct {
	Operator       string `json:"operator"`        // 操作者
	OperationType  string `json:"operation_type"`  // 操作类型（中文，如：系统登录、密码修改）
	OperationEvent string `json:"operation_event"` // 操作事件（中文，如：登录成功、登录失败）
	IPAddress      string `json:"ip_address"`      // IP地址
	CreatedAt      string `json:"created_at"`      // 操作时间
}

// OperationLogGetRequest 操作日志获取请求
type OperationLogGetRequest struct {
	Page      int    `form:"page" binding:"min=1"`          // 页码
	Limit     int    `form:"limit" binding:"min=1,max=100"` // 每页数量
	Username  string `form:"username"`                      // 用户名筛选
	UserType  string `form:"user_type"`                     // 用户类型筛选
	Operation string `form:"operation"`                     // 操作类型筛选
	StartTime string `form:"start_time"`                    // 开始时间
	EndTime   string `form:"end_time"`                      // 结束时间
}

// OperationLogGetResponse 操作日志获取响应
type OperationLogGetResponse struct {
	Success bool              `json:"success"`
	Message string            `json:"message"`
	Data    *OperationLogData `json:"data,omitempty"`
}

// OperationLogData 操作日志数据
type OperationLogData struct {
	Logs       []OperationLogSimple `json:"logs"`
	Total      int                  `json:"total"`
	Page       int                  `json:"page"`
	Limit      int                  `json:"limit"`
	TotalPages int                  `json:"total_pages"`
}

// OperationTypeMapping 操作类型中文映射
type OperationTypeMapping struct {
	Login        string `json:"login"`
	Logout       string `json:"logout"`
	PasswordMod  string `json:"password_mod"`
	UserAdd      string `json:"user_add"`
	UserEdit     string `json:"user_edit"`
	UserDelete   string `json:"user_delete"`
	ConfigSave   string `json:"config_save"`
	BlacklistAdd string `json:"blacklist_add"`
	BlacklistDel string `json:"blacklist_del"`
}

// OperationEventMapping 操作事件中文映射
type OperationEventMapping struct {
	UserManagement  string `json:"user_management"`
	SystemConfig    string `json:"system_config"`
	SecurityConfig  string `json:"security_config"`
	SMTPConfig      string `json:"smtp_config"`
	BlacklistManage string `json:"blacklist_manage"`
	PasswordManage  string `json:"password_manage"`
}

// AuthConfigGetResponse 系统配置获取响应结构体
type AuthConfigGetResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// AuthConfigData 系统配置数据结构体
type AuthConfigData struct {
	SMTPHost      string `json:"smtp_host"`
	SMTPPort      int    `json:"smtp_port"`
	SMTPUser      string `json:"smtp_user"`
	SMTPPassword  string `json:"smtp_password"`
	SMTPSSL       bool   `json:"smtp_ssl"`
	RedisHost     string `json:"redis_host"`
	RedisPort     int    `json:"redis_port"`
	RedisDB       int    `json:"redis_db"`
	RedisPassword string `json:"redis_password"`
	MySQLHost     string `json:"mysql_host"`
	MySQLPort     int    `json:"mysql_port"`
	MySQLUser     string `json:"mysql_user"`
	MySQLPassword string `json:"mysql_password"`
	MySQLDatabase string `json:"mysql_database"`
}

// SystemConfig 系统配置模型
type SystemConfig struct {
	ID          int       `json:"id" db:"id"`
	ConfigKey   string    `json:"config_key" db:"config_key"`
	ConfigValue string    `json:"config_value" db:"config_value"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// Blacklist IP黑名单结构体
type Blacklist struct {
	ID          int       `json:"id" db:"id"`
	IPAddress   string    `json:"ip_address" db:"ip_address"`
	BanDuration int       `json:"ban_duration" db:"ban_duration"` // 封禁时长(分钟)
	EventType   string    `json:"event_type" db:"event_type"`     // 事件类型
	CreatedAt   time.Time `json:"created_at" db:"created_at"`     // 添加时间
	ExpiresAt   time.Time `json:"expires_at" db:"expires_at"`     // 到期时间
	Status      string    `json:"status" db:"status"`             // 状态：active/expired
}

// BlacklistResponse 黑名单响应结构体
type BlacklistResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    []Blacklist `json:"data,omitempty"`
}

// BlacklistPaginationResponse 分页黑名单响应结构体
type BlacklistPaginationResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    []Blacklist `json:"data,omitempty"`
	Total   int         `json:"total"`
	Page    int         `json:"page"`
	Limit   int         `json:"limit"`
	Pages   int         `json:"pages"`
}

// AddBlacklistRequest 添加黑名单请求结构体
type AddBlacklistRequest struct {
	IPAddress   string `json:"ip_address" binding:"required" validate:"ip"`
	BanDuration int    `json:"ban_duration" binding:"required,min=60,max=86400"`
	EventType   string `json:"event_type" binding:"required,max=100"`
}

// BlacklistAddRequest 添加黑名单请求结构体（兼容性）
type BlacklistAddRequest struct {
	IPAddress   string `json:"ip_address" binding:"required"`
	BanDuration int    `json:"ban_duration" binding:"required"`
	EventType   string `json:"event_type" binding:"required"`
}

// BlacklistAddResponse 添加黑名单响应结构体
type BlacklistAddResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// DeleteBlacklistRequest 删除黑名单请求结构体
type DeleteBlacklistRequest struct {
	IPAddress string `json:"ip_address" binding:"required"`
}

// BlacklistDeleteResponse 删除黑名单响应结构体
type BlacklistDeleteResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}
