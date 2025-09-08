package models

import "time"

// User 用户模型
type User struct {
	ID          int       `json:"id" db:"id"` // 在JSON中显示ID
	Username    string    `json:"username" db:"username"`
	Password    string    `json:"-" db:"password"` // 不在JSON中显示密码
	Owner       string    `json:"owner" db:"owner"`
	TOTPSecret  string    `json:"-" db:"totp_secret"` // 不在JSON中显示TOTP密钥
	TOTPEnabled bool      `json:"totp_enabled" db:"totp_enabled"`
	TOTPQRCode  string    `json:"totp_qr_code,omitempty" db:"-"` // TOTP二维码图片Base64编码，仅在启用TOTP时返回
	CreatedAt   time.Time `json:"-" db:"created_at"`             // 不在JSON中显示创建时间
	UpdatedAt   time.Time `json:"-" db:"updated_at"`             // 不在JSON中显示更新时间
}

// UserCreateRequest 创建用户请求
type UserCreateRequest struct {
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required"`
	Owner       string `json:"owner" binding:"required"`
	TOTPEnabled string `json:"totp_enabled" binding:"required"`
}

// UserUpdateRequest 更新用户请求
type UserUpdateRequest struct {
	Password    string `json:"password,omitempty"`
	TOTPEnabled *bool  `json:"totp_enabled,omitempty"`
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	TOTPCode string `json:"totp_code,omitempty"` // TOTP验证码（可选）
}

// LoginResponse 登录响应
type LoginResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	User         User   `json:"user"`
	ExpiresAt    int64  `json:"expires_at"`
	RequiresTOTP bool   `json:"requires_totp,omitempty"` // 是否需要TOTP验证
}

// UserListResponse 用户列表响应
type UserListResponse struct {
	Users []User `json:"users"`
	Total int    `json:"total"`
	Page  int    `json:"page"`
	Limit int    `json:"limit"`
}

// UserStats 用户统计
type UserStats struct {
	TotalUsers       int `json:"total_users"`
	TOTPEnabledUsers int `json:"totp_enabled_users"`
	LockedUsers      int `json:"locked_users"`
}
