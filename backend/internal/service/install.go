// Package service 是业务逻辑层：handler 只做参数绑定与响应输出，业务全部在这里。
package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql" // MySQL 驱动（database/sql 直连，安装期使用）
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/config"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/migration"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/model"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/notify"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/app"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/errcode"
)

// 安装参数校验用的正则（同时兜底防 SQL/反引号注入）。
var (
	dbnameRe    = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	adminUserRe = regexp.MustCompile(`^[A-Za-z0-9_]{3,32}$`)
	colorRe     = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

// InstallService 处理安装向导业务。
type InstallService struct {
	app *app.App
}

// NewInstall 构造安装服务。
func NewInstall(a *app.App) *InstallService {
	return &InstallService{app: a}
}

// Status 返回运行时安装状态。
func (s *InstallService) Status() bool {
	return s.app.IsInstalled()
}

// DBTestRequest 是数据库连接测试请求（契约文档 1.2）。
type DBTestRequest struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	DBName   string `json:"dbname"`
}

// DBTestResult 是数据库连接测试成功结果。
type DBTestResult struct {
	Version string `json:"version"`
}

// DBTest 用提交的凭据连接 MySQL 服务器（不连接具体数据库），校验版本 >= 5.7。
// 失败时返回可读错误（底层驱动错误原文透传给管理员）。
func (s *InstallService) DBTest(ctx context.Context, req *DBTestRequest) (*DBTestResult, error) {
	if s.app.IsInstalled() {
		return nil, errcode.AlreadyInstalled()
	}
	if err := validateDBParams(req.Host, req.Port, req.User, req.DBName); err != nil {
		return nil, err
	}
	version, err := probeMySQL(ctx, req.Host, req.Port, req.User, req.Password)
	if err != nil {
		return nil, errcode.New(400, errcode.CodeDBConnectFailed, fmt.Sprintf("数据库连接失败: %v", err))
	}
	if !versionSupported(version) {
		return nil, errcode.DBVersionUnsupported(version)
	}
	return &DBTestResult{Version: version}, nil
}

// InstallRequest 是完成安装请求（契约文档 1.3）。
type InstallRequest struct {
	Database struct {
		Host     string `json:"host"`
		Port     int    `json:"port"`
		User     string `json:"user"`
		Password string `json:"password"`
		DBName   string `json:"dbname"`
	} `json:"database"`
	SiteName     string `json:"site_name"`
	Logo         string `json:"logo"`
	PrimaryColor string `json:"primary_color"`
	Admin        struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
		RealName string `json:"real_name"`
	} `json:"admin"`
}

// InstallResult 是完成安装结果。
type InstallResult struct {
	Installed       bool `json:"installed"`
	RestartRequired bool `json:"restart_required"`
	AdminID         uint `json:"admin_id"`
}

// Complete 一次性完成安装：建库建表 → 创建管理员 → 种子数据 → 回写配置 → 热切换已安装态。
// 全程持安装互斥锁，已安装态直接拒绝（防重装攻击）。
func (s *InstallService) Complete(ctx context.Context, req *InstallRequest) (*InstallResult, error) {
	unlock := s.app.InstallLock()
	defer unlock()

	if s.app.IsInstalled() {
		return nil, errcode.AlreadyInstalled()
	}

	// ---- 1. 参数校验 ----
	if err := validateDBParams(req.Database.Host, req.Database.Port, req.Database.User, req.Database.DBName); err != nil {
		return nil, err
	}
	if req.Database.DBName == "" {
		return nil, errcode.InvalidParam("database.dbname 不能为空")
	}
	if req.SiteName == "" || len([]rune(req.SiteName)) > 64 {
		return nil, errcode.InvalidParam("site_name 必填且不超过 64 字符")
	}
	if !colorRe.MatchString(req.PrimaryColor) {
		return nil, errcode.InvalidParam("primary_color 必须是 #RRGGBB 十六进制颜色，如 #2563eb")
	}
	if !adminUserRe.MatchString(req.Admin.Username) {
		return nil, errcode.InvalidParam("管理员用户名须为 3-32 位字母/数字/下划线")
	}
	if len(req.Admin.Password) < 8 || len(req.Admin.Password) > 72 {
		return nil, errcode.InvalidParam("管理员密码长度须为 8-72 字符")
	}
	if req.Admin.Email != "" {
		if _, err := mail.ParseAddress(req.Admin.Email); err != nil {
			return nil, errcode.InvalidParam("管理员邮箱格式不正确")
		}
	}
	if len([]rune(req.Admin.RealName)) > 64 {
		return nil, errcode.InvalidParam("real_name 不超过 64 字符")
	}

	// ---- 2. 连接服务器并校验版本 ----
	version, err := probeMySQL(ctx, req.Database.Host, req.Database.Port, req.Database.User, req.Database.Password)
	if err != nil {
		return nil, errcode.New(500, errcode.CodeDBConnectFailed, fmt.Sprintf("数据库连接失败: %v", err))
	}
	if !versionSupported(version) {
		return nil, errcode.DBVersionUnsupported(version)
	}

	// ---- 3. 建库（utf8mb4，见 ADR-013） ----
	if err := createDatabase(ctx, req.Database.Host, req.Database.Port, req.Database.User, req.Database.Password, req.Database.DBName); err != nil {
		return nil, err
	}

	// ---- 4. 建表 ----
	dbCfg := config.DatabaseConfig{
		Host:     req.Database.Host,
		Port:     req.Database.Port,
		User:     req.Database.User,
		Password: req.Database.Password,
		DBName:   req.Database.DBName,
		Charset:  "utf8mb4",
	}
	db, err := migration.Open(dbCfg)
	if err != nil {
		return nil, errcode.New(500, errcode.CodeDBConnectFailed, fmt.Sprintf("连接目标数据库失败: %v", err))
	}
	if err := migration.AutoMigrate(db); err != nil {
		closeDB(db)
		slog.Error("安装迁移失败", "err", err)
		return nil, errcode.New(500, errcode.CodeMigrationFailed, "建表失败，详见服务端日志").WithErr(err)
	}

	// ---- 5. 创建管理员（bcrypt，见 ADR-003） ----
	admin, err := s.createAdmin(db, req)
	if err != nil {
		closeDB(db)
		return nil, err
	}

	// ---- 6. 种子数据：settings（品牌 + 通知通道继承 config.toml）与默认模板 ----
	if err := s.seed(db, req); err != nil {
		closeDB(db)
		slog.Error("安装种子数据失败", "err", err)
		return nil, errcode.New(500, errcode.CodeMigrationFailed, "初始化数据失败，详见服务端日志").WithErr(err)
	}

	// ---- 7. 回写配置文件（installed=true、数据库、品牌、JWT secret） ----
	newCfg := *s.app.Cfg() // 复制后修改，热替换，避免并发读写
	newCfg.Database = dbCfg
	newCfg.Brand = config.BrandConfig{
		SiteName:     req.SiteName,
		Logo:         req.Logo,
		PrimaryColor: req.PrimaryColor,
	}
	if newCfg.JWT.Secret == "" {
		secret, err := config.GenerateSecret()
		if err != nil {
			closeDB(db)
			return nil, errcode.New(500, errcode.CodeConfigWriteFailed, "生成 JWT 密钥失败").WithErr(err)
		}
		newCfg.JWT.Secret = secret
	}
	newCfg.Install.Installed = true
	if err := newCfg.Save(s.app.ConfigPath); err != nil {
		closeDB(db)
		slog.Error("回写配置文件失败", "path", s.app.ConfigPath, "err", err)
		return nil, errcode.New(500, errcode.CodeConfigWriteFailed,
			fmt.Sprintf("数据库已初始化，但写配置文件 %s 失败: %v；请修复文件权限后重试，或手工配置后置 installed=true", s.app.ConfigPath, err))
	}

	// ---- 8. 进程内热切换（无需重启，见 ADR-010） ----
	s.app.SetCfg(&newCfg)
	s.app.DB.Store(db)
	s.app.Notify.Store(notify.NewManager(db))
	s.app.Installed.Store(true)
	slog.Info("安装完成，系统已切换为已安装态", "admin_id", admin.ID, "dbname", dbCfg.DBName, "mysql_version", version)

	return &InstallResult{Installed: true, RestartRequired: false, AdminID: admin.ID}, nil
}

// createAdmin 创建管理员账号（用户名冲突返回可读错误）。
func (s *InstallService) createAdmin(db *gorm.DB, req *InstallRequest) (*model.Admin, error) {
	var count int64
	if err := db.Model(&model.Admin{}).Where("username = ?", req.Admin.Username).Count(&count).Error; err != nil {
		return nil, errcode.New(500, errcode.CodeMigrationFailed, "检查管理员用户名失败").WithErr(err)
	}
	if count > 0 {
		return nil, errcode.InvalidParam("管理员用户名 %s 已存在（若为上次安装中断的残留，请更换用户名或清空数据库后重试）", req.Admin.Username)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Admin.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errcode.New(500, errcode.CodeInternalError, "密码哈希失败").WithErr(err)
	}
	admin := &model.Admin{
		Username:     req.Admin.Username,
		PasswordHash: string(hash),
		Email:        req.Admin.Email,
		RealName:     req.Admin.RealName,
		Status:       model.AdminStatusEnabled,
	}
	if err := db.Create(admin).Error; err != nil {
		return nil, errcode.New(500, errcode.CodeMigrationFailed, "创建管理员失败").WithErr(err)
	}
	return admin, nil
}

// seed 写入 settings 种子（品牌 + 从 config.toml 继承的通知通道配置）与默认通知模板。
func (s *InstallService) seed(db *gorm.DB, req *InstallRequest) error {
	cfg := s.app.Cfg()
	kvs := map[string]string{
		model.SettingSiteName:          req.SiteName,
		model.SettingLogo:              req.Logo,
		model.SettingPrimaryColor:      req.PrimaryColor,
		model.SettingSmsbaoUsername:    cfg.Smsbao.Username,
		model.SettingSmsbaoPasswordMD5: cfg.Smsbao.PasswordMD5,
		model.SettingSmsbaoSign:        cfg.Smsbao.Sign,
		model.SettingSMTPHost:          cfg.SMTP.Host,
		model.SettingSMTPPort:          strconv.Itoa(cfg.SMTP.Port),
		model.SettingSMTPUsername:      cfg.SMTP.Username,
		model.SettingSMTPPassword:      cfg.SMTP.Password,
		model.SettingSMTPFrom:          cfg.SMTP.From,
		model.SettingSMTPTLS:           strconv.FormatBool(cfg.SMTP.TLS),
	}
	if err := migration.SeedSettings(db, kvs); err != nil {
		return err
	}
	return migration.SeedTemplates(db)
}

// validateDBParams 校验数据库连接公共参数。
func validateDBParams(host string, port int, user, dbname string) error {
	if host == "" {
		return errcode.InvalidParam("host 不能为空")
	}
	if port < 1 || port > 65535 {
		return errcode.InvalidParam("port 必须在 1-65535 之间")
	}
	if user == "" {
		return errcode.InvalidParam("user 不能为空")
	}
	if dbname != "" && !dbnameRe.MatchString(dbname) {
		return errcode.InvalidParam("dbname 只允许字母/数字/下划线/中划线，且不超过 64 字符")
	}
	return nil
}

// probeMySQL 连接 MySQL 服务器（不指定库）并返回版本号；连接失败返回可读错误。
//
// 两段式探测：
//  1. 先做 TCP 拨号——网络层失败（拒绝/超时/DNS）时拿到的是可读的 dial 错误，
//     而 database/sql 会把底层错误吞成 "invalid connection"；
//  2. 再走 SQL 协议 Ping——覆盖认证失败（Access denied）等错误。
func probeMySQL(ctx context.Context, host string, port int, user, password string) (string, error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	dialCtx, dialCancel := context.WithTimeout(ctx, 5*time.Second)
	defer dialCancel()
	var dialer net.Dialer
	conn, err := dialer.DialContext(dialCtx, "tcp", addr)
	if err != nil {
		return "", fmt.Errorf("无法连接到 %s（请检查地址/端口、MySQL 服务与防火墙）: %w", addr, readableMySQLError(err))
	}
	_ = conn.Close()

	dsn := config.BuildDSN(host, port, user, password, "", "utf8mb4") + "&timeout=5s&readTimeout=10s"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return "", fmt.Errorf("无效的数据库参数: %w", err)
	}
	defer db.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		return "", readableMySQLError(err)
	}
	var version string
	if err := db.QueryRowContext(pingCtx, "SELECT VERSION()").Scan(&version); err != nil {
		return "", readableMySQLError(err)
	}
	return version, nil
}

// createDatabase 建库（IF NOT EXISTS；库名已过正则白名单，反引号包裹双保险）。
func createDatabase(ctx context.Context, host string, port int, user, password, dbname string) error {
	if !dbnameRe.MatchString(dbname) {
		return errcode.InvalidParam("dbname 只允许字母/数字/下划线/中划线")
	}
	dsn := config.BuildDSN(host, port, user, password, "", "utf8mb4") + "&timeout=5s&readTimeout=30s"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return errcode.New(500, errcode.CodeDBConnectFailed, fmt.Sprintf("无效的数据库参数: %v", err))
	}
	defer db.Close()

	stmtCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	stmt := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci", dbname)
	if _, err := db.ExecContext(stmtCtx, stmt); err != nil {
		return errcode.New(500, errcode.CodeMigrationFailed, fmt.Sprintf("创建数据库 %s 失败: %v", dbname, readableMySQLError(err)))
	}
	return nil
}

// versionSupported 校验 MySQL 版本 >= 5.7（兼容 "5.7.44-log"、"10.5.8-MariaDB" 等格式）。
func versionSupported(version string) bool {
	major, minor, ok := parseVersion(version)
	if !ok {
		return false
	}
	return major > 5 || (major == 5 && minor >= 7)
}

// parseVersion 从版本字符串解析主次版本号。
func parseVersion(version string) (major, minor int, ok bool) {
	core := version
	if i := strings.IndexAny(core, "- "); i >= 0 {
		core = core[:i]
	}
	parts := strings.Split(core, ".")
	if len(parts) < 2 {
		return 0, 0, false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return major, minor, true
}

// readableMySQLError 把底层驱动错误整理为管理员可读信息（保留原文关键词，如 Access denied）。
func readableMySQLError(err error) error {
	if err == nil {
		return nil
	}
	// context 超时单独提示，避免展示晦涩的驱动错误
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("连接超时（请检查地址/端口与防火墙）: %w", err)
	}
	// TCP 已连通但对端不是可用的 MySQL（握手失败/中途重置/非 MySQL 服务）：
	// database/sql 会把 driver.ErrBadConn 吞成语义模糊的 "invalid connection"，
	// 这里统一翻译成管理员能据以排查的提示。
	if errors.Is(err, driver.ErrBadConn) ||
		strings.Contains(err.Error(), "invalid connection") ||
		strings.Contains(err.Error(), "bad connection") {
		return fmt.Errorf("无法与目标完成 MySQL 握手（请确认该地址端口确为 MySQL 服务且网络未被中间设备拦截）: %w", err)
	}
	return err
}

// closeDB 关闭 GORM 底层连接（安装失败回滚用）。
func closeDB(db *gorm.DB) {
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}
