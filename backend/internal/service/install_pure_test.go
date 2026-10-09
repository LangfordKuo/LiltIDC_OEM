package service

import (
	"testing"
)

func TestVersionSupported(t *testing.T) {
	cases := []struct {
		version string
		want    bool
	}{
		{"5.7.44", true},
		{"5.7.0", true},
		{"5.7.44-log", true},
		{"8.0.32", true},
		{"8.4.0", true},
		{"10.5.8-MariaDB", true}, // MariaDB 主版本 10 > 5
		{"5.6.51", false},        // 低于 5.7
		{"5.6.0", false},
		{"5.5.62-log", false},
		{"garbage", false},
		{"", false},
		{"5", false}, // 缺次版本
	}
	for _, tc := range cases {
		if got := versionSupported(tc.version); got != tc.want {
			t.Errorf("versionSupported(%q) = %v, want %v", tc.version, got, tc.want)
		}
	}
}

func TestParseVersion(t *testing.T) {
	major, minor, ok := parseVersion("5.7.44-log")
	if !ok || major != 5 || minor != 7 {
		t.Errorf("parseVersion 解析错误: %d.%d ok=%v", major, minor, ok)
	}
	if _, _, ok := parseVersion("bad"); ok {
		t.Error("非法版本串应返回 ok=false")
	}
}

func TestValidateDBParams(t *testing.T) {
	cases := []struct {
		name    string
		host    string
		port    int
		user    string
		dbname  string
		wantErr bool
	}{
		{"合法", "127.0.0.1", 3306, "root", "liltidc_oem", false},
		{"dbname 为空合法（dbtest 不指定库）", "127.0.0.1", 3306, "root", "", false},
		{"host 为空", "", 3306, "root", "db", true},
		{"port 越界", "h", 70000, "u", "db", true},
		{"port 为 0", "h", 0, "u", "db", true},
		{"user 为空", "h", 3306, "", "db", true},
		{"dbname 含非法字符（防注入）", "h", 3306, "u", "db`; DROP", true},
		{"dbname 含反引号", "h", 3306, "u", "a`b", true},
		{"dbname 合法下划线中划线", "h", 3306, "u", "my_db-1", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateDBParams(tc.host, tc.port, tc.user, tc.dbname)
			if tc.wantErr && err == nil {
				t.Error("期望报错但没有")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("不期望报错但得到: %v", err)
			}
		})
	}
}

func TestToMD5(t *testing.T) {
	// 已是 32 位十六进制：原样（转小写）返回
	in := "0123456789ABCDEF0123456789ABCDEF"
	if got := toMD5(in); got != "0123456789abcdef0123456789abcdef" {
		t.Errorf("已 MD5 值应转小写原样返回，得到 %q", got)
	}
	// 明文 "123456" 的 MD5 是公知的 e10adc3949ba59abbe56e057f20f883e
	if got := toMD5("123456"); got != "e10adc3949ba59abbe56e057f20f883e" {
		t.Errorf("明文 MD5 计算错误，得到 %q", got)
	}
	// 非 32 位的十六进制样串应被当作明文再哈希（长度变化）
	if got := toMD5("abc"); len(got) != 32 {
		t.Errorf("MD5 结果长度应为 32，得到 %d", len(got))
	}
}

func TestContains(t *testing.T) {
	list := []string{"sms", "email"}
	if !contains(list, "sms") {
		t.Error("应包含 sms")
	}
	if contains(list, "wechat") {
		t.Error("不应包含 wechat")
	}
	if contains(nil, "x") {
		t.Error("空列表不应包含任何值")
	}
}

func TestColorAndAdminRegex(t *testing.T) {
	if !colorRe.MatchString("#2563eb") || !colorRe.MatchString("#AABBCC") {
		t.Error("合法颜色应通过")
	}
	if colorRe.MatchString("2563eb") || colorRe.MatchString("#12345") || colorRe.MatchString("#GGGGGG") {
		t.Error("非法颜色应被拒绝")
	}
	if !adminUserRe.MatchString("admin_01") {
		t.Error("合法管理员用户名应通过")
	}
	if adminUserRe.MatchString("ab") || adminUserRe.MatchString("a-b") || adminUserRe.MatchString("用户名") {
		t.Error("非法管理员用户名应被拒绝")
	}
	if !dbnameRe.MatchString("liltidc_oem") {
		t.Error("合法库名应通过")
	}
	if dbnameRe.MatchString("db name") || dbnameRe.MatchString("db;") {
		t.Error("非法库名应被拒绝")
	}
}
