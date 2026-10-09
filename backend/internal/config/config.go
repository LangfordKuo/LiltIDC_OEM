// Package config 负责 TOML 配置文件的加载、默认值、校验与回写。
//
// 配置文件路径解析顺序：
//  1. 环境变量 LILTIDC_CONFIG 指定的路径；
//  2. 默认路径 ./config.toml（相对进程工作目录，通常是 backend/）。
package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// EnvConfigPath 是覆盖配置文件路径的环境变量名。
const EnvConfigPath = "LILTIDC_CONFIG"

// DefaultConfigPath 是未设置环境变量时使用的默认配置文件路径。
const DefaultConfigPath = "config.toml"

// Config 是后端全部配置的根结构，与 config.toml 一一对应。
type Config struct {
	Server   ServerConfig   `toml:"server"`
	Database DatabaseConfig `toml:"database"`
	JWT      JWTConfig      `toml:"jwt"`
	Brand    BrandConfig    `toml:"brand"`
	SMTP     SMTPConfig     `toml:"smtp"`
	Smsbao   SmsbaoConfig   `toml:"smsbao"`
	Upstream UpstreamConfig `toml:"upstream"`
	Install  InstallConfig  `toml:"install"`
}

// ServerConfig 是 HTTP 服务器监听配置。
type ServerConfig struct {
	Addr      string `toml:"addr"`       // 监听地址，如 127.0.0.1
	Port      int    `toml:"port"`       // 监听端口
	Mode      string `toml:"mode"`       // gin 模式：debug/release/test
	StaticDir string `toml:"static_dir"` // 可选：前端静态产物目录
	// CorsAllowOrigins 是 CORS 跨域白名单（含协议与端口）。
	// 开发期前后端分离（Vite 5173 → API 8080）需要；生产 Nginx 同域反代不依赖 CORS。
	CorsAllowOrigins []string `toml:"cors_allow_origins"`
}

// DefaultCorsAllowOrigins 返回 CORS 白名单默认值（本机 Vite dev server 地址）。
// 每次返回新切片，避免共享底层数组被意外修改。
func DefaultCorsAllowOrigins() []string {
	return []string{"http://localhost:5173", "http://127.0.0.1:5173"}
}

// ListenAddr 返回拼接好的监听地址（host:port）。
func (s ServerConfig) ListenAddr() string {
	return fmt.Sprintf("%s:%d", s.Addr, s.Port)
}

// DatabaseConfig 是 MySQL 连接配置。安装向导完成时由安装器回写。
type DatabaseConfig struct {
	Host     string `toml:"host"`
	Port     int    `toml:"port"`
	User     string `toml:"user"`
	Password string `toml:"password"`
	DBName   string `toml:"dbname"`
	Charset  string `toml:"charset"`
}

// DSN 返回连接指定数据库的 go-sql-driver DSN。
func (d DatabaseConfig) DSN() string {
	return BuildDSN(d.Host, d.Port, d.User, d.Password, d.DBName, d.Charset)
}

// ServerDSN 返回不指定数据库名的 DSN（用于安装期连接服务器/建库）。
func (d DatabaseConfig) ServerDSN() string {
	return BuildDSN(d.Host, d.Port, d.User, d.Password, "", d.Charset)
}

// BuildDSN 拼接 go-sql-driver/mysql 的 DSN。
// parseTime=True 让 driver 把 DATETIME 解析为 time.Time；loc=Local 使用进程时区。
func BuildDSN(host string, port int, user, password, dbname, charset string) string {
	if charset == "" {
		charset = "utf8mb4"
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/", user, password, host, port)
	if dbname != "" {
		dsn += dbname
	}
	return dsn + fmt.Sprintf("?charset=%s&parseTime=True&loc=Local", charset)
}

// JWTConfig 是 JWT 签发配置。
type JWTConfig struct {
	Secret      string `toml:"secret"`
	ExpireHours int    `toml:"expire_hours"`
}

// BrandConfig 是品牌信息（未安装态的来源；安装后作为 settings 表种子）。
type BrandConfig struct {
	SiteName     string `toml:"site_name"`
	Logo         string `toml:"logo"`
	PrimaryColor string `toml:"primary_color"`
}

// SMTPConfig 是邮件通道种子配置。
type SMTPConfig struct {
	Host     string `toml:"host"`
	Port     int    `toml:"port"`
	Username string `toml:"username"`
	Password string `toml:"password"`
	From     string `toml:"from"`
	TLS      bool   `toml:"tls"` // true = 要求 STARTTLS
}

// SmsbaoConfig 是短信宝通道种子配置。
type SmsbaoConfig struct {
	Username    string `toml:"username"`
	PasswordMD5 string `toml:"password_md5"`
	Sign        string `toml:"sign"`
}

// UpstreamConfig 是上游魔方财务的预留配置（批 2 实现调用逻辑）。
type UpstreamConfig struct {
	Hostname string `toml:"hostname"`
	Username string `toml:"username"`
	APIKey   string `toml:"api_key"`
}

// InstallConfig 是安装状态标记（持久化真源，见 docs/architecture.md ADR-004）。
type InstallConfig struct {
	Installed bool `toml:"installed"`
}

// ResolvePath 解析配置文件路径：优先环境变量 LILTIDC_CONFIG，否则默认 config.toml。
func ResolvePath() string {
	if p := os.Getenv(EnvConfigPath); p != "" {
		return p
	}
	return DefaultConfigPath
}

// Load 从指定路径加载配置，并应用默认值与校验。
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败（%s）: %w；提示：可复制 config.example.toml 为 config.toml，或用环境变量 %s 指定路径", path, err, EnvConfigPath)
	}
	cfg := &Config{}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败（%s）: %w", path, err)
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("配置文件非法（%s）: %w", path, err)
	}
	return cfg, nil
}

// applyDefaults 为零值字段填充默认值（保证手工精简的配置文件也能跑）。
func (c *Config) applyDefaults() {
	if c.Server.Addr == "" {
		c.Server.Addr = "127.0.0.1"
	}
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Server.Mode == "" {
		c.Server.Mode = "debug"
	}
	if len(c.Server.CorsAllowOrigins) == 0 {
		c.Server.CorsAllowOrigins = DefaultCorsAllowOrigins()
	}
	if c.Database.Host == "" {
		c.Database.Host = "127.0.0.1"
	}
	if c.Database.Port == 0 {
		c.Database.Port = 3306
	}
	if c.Database.Charset == "" {
		c.Database.Charset = "utf8mb4"
	}
	if c.JWT.ExpireHours <= 0 {
		c.JWT.ExpireHours = 24
	}
	if c.Brand.SiteName == "" {
		c.Brand.SiteName = "LiltIDC"
	}
	if c.Brand.PrimaryColor == "" {
		c.Brand.PrimaryColor = "#2563eb"
	}
	if c.SMTP.Port == 0 {
		c.SMTP.Port = 25
	}
}

// Validate 做最小限度的合法性校验（宽松策略：未安装时数据库配置允许为空）。
func (c *Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("[server] port 必须在 1-65535 之间，当前为 %d", c.Server.Port)
	}
	switch c.Server.Mode {
	case "debug", "release", "test":
	default:
		return fmt.Errorf("[server] mode 必须是 debug/release/test，当前为 %q", c.Server.Mode)
	}
	if c.Database.Port < 1 || c.Database.Port > 65535 {
		return fmt.Errorf("[database] port 必须在 1-65535 之间，当前为 %d", c.Database.Port)
	}
	if c.JWT.ExpireHours < 1 {
		return fmt.Errorf("[jwt] expire_hours 必须 >= 1，当前为 %d", c.JWT.ExpireHours)
	}
	if c.Install.Installed {
		// 已安装态下数据库配置必须完整，否则启动时连库必然失败，提前报清晰错误
		if c.Database.Host == "" || c.Database.User == "" || c.Database.DBName == "" {
			return fmt.Errorf("[install] installed = true 但 [database] 配置不完整（host/user/dbname 均不可为空）")
		}
		if c.JWT.Secret == "" {
			return fmt.Errorf("[install] installed = true 但 [jwt] secret 为空")
		}
	}
	return nil
}

// Save 将配置回写到指定路径（安装向导完成时调用）。
// 权限 0600：文件含数据库密码与 JWT secret。注意回写会丢失原文件中的注释。
func (c *Config) Save(path string) error {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("创建配置目录失败: %w", err)
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("打开配置文件失败: %w", err)
	}
	defer f.Close()
	if err := toml.NewEncoder(f).Encode(c); err != nil {
		return fmt.Errorf("编码配置文件失败: %w", err)
	}
	return nil
}

// GenerateSecret 生成 32 字节加密随机数的十六进制串（64 字符），用作 JWT secret。
func GenerateSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("生成随机密钥失败: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
