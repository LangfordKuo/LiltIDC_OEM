package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTemp 在临时目录写入配置文件并返回路径。
func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写临时配置失败: %v", err)
	}
	return path
}

const sampleTOML = `
[server]
addr = "0.0.0.0"
port = 9090
mode = "release"
cors_allow_origins = ["https://console.example.com"]

[database]
host = "db.example.com"
port = 3307
user = "lilt"
password = "pwd"
dbname = "liltidc"
charset = "utf8mb4"

[jwt]
secret = "test-secret"
expire_hours = 12

[brand]
site_name = "测试站点"
logo = "/logo.png"
primary_color = "#ff6600"

[smtp]
host = "smtp.example.com"
port = 465
username = "u"
password = "p"
from = "u@example.com"
tls = true

[smsbao]
username = "smsuser"
password_md5 = "0123456789abcdef0123456789abcdef"
sign = "岭云互联"

[upstream]
hostname = "upstream.example.com"
username = "up"
api_key = "key"

[install]
installed = true
`

func TestLoadFull(t *testing.T) {
	path := writeTemp(t, sampleTOML)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if cfg.Server.Addr != "0.0.0.0" || cfg.Server.Port != 9090 || cfg.Server.Mode != "release" {
		t.Errorf("[server] 解析错误: %+v", cfg.Server)
	}
	if len(cfg.Server.CorsAllowOrigins) != 1 || cfg.Server.CorsAllowOrigins[0] != "https://console.example.com" {
		t.Errorf("cors_allow_origins 解析错误: %v", cfg.Server.CorsAllowOrigins)
	}
	if cfg.Server.ListenAddr() != "0.0.0.0:9090" {
		t.Errorf("ListenAddr 错误: %s", cfg.Server.ListenAddr())
	}
	if cfg.Database.Host != "db.example.com" || cfg.Database.Port != 3307 || cfg.Database.DBName != "liltidc" {
		t.Errorf("[database] 解析错误: %+v", cfg.Database)
	}
	if cfg.JWT.Secret != "test-secret" || cfg.JWT.ExpireHours != 12 {
		t.Errorf("[jwt] 解析错误: %+v", cfg.JWT)
	}
	if cfg.Brand.SiteName != "测试站点" || cfg.Brand.PrimaryColor != "#ff6600" {
		t.Errorf("[brand] 解析错误: %+v", cfg.Brand)
	}
	if !cfg.SMTP.TLS || cfg.SMTP.Port != 465 {
		t.Errorf("[smtp] 解析错误: %+v", cfg.SMTP)
	}
	if cfg.Smsbao.Sign != "岭云互联" || cfg.Smsbao.PasswordMD5 != "0123456789abcdef0123456789abcdef" {
		t.Errorf("[smsbao] 解析错误: %+v", cfg.Smsbao)
	}
	if cfg.Upstream.Hostname != "upstream.example.com" || cfg.Upstream.APIKey != "key" {
		t.Errorf("[upstream] 解析错误: %+v", cfg.Upstream)
	}
	if !cfg.Install.Installed {
		t.Error("[install] installed 应为 true")
	}
}

func TestLoadDefaults(t *testing.T) {
	// 空文件也应加载成功并得到全部默认值
	path := writeTemp(t, "")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if cfg.Server.Addr != "127.0.0.1" || cfg.Server.Port != 8080 || cfg.Server.Mode != "debug" {
		t.Errorf("server 默认值错误: %+v", cfg.Server)
	}
	want := DefaultCorsAllowOrigins()
	if len(cfg.Server.CorsAllowOrigins) != len(want) {
		t.Fatalf("cors 默认值错误: %v", cfg.Server.CorsAllowOrigins)
	}
	for i := range want {
		if cfg.Server.CorsAllowOrigins[i] != want[i] {
			t.Errorf("cors 默认值错误: %v", cfg.Server.CorsAllowOrigins)
		}
	}
	if cfg.Database.Host != "127.0.0.1" || cfg.Database.Port != 3306 || cfg.Database.Charset != "utf8mb4" {
		t.Errorf("database 默认值错误: %+v", cfg.Database)
	}
	if cfg.JWT.ExpireHours != 24 {
		t.Errorf("jwt 默认值错误: %+v", cfg.JWT)
	}
	if cfg.Brand.SiteName != "LiltIDC" || cfg.Brand.PrimaryColor != "#2563eb" {
		t.Errorf("brand 默认值错误: %+v", cfg.Brand)
	}
	if cfg.SMTP.Port != 25 {
		t.Errorf("smtp 默认值错误: %+v", cfg.SMTP)
	}
	if cfg.Install.Installed {
		t.Error("install 默认应为未安装")
	}
}

func TestLoadFileNotFound(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nonexistent.toml"))
	if err == nil {
		t.Fatal("文件不存在时应返回错误")
	}
	if !strings.Contains(err.Error(), "读取配置文件失败") {
		t.Errorf("错误信息不清晰: %v", err)
	}
}

func TestLoadInvalidTOML(t *testing.T) {
	path := writeTemp(t, "this is not [valid toml")
	if _, err := Load(path); err == nil {
		t.Fatal("非法 TOML 应返回错误")
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{"默认合法", func(c *Config) {}, false},
		{"server端口越界", func(c *Config) { c.Server.Port = 70000 }, true},
		{"server模式非法", func(c *Config) { c.Server.Mode = "prod" }, true},
		{"jwt过期为0", func(c *Config) { c.JWT.ExpireHours = 0 }, true},
		{"已安装但库名缺失", func(c *Config) { c.Install.Installed = true; c.JWT.Secret = "s"; c.Database.DBName = "" }, true},
		{"已安装但secret缺失", func(c *Config) { c.Install.Installed = true; c.Database.DBName = "d" }, true},
		{"已安装且配置完整", func(c *Config) {
			c.Install.Installed = true
			c.JWT.Secret = "s"
			c.Database.User = "root"
			c.Database.DBName = "d"
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{}
			cfg.applyDefaults()
			tc.mutate(cfg)
			err := cfg.Validate()
			if tc.wantErr && err == nil {
				t.Error("期望报错但没有")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("不期望报错但得到: %v", err)
			}
		})
	}
}

func TestResolvePath(t *testing.T) {
	t.Setenv(EnvConfigPath, "")
	os.Unsetenv(EnvConfigPath)
	if got := ResolvePath(); got != DefaultConfigPath {
		t.Errorf("无环境变量时应返回默认路径 %q，得到 %q", DefaultConfigPath, got)
	}
	t.Setenv(EnvConfigPath, "/custom/path/config.toml")
	if got := ResolvePath(); got != "/custom/path/config.toml" {
		t.Errorf("环境变量覆盖失败，得到 %q", got)
	}
}

func TestSaveRoundTrip(t *testing.T) {
	path := writeTemp(t, sampleTOML)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	// 模拟安装器回写
	out := filepath.Join(t.TempDir(), "out.toml")
	cfg.Install.Installed = true
	cfg.Database.Password = "newpwd"
	if err := cfg.Save(out); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}
	reloaded, err := Load(out)
	if err != nil {
		t.Fatalf("重新加载失败: %v", err)
	}
	if !reloaded.Install.Installed || reloaded.Database.Password != "newpwd" {
		t.Errorf("回写内容不一致: %+v", reloaded.Install)
	}
	if reloaded.Smsbao.Sign != cfg.Smsbao.Sign || reloaded.Brand.SiteName != cfg.Brand.SiteName {
		t.Errorf("中文字段回写不一致: %+v", reloaded.Brand)
	}
}

func TestBuildDSN(t *testing.T) {
	got := BuildDSN("127.0.0.1", 3306, "root", "pwd", "liltidc", "")
	want := "root:pwd@tcp(127.0.0.1:3306)/liltidc?charset=utf8mb4&parseTime=True&loc=Local"
	if got != want {
		t.Errorf("DSN 错误:\n got: %s\nwant: %s", got, want)
	}
	server := BuildDSN("h", 3307, "u", "", "", "utf8mb4")
	if !strings.HasSuffix(server, "u:@tcp(h:3307)/?charset=utf8mb4&parseTime=True&loc=Local") &&
		server != "u:@tcp(h:3307)/?charset=utf8mb4&parseTime=True&loc=Local" {
		t.Errorf("ServerDSN 错误: %s", server)
	}
}

func TestGenerateSecret(t *testing.T) {
	s1, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret 失败: %v", err)
	}
	s2, _ := GenerateSecret()
	if len(s1) != 64 {
		t.Errorf("secret 长度应为 64，得到 %d", len(s1))
	}
	if s1 == s2 {
		t.Error("两次生成的 secret 不应相同")
	}
}
