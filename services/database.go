package services

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"MailGateway/config"
	"MailGateway/models"

	_ "github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// DatabaseService 数据库服务
type DatabaseService struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewDatabaseService 创建数据库服务实例
func NewDatabaseService(cfg *config.Config, logger *zap.Logger) (*DatabaseService, error) {
	// 首先连接到MySQL服务器（不指定数据库）
	rootDSN := fmt.Sprintf("%s:%s@tcp(%s:%s)/", cfg.Database.User, cfg.Database.Password, cfg.Database.Host, cfg.Database.Port)
	rootDB, err := sql.Open("mysql", rootDSN)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MySQL server: %w", err)
	}
	defer rootDB.Close()

	// 检查并创建数据库
	if err := createDatabaseIfNotExists(rootDB, cfg.Database.Database, logger); err != nil {
		return nil, fmt.Errorf("failed to create database: %w", err)
	}

	// 连接到指定数据库
	db, err := sql.Open("mysql", cfg.GetDSN())
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 设置连接池参数
	db.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	db.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.Database.ConnMaxLifetime)

	// 测试连接
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// 创建表
	if err := createTablesIfNotExists(db, logger); err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	// 创建数据库服务实例
	dbService := &DatabaseService{
		db:     db,
		logger: logger,
	}

	// 初始化默认管理员账户
	if err := dbService.initializeDefaultAdmin(cfg); err != nil {
		return nil, fmt.Errorf("failed to initialize default admin: %w", err)
	}

	logger.Info("数据库连接和初始化成功")

	return dbService, nil
}

// Close 关闭数据库连接
func (s *DatabaseService) Close() error {
	return s.db.Close()
}

// createDatabaseIfNotExists 检查并创建数据库
func createDatabaseIfNotExists(db *sql.DB, dbName string, logger *zap.Logger) error {
	// 检查数据库是否存在
	var schemaName string
	query := "SELECT SCHEMA_NAME FROM INFORMATION_SCHEMA.SCHEMATA WHERE SCHEMA_NAME = ?"
	err := db.QueryRow(query, dbName).Scan(&schemaName)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check database existence: %w", err)
	}

	if err == sql.ErrNoRows {
		// 数据库不存在，创建它
		createQuery := fmt.Sprintf("CREATE DATABASE %s CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", dbName)
		_, err := db.Exec(createQuery)
		if err != nil {
			return fmt.Errorf("failed to create database: %w", err)
		}
		logger.Info("数据库创建成功", zap.String("database", dbName))
	} else {
		logger.Info("数据库已存在", zap.String("database", dbName))
	}

	return nil
}

// SaveSystemConfig 保存系统配置
func (s *DatabaseService) SaveSystemConfig(configKey, configValue string) error {
	query := `
		INSERT INTO system_config (config_key, config_value) 
		VALUES (?, ?) 
		ON DUPLICATE KEY UPDATE 
		config_value = VALUES(config_value), 
		updated_at = CURRENT_TIMESTAMP
	`
	_, err := s.db.Exec(query, configKey, configValue)
	if err != nil {
		return fmt.Errorf("failed to save system config: %w", err)
	}
	return nil
}

// GetSystemConfig 获取系统配置
func (s *DatabaseService) GetSystemConfig(configKey string) (string, error) {
	var configValue string
	query := "SELECT config_value FROM system_config WHERE config_key = ?"
	err := s.db.QueryRow(query, configKey).Scan(&configValue)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("config key %s not found", configKey)
		}
		return "", fmt.Errorf("failed to get system config: %w", err)
	}
	return configValue, nil
}

// GetAllSystemConfigs 获取所有系统配置
func (s *DatabaseService) GetAllSystemConfigs() ([]models.SystemConfig, error) {
	query := "SELECT id, config_key, config_value, created_at, updated_at FROM system_config ORDER BY config_key"
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query system configs: %w", err)
	}
	defer rows.Close()

	var configs []models.SystemConfig
	for rows.Next() {
		var config models.SystemConfig
		err := rows.Scan(&config.ID, &config.ConfigKey, &config.ConfigValue, &config.CreatedAt, &config.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan system config: %w", err)
		}
		configs = append(configs, config)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate system configs: %w", err)
	}

	return configs, nil
}

// DeleteSystemConfig 删除系统配置
func (s *DatabaseService) DeleteSystemConfig(configKey string) error {
	query := "DELETE FROM system_config WHERE config_key = ?"
	_, err := s.db.Exec(query, configKey)
	if err != nil {
		return fmt.Errorf("failed to delete system config: %w", err)
	}
	return nil
}

// createTablesIfNotExists 检查并创建表
func createTablesIfNotExists(db *sql.DB, logger *zap.Logger) error {
	// 创建users表
	if err := createUsersTable(db, logger); err != nil {
		return err
	}

	// 创建admin表
	if err := createAdminTable(db, logger); err != nil {
		return err
	}

	// 创建系统配置表
	if err := createSystemConfigTable(db, logger); err != nil {
		return err
	}

	// 创建黑名单表
	if err := createBlacklistTable(db, logger); err != nil {
		return err
	}

	// 创建操作日志表
	if err := createOperationLogTable(db, logger); err != nil {
		return err
	}

	// 创建认证日志表
	if err := createAuthLogTable(db, logger); err != nil {
		return err
	}

	return nil
}

// createSystemConfigTable 创建系统配置表
func createSystemConfigTable(db *sql.DB, logger *zap.Logger) error {
	var tableName string
	query := "SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'system_config'"
	err := db.QueryRow(query).Scan(&tableName)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check system_config table existence: %w", err)
	}

	if err == sql.ErrNoRows {
		createTableQuery := `
			CREATE TABLE system_config (
				id INT AUTO_INCREMENT PRIMARY KEY,
				config_key VARCHAR(255) NOT NULL UNIQUE,
				config_value TEXT NOT NULL,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				UNIQUE INDEX idx_config_key (config_key)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
		`
		_, err := db.Exec(createTableQuery)
		if err != nil {
			return fmt.Errorf("failed to create system_config table: %w", err)
		}
		logger.Info("系统配置表创建成功")
	} else {
		logger.Info("系统配置表已存在")
	}

	return nil
}

// createUsersTable 创建用户表
func createUsersTable(db *sql.DB, logger *zap.Logger) error {
	var tableName string
	query := "SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users'"
	err := db.QueryRow(query).Scan(&tableName)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check users table existence: %w", err)
	}

	if err == sql.ErrNoRows {
		createTableQuery := `
			CREATE TABLE users (
				id INT PRIMARY KEY,
				username VARCHAR(255) NOT NULL UNIQUE,
				password VARCHAR(255) NOT NULL,
				owner VARCHAR(255) NOT NULL,
				totp_secret VARCHAR(255) DEFAULT NULL,
				totp_enabled BOOLEAN DEFAULT FALSE,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				INDEX idx_username (username)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
		`
		_, err := db.Exec(createTableQuery)
		if err != nil {
			return fmt.Errorf("failed to create users table: %w", err)
		}
		logger.Info("用户表创建成功")
	} else {
		logger.Info("用户表已存在")
	}

	return nil
}

// createBlacklistTable 创建IP黑名单表
func createBlacklistTable(db *sql.DB, logger *zap.Logger) error {
	var tableName string
	query := "SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'blacklist'"
	err := db.QueryRow(query).Scan(&tableName)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check blacklist table existence: %w", err)
	}

	if err == sql.ErrNoRows {
		createTableQuery := `
			CREATE TABLE blacklist (
				id INT AUTO_INCREMENT PRIMARY KEY,
				ip_address VARCHAR(45) NOT NULL,
				ban_duration INT NOT NULL COMMENT '封禁时长(分钟)',
				event_type VARCHAR(100) NOT NULL COMMENT '事件类型：爆破攻击、SQL注入等',
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '添加时间',
				expires_at TIMESTAMP NOT NULL COMMENT '到期时间',
				status ENUM('active', 'expired') DEFAULT 'active' COMMENT '状态',
				INDEX idx_ip_address (ip_address),
				INDEX idx_expires_at (expires_at),
				INDEX idx_status (status)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
		`
		_, err := db.Exec(createTableQuery)
		if err != nil {
			return fmt.Errorf("failed to create blacklist table: %w", err)
		}
		logger.Info("IP黑名单表创建成功")
	} else {
		logger.Info("IP黑名单表已存在")
	}

	return nil
}

// createAdminTable 创建管理员表
func createAdminTable(db *sql.DB, logger *zap.Logger) error {
	var tableName string
	query := "SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'admin'"
	err := db.QueryRow(query).Scan(&tableName)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check admin table existence: %w", err)
	}

	if err == sql.ErrNoRows {
		createTableQuery := `
			CREATE TABLE admin (
				id INT AUTO_INCREMENT PRIMARY KEY,
				username VARCHAR(255) NOT NULL UNIQUE,
				password VARCHAR(255) NOT NULL,
				enabled BOOLEAN DEFAULT TRUE,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				INDEX idx_username (username)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
		`
		_, err := db.Exec(createTableQuery)
		if err != nil {
			return fmt.Errorf("failed to create admin table: %w", err)
		}
		logger.Info("管理员表创建成功")
	} else {
		logger.Info("管理员表已存在")
	}

	return nil
}

// GetUser 根据用户名获取用户信息
func (s *DatabaseService) GetUser(username string) (*models.User, error) {
	query := `SELECT id, username, password, owner, totp_secret, totp_enabled, created_at, updated_at FROM users WHERE username = ?`

	var user models.User
	var totpSecret sql.NullString
	var totpEnabled sql.NullBool
	err := s.db.QueryRow(query, username).Scan(
		&user.ID,
		&user.Username,
		&user.Password,
		&user.Owner,
		&totpSecret,
		&totpEnabled,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // 用户不存在
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// 处理NULL值
	if totpSecret.Valid {
		user.TOTPSecret = totpSecret.String
	} else {
		user.TOTPSecret = ""
	}

	if totpEnabled.Valid {
		user.TOTPEnabled = totpEnabled.Bool
	} else {
		user.TOTPEnabled = false
	}

	return &user, nil
}

// ValidatePassword 验证用户密码
func (s *DatabaseService) ValidatePassword(username, password string) (bool, error) {
	user, err := s.GetUser(username)
	if err != nil {
		return false, err
	}
	if user == nil {
		return false, nil // 用户不存在
	}

	// 这里应该使用哈希密码比较，但为了兼容现有系统，暂时使用明文比较
	return user.Password == password, nil
}

// UpdateTOTPSecret 更新用户的TOTP密钥并启用TOTP
func (s *DatabaseService) UpdateTOTPSecret(username, secret string) error {
	// 首先检查用户是否存在
	user, err := s.GetUser(username)
	if err != nil {
		return fmt.Errorf("failed to check user existence: %w", err)
	}
	if user == nil {
		return fmt.Errorf("用户不存在")
	}

	query := `UPDATE users SET totp_secret = ?, totp_enabled = 1 WHERE username = ?`
	result, err := s.db.Exec(query, secret, username)
	if err != nil {
		return fmt.Errorf("failed to update TOTP secret: %w", err)
	}

	// 检查是否有行被更新
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("用户不存在")
	}

	return nil
}

// UpdateTOTPSecretOnly 只更新用户的TOTP密钥，不改变启用状态
func (s *DatabaseService) UpdateTOTPSecretOnly(username, secret string) error {
	// 首先检查用户是否存在
	user, err := s.GetUser(username)
	if err != nil {
		return fmt.Errorf("failed to check user existence: %w", err)
	}
	if user == nil {
		return fmt.Errorf("用户不存在")
	}

	query := `UPDATE users SET totp_secret = ? WHERE username = ?`
	result, err := s.db.Exec(query, secret, username)
	if err != nil {
		return fmt.Errorf("failed to update TOTP secret: %w", err)
	}

	// 检查是否有行被更新
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("用户不存在")
	}

	return nil
}

// DeleteTOTPSecret 删除用户的TOTP密钥
func (s *DatabaseService) DeleteTOTPSecret(username string) error {
	// 先检查用户是否存在
	user, err := s.GetUser(username)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("用户不存在")
	}

	query := `UPDATE users SET totp_secret = NULL, totp_enabled = NULL WHERE username = ?`
	result, err := s.db.Exec(query, username)
	if err != nil {
		return fmt.Errorf("failed to delete TOTP secret: %w", err)
	}

	// 检查是否有行被更新
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("用户不存在")
	}

	return nil
}

// GetAllUsers 获取所有用户（用于TOTP初始化）
func (s *DatabaseService) GetAllUsers() ([]models.User, error) {
	query := `SELECT id, username, password, owner, totp_secret, totp_enabled, created_at, updated_at FROM users ORDER BY created_at DESC`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query users: %w", err)
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var user models.User
		var totpSecret sql.NullString
		var totpEnabled sql.NullBool
		err := rows.Scan(
			&user.ID,
			&user.Username,
			&user.Password,
			&user.Owner,
			&totpSecret,
			&totpEnabled,
			&user.CreatedAt,
			&user.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}

		// 处理NULL值
		if totpSecret.Valid {
			user.TOTPSecret = totpSecret.String
		} else {
			user.TOTPSecret = ""
		}

		if totpEnabled.Valid {
			user.TOTPEnabled = totpEnabled.Bool
		} else {
			user.TOTPEnabled = false
		}

		users = append(users, user)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return users, nil
}

// GetUsersWithPagination 获取用户列表（支持分页和搜索）
func (s *DatabaseService) GetUsersWithPagination(page, limit int, search string) ([]models.User, int, error) {
	// 构建基础查询
	baseQuery := `SELECT id, username, password, owner, totp_secret, totp_enabled, created_at, updated_at FROM users`
	countQuery := `SELECT COUNT(*) FROM users`

	var args []interface{}
	var whereClause string

	// 添加搜索条件
	if search != "" {
		whereClause = ` WHERE username LIKE ? OR owner LIKE ?`
		searchPattern := "%" + search + "%"
		args = append(args, searchPattern, searchPattern)
	}

	// 获取总数
	var total int
	err := s.db.QueryRow(countQuery+whereClause, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count users: %w", err)
	}

	// 添加排序和分页
	orderClause := ` ORDER BY id ASC`
	limitClause := ` LIMIT ? OFFSET ?`
	offset := (page - 1) * limit
	args = append(args, limit, offset)

	finalQuery := baseQuery + whereClause + orderClause + limitClause
	rows, err := s.db.Query(finalQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query users: %w", err)
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var user models.User
		var totpSecret sql.NullString
		var totpEnabled sql.NullBool
		err := rows.Scan(
			&user.ID,
			&user.Username,
			&user.Password,
			&user.Owner,
			&totpSecret,
			&totpEnabled,
			&user.CreatedAt,
			&user.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan user: %w", err)
		}

		// 处理NULL值
		if totpSecret.Valid {
			user.TOTPSecret = totpSecret.String
		} else {
			user.TOTPSecret = ""
		}

		if totpEnabled.Valid {
			user.TOTPEnabled = totpEnabled.Bool
		} else {
			user.TOTPEnabled = false
		}

		users = append(users, user)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows iteration error: %w", err)
	}

	return users, total, nil
}

// getNextAvailableUserID 获取下一个可用的用户ID
func (s *DatabaseService) getNextAvailableUserID() (int, error) {
	// 查找最小可用ID
	query := `
		SELECT COALESCE(MIN(t1.id + 1), 1) AS next_id 
		FROM (
			SELECT 0 AS id
			UNION ALL
			SELECT id FROM users
		) t1 
		LEFT JOIN users t2 ON t1.id + 1 = t2.id 
		WHERE t2.id IS NULL
		ORDER BY next_id
		LIMIT 1
	`

	var nextID int
	err := s.db.QueryRow(query).Scan(&nextID)
	if err != nil {
		return 0, fmt.Errorf("failed to get next available user ID: %w", err)
	}

	return nextID, nil
}

// CreateUser 创建新用户
func (s *DatabaseService) CreateUser(req *models.UserCreateRequest) error {
	// 获取下一个可用的ID
	nextID, err := s.getNextAvailableUserID()
	if err != nil {
		return fmt.Errorf("failed to get next available ID: %w", err)
	}

	// 将字符串类型的totp_enabled转换为布尔值
	totpEnabled := false
	if strings.ToLower(req.TOTPEnabled) == "true" || req.TOTPEnabled == "1" {
		totpEnabled = true
	}

	query := `INSERT INTO users (id, username, password, owner, totp_secret, totp_enabled) VALUES (?, ?, ?, ?, NULL, ?)`
	_, err = s.db.Exec(query, nextID, req.Username, req.Password, req.Owner, totpEnabled)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}
	return nil
}

// UpdateUser 更新用户信息
func (s *DatabaseService) UpdateUser(username string, req *models.UserUpdateRequest) error {
	// 首先检查用户是否存在
	user, err := s.GetUser(username)
	if err != nil {
		return fmt.Errorf("failed to check user existence: %w", err)
	}
	if user == nil {
		return fmt.Errorf("用户不存在")
	}

	// 检查是否有字段需要更新
	if req.Password == "" && req.TOTPEnabled == nil {
		return fmt.Errorf("no fields to update")
	}

	// 构建动态更新查询
	var setParts []string
	var args []interface{}

	if req.Password != "" {
		setParts = append(setParts, "password = ?")
		args = append(args, req.Password)
	}

	if req.TOTPEnabled != nil {
		setParts = append(setParts, "totp_enabled = ?")
		args = append(args, *req.TOTPEnabled)
	}

	query := fmt.Sprintf("UPDATE users SET %s WHERE username = ?", strings.Join(setParts, ", "))
	args = append(args, username)

	result, err := s.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	// 检查是否有行被更新
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("用户不存在")
	}

	return nil
}

// DeleteUser 删除用户
func (s *DatabaseService) DeleteUser(username string) error {
	query := "DELETE FROM users WHERE username = ?"
	_, err := s.db.Exec(query, username)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	return nil
}

// RecordLoginAttempt 记录登录尝试

// GetAdmin 根据用户名获取管理员信息
func (s *DatabaseService) GetAdmin(username string) (*models.Admin, error) {
	query := "SELECT id, username, password, enabled, created_at, updated_at FROM admin WHERE username = ?"
	row := s.db.QueryRow(query, username)

	var admin models.Admin
	err := row.Scan(&admin.ID, &admin.Username, &admin.Password, &admin.Enabled, &admin.CreatedAt, &admin.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get admin: %w", err)
	}

	return &admin, nil
}

// ValidateAdminPassword 验证管理员密码
func (s *DatabaseService) ValidateAdminPassword(username, password string) (bool, error) {
	admin, err := s.GetAdmin(username)
	if err != nil {
		return false, err
	}
	if admin == nil || !admin.Enabled {
		return false, nil
	}

	// 使用bcrypt验证密码
	err = bcrypt.CompareHashAndPassword([]byte(admin.Password), []byte(password))
	if err != nil {
		if err == bcrypt.ErrMismatchedHashAndPassword {
			return false, nil // 密码不匹配
		}
		return false, err // 其他错误
	}
	return true, nil
}

// CreateAdmin 创建管理员
func (s *DatabaseService) CreateAdmin(req *models.AdminCreateRequest) error {
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	// 对密码进行bcrypt加密
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	query := "INSERT INTO admin (username, password, enabled) VALUES (?, ?, ?)"
	_, err = s.db.Exec(query, req.Username, string(hashedPassword), enabled)
	if err != nil {
		return fmt.Errorf("failed to create admin: %w", err)
	}
	return nil
}

// CreateAdminWithTOTP 创建带TOTP密钥的管理员
func (s *DatabaseService) CreateAdminWithTOTP(username, password, totpSecret string) error {
	// 对密码进行bcrypt加密
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	query := "INSERT INTO admin (username, password, enabled, totp_secret) VALUES (?, ?, ?, ?)"
	_, err = s.db.Exec(query, username, string(hashedPassword), true, totpSecret)
	if err != nil {
		return fmt.Errorf("failed to create admin with TOTP: %w", err)
	}
	return nil
}

// UpdateAdmin 更新管理员信息
func (s *DatabaseService) UpdateAdmin(username string, req *models.AdminUpdateRequest) error {
	var setParts []string
	var args []interface{}

	if req.Password != nil {
		setParts = append(setParts, "password = ?")
		args = append(args, *req.Password)
	}

	if req.Enabled != nil {
		setParts = append(setParts, "enabled = ?")
		args = append(args, *req.Enabled)
	}

	if len(setParts) == 0 {
		return fmt.Errorf("no fields to update")
	}

	setParts = append(setParts, "updated_at = CURRENT_TIMESTAMP")
	args = append(args, username)

	query := fmt.Sprintf("UPDATE admin SET %s WHERE username = ?", strings.Join(setParts, ", "))
	_, err := s.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("failed to update admin: %w", err)
	}
	return nil
}

// DeleteAdmin 删除管理员
func (s *DatabaseService) DeleteAdmin(username string) error {
	query := "DELETE FROM admin WHERE username = ?"
	_, err := s.db.Exec(query, username)
	if err != nil {
		return fmt.Errorf("failed to delete admin: %w", err)
	}
	return nil
}

// GetAllAdmins 获取所有管理员
func (s *DatabaseService) GetAllAdmins() ([]models.Admin, error) {
	query := "SELECT id, username, password, enabled, created_at FROM admin ORDER BY id ASC"
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query admins: %w", err)
	}
	defer rows.Close()

	var admins []models.Admin
	for rows.Next() {
		var admin models.Admin
		err := rows.Scan(&admin.ID, &admin.Username, &admin.Password, &admin.Enabled, &admin.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan admin: %w", err)
		}
		admins = append(admins, admin)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate admins: %w", err)
	}

	return admins, nil
}

// GetAdminByUsername 根据用户名获取管理员
func (s *DatabaseService) GetAdminByUsername(username string) (*models.Admin, error) {
	return s.GetAdmin(username)
}

// CheckAdminPassword 检查管理员密码
func (s *DatabaseService) CheckAdminPassword(admin *models.Admin, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(admin.Password), []byte(password))
	return err == nil
}

// UpdateAdminPassword 更新管理员密码
func (s *DatabaseService) UpdateAdminPassword(username, newPassword string) error {
	// 加密新密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	query := "UPDATE admin SET password = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?"
	_, err = s.db.Exec(query, string(hashedPassword), username)
	if err != nil {
		return fmt.Errorf("failed to update admin password: %w", err)
	}
	return nil
}

// GetAllBlacklist 获取所有黑名单记录
func (s *DatabaseService) GetAllBlacklist() ([]models.Blacklist, error) {
	query := "SELECT id, ip_address, ban_duration, event_type, created_at, expires_at, status FROM blacklist ORDER BY created_at DESC"
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query blacklist: %w", err)
	}
	defer rows.Close()

	var blacklists []models.Blacklist
	for rows.Next() {
		var blacklist models.Blacklist
		err := rows.Scan(&blacklist.ID, &blacklist.IPAddress, &blacklist.BanDuration, &blacklist.EventType, &blacklist.CreatedAt, &blacklist.ExpiresAt, &blacklist.Status)
		if err != nil {
			return nil, fmt.Errorf("failed to scan blacklist row: %w", err)
		}
		blacklists = append(blacklists, blacklist)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating blacklist rows: %w", err)
	}

	return blacklists, nil
}

// GetBlacklistWithPagination 获取黑名单列表（支持分页和搜索）
func (s *DatabaseService) GetBlacklistWithPagination(page, limit int, search string) ([]models.Blacklist, int, error) {
	// 构建基础查询
	baseQuery := `SELECT id, ip_address, ban_duration, event_type, created_at, expires_at, status FROM blacklist`
	countQuery := `SELECT COUNT(*) FROM blacklist`

	var args []interface{}
	var whereClause string

	// 添加搜索条件
	if search != "" {
		whereClause = ` WHERE ip_address LIKE ? OR event_type LIKE ?`
		searchPattern := "%" + search + "%"
		args = append(args, searchPattern, searchPattern)
	}

	// 获取总数
	var total int
	err := s.db.QueryRow(countQuery+whereClause, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count blacklist: %w", err)
	}

	// 添加排序和分页
	orderClause := ` ORDER BY created_at DESC`
	limitClause := ` LIMIT ? OFFSET ?`
	offset := (page - 1) * limit
	args = append(args, limit, offset)

	finalQuery := baseQuery + whereClause + orderClause + limitClause
	rows, err := s.db.Query(finalQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query blacklist: %w", err)
	}
	defer rows.Close()

	var blacklists []models.Blacklist
	for rows.Next() {
		var blacklist models.Blacklist
		err := rows.Scan(
			&blacklist.ID,
			&blacklist.IPAddress,
			&blacklist.BanDuration,
			&blacklist.EventType,
			&blacklist.CreatedAt,
			&blacklist.ExpiresAt,
			&blacklist.Status,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan blacklist: %w", err)
		}

		blacklists = append(blacklists, blacklist)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows iteration error: %w", err)
	}

	return blacklists, total, nil
}

// AddBlacklist 添加IP黑名单记录
func (s *DatabaseService) AddBlacklist(ipAddress string, banDuration int, eventType string) error {
	// 检查IP是否已存在且仍在有效期内
	var existingID int
	checkQuery := "SELECT id FROM blacklist WHERE ip_address = ? AND status = 'active' AND expires_at > NOW()"
	err := s.db.QueryRow(checkQuery, ipAddress).Scan(&existingID)
	if err == nil {
		// IP已存在且仍在有效期内，更新记录
		updateQuery := `
			UPDATE blacklist 
			SET ban_duration = ?, event_type = ?, expires_at = DATE_ADD(NOW(), INTERVAL ? SECOND), created_at = NOW() 
			WHERE id = ?
		`
		_, err = s.db.Exec(updateQuery, banDuration, eventType, banDuration, existingID)
		if err != nil {
			return fmt.Errorf("failed to update blacklist: %w", err)
		}
		return nil
	} else if err != sql.ErrNoRows {
		return fmt.Errorf("failed to check existing blacklist: %w", err)
	}

	// 创建新的黑名单记录
	insertQuery := `
		INSERT INTO blacklist (ip_address, ban_duration, event_type, created_at, expires_at, status) 
		VALUES (?, ?, ?, NOW(), DATE_ADD(NOW(), INTERVAL ? SECOND), 'active')
	`
	_, err = s.db.Exec(insertQuery, ipAddress, banDuration, eventType, banDuration)
	if err != nil {
		return fmt.Errorf("failed to create blacklist: %w", err)
	}
	return nil
}

// CheckBlacklistExists 检查IP是否已在黑名单中
func (s *DatabaseService) CheckBlacklistExists(ipAddress string) (bool, error) {
	var count int
	query := "SELECT COUNT(*) FROM blacklist WHERE ip_address = ? AND status = 'active' AND expires_at > NOW()"
	err := s.db.QueryRow(query, ipAddress).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check blacklist existence: %w", err)
	}
	return count > 0, nil
}

// DeleteBlacklist 删除IP黑名单记录
func (s *DatabaseService) DeleteBlacklist(ipAddress string) error {
	// 检查IP是否存在于黑名单中
	var count int
	checkQuery := "SELECT COUNT(*) FROM blacklist WHERE ip_address = ?"
	err := s.db.QueryRow(checkQuery, ipAddress).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check blacklist existence: %w", err)
	}

	if count == 0 {
		return fmt.Errorf("IP地址不在黑名单中")
	}

	// 删除黑名单记录
	deleteQuery := "DELETE FROM blacklist WHERE ip_address = ?"
	result, err := s.db.Exec(deleteQuery, ipAddress)
	if err != nil {
		return fmt.Errorf("failed to delete blacklist: %w", err)
	}

	// 检查是否有记录被删除
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("没有记录被删除")
	}

	return nil
}

// initializeDefaultAdmin 初始化默认管理员账户
// createOperationLogTable 创建操作日志表
func createOperationLogTable(db *sql.DB, logger *zap.Logger) error {
	var tableName string
	query := "SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'operation_log'"
	err := db.QueryRow(query).Scan(&tableName)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check operation_log table existence: %w", err)
	}

	if err == sql.ErrNoRows {
		createTableQuery := `
			CREATE TABLE operation_log (
				id INT AUTO_INCREMENT PRIMARY KEY,
				username VARCHAR(255) NOT NULL COMMENT '操作用户名',
				user_type ENUM('admin', 'user') NOT NULL COMMENT '用户类型',
				operation VARCHAR(100) NOT NULL COMMENT '操作类型',
				resource VARCHAR(100) NOT NULL COMMENT '操作资源',
				details TEXT COMMENT '操作详情',
				ip_address VARCHAR(45) NOT NULL COMMENT '客户端IP',
				user_agent TEXT COMMENT '用户代理',
				status ENUM('success', 'failed') NOT NULL COMMENT '操作状态',
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '操作时间',
				INDEX idx_username (username),
				INDEX idx_user_type (user_type),
				INDEX idx_operation (operation),
				INDEX idx_created_at (created_at),
				INDEX idx_status (status)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
		`
		_, err := db.Exec(createTableQuery)
		if err != nil {
			return fmt.Errorf("failed to create operation_log table: %w", err)
		}
		logger.Info("操作日志表创建成功")
	} else {
		logger.Info("操作日志表已存在")
	}

	return nil
}

// GetUserTypeByUsername 根据用户名动态判断用户类型
func (s *DatabaseService) GetUserTypeByUsername(username string) string {
	// 首先检查是否为管理员
	admin, err := s.GetAdmin(username)
	if err == nil && admin != nil {
		return "admin"
	}

	// 然后检查是否为普通用户
	user, err := s.GetUser(username)
	if err == nil && user != nil {
		return "user"
	}

	// 如果都不是，返回unknown
	return "unknown"
}

// AddOperationLog 添加操作日志
func (s *DatabaseService) AddOperationLog(username, userType, operation, resource, details, ipAddress, userAgent, status string) error {
	query := `
		INSERT INTO operation_log (username, user_type, operation, resource, details, ip_address, user_agent, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query, username, userType, operation, resource, details, ipAddress, userAgent, status)
	if err != nil {
		return fmt.Errorf("failed to add operation log: %w", err)
	}
	return nil
}

// GetOperationLogsWithPagination 分页获取操作日志
func (s *DatabaseService) GetOperationLogsWithPagination(page, limit int, username, userType, operation, startTime, endTime string) ([]models.OperationLog, int, error) {
	// 构建查询条件
	where := "WHERE 1=1"
	args := []interface{}{}

	if username != "" {
		where += " AND username LIKE ?"
		args = append(args, "%"+username+"%")
	}
	if userType != "" {
		where += " AND user_type = ?"
		args = append(args, userType)
	}
	if operation != "" {
		where += " AND operation LIKE ?"
		args = append(args, "%"+operation+"%")
	}
	if startTime != "" {
		where += " AND created_at >= ?"
		args = append(args, startTime)
	}
	if endTime != "" {
		where += " AND created_at <= ?"
		args = append(args, endTime)
	}

	// 获取总数
	countQuery := "SELECT COUNT(*) FROM operation_log " + where
	var total int
	err := s.db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count operation logs: %w", err)
	}

	// 获取分页数据
	offset := (page - 1) * limit
	query := `
		SELECT id, username, user_type, operation, resource, details, ip_address, user_agent, status, created_at
		FROM operation_log ` + where + `
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query operation logs: %w", err)
	}
	defer rows.Close()

	var logs []models.OperationLog
	for rows.Next() {
		var log models.OperationLog
		err := rows.Scan(
			&log.ID,
			&log.Username,
			&log.UserType,
			&log.Operation,
			&log.Resource,
			&log.Details,
			&log.IPAddress,
			&log.UserAgent,
			&log.Status,
			&log.CreatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan operation log: %w", err)
		}
		logs = append(logs, log)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate operation logs: %w", err)
	}

	return logs, total, nil
}

func (s *DatabaseService) initializeDefaultAdmin(cfg *config.Config) error {
	// 检查是否已存在管理员账户
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM admin").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check admin count: %w", err)
	}

	// 如果已存在管理员账户，则不需要初始化
	if count > 0 {
		s.logger.Info("管理员账户已存在，跳过初始化")
		return nil
	}

	// 加密默认密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(cfg.Admin.DefaultPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash default password: %w", err)
	}

	// 插入默认管理员账户
	query := `INSERT INTO admin (username, password, enabled, created_at, updated_at) 
			  VALUES (?, ?, 1, NOW(), NOW())`

	_, err = s.db.Exec(query, cfg.Admin.DefaultUsername, string(hashedPassword))
	if err != nil {
		return fmt.Errorf("failed to create default admin: %w", err)
	}

	s.logger.Info("默认管理员账户创建成功",
		zap.String("username", cfg.Admin.DefaultUsername))

	return nil
}

// createAuthLogTable 创建认证日志表
func createAuthLogTable(db *sql.DB, logger *zap.Logger) error {
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS auth_log (
		id INT AUTO_INCREMENT PRIMARY KEY,
		username VARCHAR(255) NOT NULL COMMENT '认证用户名',
		ip_address VARCHAR(45) NOT NULL COMMENT 'IP地址',
		protocol VARCHAR(10) NOT NULL COMMENT '认证协议',
		result VARCHAR(50) NOT NULL COMMENT '认证结果',
		result_detail TEXT COMMENT '认证结果详情',
		client_ip VARCHAR(45) COMMENT '原始客户端IP',
		user_agent TEXT COMMENT '用户代理',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '认证时间',
		INDEX idx_username (username),
		INDEX idx_ip_address (ip_address),
		INDEX idx_protocol (protocol),
		INDEX idx_result (result),
		INDEX idx_created_at (created_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='认证日志表';
	`

	_, err := db.Exec(createTableSQL)
	if err != nil {
		return fmt.Errorf("failed to create auth_log table: %w", err)
	}

	logger.Info("认证日志表创建成功")
	return nil
}

// AddAuthLog 添加认证日志
func (s *DatabaseService) AddAuthLog(username, ipAddress, protocol, result, resultDetail, clientIP, userAgent string) error {
	query := `
		INSERT INTO auth_log (username, ip_address, protocol, result, result_detail, client_ip, user_agent) 
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query, username, ipAddress, protocol, result, resultDetail, clientIP, userAgent)
	if err != nil {
		return fmt.Errorf("failed to add auth log: %w", err)
	}
	return nil
}

// GetAuthLogsWithPagination 分页获取认证日志
func (s *DatabaseService) GetAuthLogsWithPagination(page, limit int, username, protocol, result, ipAddress, startTime, endTime string) ([]models.AuthLog, int, error) {
	// 构建WHERE条件
	var conditions []string
	var args []interface{}

	if username != "" {
		conditions = append(conditions, "username LIKE ?")
		args = append(args, "%"+username+"%")
	}
	if protocol != "" {
		conditions = append(conditions, "protocol = ?")
		args = append(args, protocol)
	}
	if result != "" {
		conditions = append(conditions, "result = ?")
		args = append(args, result)
	}
	if ipAddress != "" {
		conditions = append(conditions, "ip_address LIKE ?")
		args = append(args, "%"+ipAddress+"%")
	}
	if startTime != "" {
		conditions = append(conditions, "created_at >= ?")
		args = append(args, startTime)
	}
	if endTime != "" {
		conditions = append(conditions, "created_at <= ?")
		args = append(args, endTime)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// 获取总数
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM auth_log %s", whereClause)
	var total int
	err := s.db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count auth logs: %w", err)
	}

	// 获取分页数据
	offset := (page - 1) * limit
	query := fmt.Sprintf(`
		SELECT id, username, ip_address, protocol, result, result_detail, client_ip, user_agent, created_at 
		FROM auth_log %s 
		ORDER BY created_at DESC 
		LIMIT ? OFFSET ?
	`, whereClause)

	args = append(args, limit, offset)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query auth logs: %w", err)
	}
	defer rows.Close()

	var logs []models.AuthLog
	for rows.Next() {
		var log models.AuthLog
		err := rows.Scan(
			&log.ID,
			&log.Username,
			&log.IPAddress,
			&log.Protocol,
			&log.Result,
			&log.ResultDetail,
			&log.ClientIP,
			&log.UserAgent,
			&log.CreatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan auth log: %w", err)
		}
		logs = append(logs, log)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate auth logs: %w", err)
	}

	return logs, total, nil
}

// GetAuthLogStats 获取认证日志统计信息
func (s *DatabaseService) GetAuthLogStats() (*models.AuthLogStats, error) {
	stats := &models.AuthLogStats{}

	// 总认证次数
	err := s.db.QueryRow("SELECT COUNT(*) FROM auth_log").Scan(&stats.TotalAuth)
	if err != nil {
		return nil, fmt.Errorf("failed to get total auth count: %w", err)
	}

	// 成功认证次数
	err = s.db.QueryRow("SELECT COUNT(*) FROM auth_log WHERE result = 'success'").Scan(&stats.SuccessAuth)
	if err != nil {
		return nil, fmt.Errorf("failed to get success auth count: %w", err)
	}

	// 失败认证次数
	err = s.db.QueryRow("SELECT COUNT(*) FROM auth_log WHERE result != 'success'").Scan(&stats.FailedAuth)
	if err != nil {
		return nil, fmt.Errorf("failed to get failed auth count: %w", err)
	}

	// 今日认证次数
	today := time.Now().Format("2006-01-02")
	err = s.db.QueryRow("SELECT COUNT(*) FROM auth_log WHERE DATE(created_at) = ?", today).Scan(&stats.TodayAuth)
	if err != nil {
		return nil, fmt.Errorf("failed to get today auth count: %w", err)
	}

	// 唯一用户数
	err = s.db.QueryRow("SELECT COUNT(DISTINCT username) FROM auth_log").Scan(&stats.UniqueUsers)
	if err != nil {
		return nil, fmt.Errorf("failed to get unique users count: %w", err)
	}

	// 唯一IP数
	err = s.db.QueryRow("SELECT COUNT(DISTINCT ip_address) FROM auth_log").Scan(&stats.UniqueIPs)
	if err != nil {
		return nil, fmt.Errorf("failed to get unique IPs count: %w", err)
	}

	return stats, nil
}

// GetDashboardStats 获取仪表盘统计信息
func (s *DatabaseService) GetDashboardStats() (*models.DashboardStats, error) {
	stats := &models.DashboardStats{}

	// 获取今日和昨日的日期
	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	// 今日认证总数
	err := s.db.QueryRow("SELECT COUNT(*) FROM auth_log WHERE DATE(created_at) = ?", today).Scan(&stats.TodayAuthCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get today auth count: %w", err)
	}

	// 昨日认证总数
	var yesterdayAuthCount int
	err = s.db.QueryRow("SELECT COUNT(*) FROM auth_log WHERE DATE(created_at) = ?", yesterday).Scan(&yesterdayAuthCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get yesterday auth count: %w", err)
	}
	// 计算认证总数的百分比变化
	if yesterdayAuthCount > 0 {
		stats.TodayAuthChange = float64(stats.TodayAuthCount-yesterdayAuthCount) / float64(yesterdayAuthCount) * 100
	} else {
		stats.TodayAuthChange = 0
	}

	// 今日成功认证数
	err = s.db.QueryRow("SELECT COUNT(*) FROM auth_log WHERE DATE(created_at) = ? AND result = 'success'", today).Scan(&stats.TodaySuccessCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get today success count: %w", err)
	}

	// 昨日成功认证数
	var yesterdaySuccessCount int
	err = s.db.QueryRow("SELECT COUNT(*) FROM auth_log WHERE DATE(created_at) = ? AND result = 'success'", yesterday).Scan(&yesterdaySuccessCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get yesterday success count: %w", err)
	}
	// 计算成功认证数的百分比变化
	if yesterdaySuccessCount > 0 {
		stats.TodaySuccessChange = float64(stats.TodaySuccessCount-yesterdaySuccessCount) / float64(yesterdaySuccessCount) * 100
	} else {
		stats.TodaySuccessChange = 0
	}

	// 今日失败认证数
	err = s.db.QueryRow("SELECT COUNT(*) FROM auth_log WHERE DATE(created_at) = ? AND result != 'success'", today).Scan(&stats.TodayFailedCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get today failed count: %w", err)
	}

	// 昨日失败认证数
	var yesterdayFailedCount int
	err = s.db.QueryRow("SELECT COUNT(*) FROM auth_log WHERE DATE(created_at) = ? AND result != 'success'", yesterday).Scan(&yesterdayFailedCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get yesterday failed count: %w", err)
	}
	// 计算失败认证数的百分比变化
	if yesterdayFailedCount > 0 {
		stats.TodayFailedChange = float64(stats.TodayFailedCount-yesterdayFailedCount) / float64(yesterdayFailedCount) * 100
	} else {
		stats.TodayFailedChange = 0
	}

	return stats, nil
}
