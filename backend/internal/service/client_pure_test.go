package service

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateClientPassword(t *testing.T) {
	cases := []struct {
		pw      string
		wantErr bool
	}{
		{"Passw0rd", false}, // 字母+数字 8 位
		{"abc12345", false},
		{"Ab1", true},                     // 太短
		{"abcdefgh", true},                // 无数字
		{"12345678", true},                // 无字母
		{"", true},                        // 空
		{strings.Repeat("a1", 36), false}, // 72 位 = 上限内
		{strings.Repeat("a1", 40), true},  // 80 位 > 72（bcrypt 上限）
	}
	for _, tc := range cases {
		err := validateClientPassword(tc.pw)
		if tc.wantErr && err == nil {
			t.Errorf("%q 应被拒绝", tc.pw)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("%q 不应被拒绝: %v", tc.pw, err)
		}
	}
}

func TestClientUsernameRegex(t *testing.T) {
	if !clientUsernameRe.MatchString("user_01") {
		t.Error("合法用户名应通过")
	}
	for _, bad := range []string{"ab", "a-b", "用户", strings.Repeat("u", 33), ""} {
		if clientUsernameRe.MatchString(bad) {
			t.Errorf("非法用户名 %q 应被拒绝", bad)
		}
	}
}

func TestClientPhoneRegex(t *testing.T) {
	if !clientPhoneRe.MatchString("13800138000") {
		t.Error("合法手机号应通过")
	}
	for _, bad := range []string{"12800138000", "1380013800", "138001380000", "abc", ""} {
		if clientPhoneRe.MatchString(bad) {
			t.Errorf("非法手机号 %q 应被拒绝", bad)
		}
	}
}

func TestParseClientStatus(t *testing.T) {
	for _, ok := range []string{"0", "1", "2", "3"} {
		if _, err := parseClientStatus(ok); err != nil {
			t.Errorf("status %s 应合法: %v", ok, err)
		}
	}
	for _, bad := range []string{"4", "-1", "abc", ""} {
		if _, err := parseClientStatus(bad); err == nil {
			t.Errorf("status %q 应被拒绝", bad)
		}
	}
}

func TestClientStatusText(t *testing.T) {
	if got := clientStatusText(3, "zh-CN"); got != "上游关联异常" {
		t.Errorf("zh 状态文案错误: %s", got)
	}
	if got := clientStatusText(3, "en-US"); got != "Upstream Issue" {
		t.Errorf("en 状态文案错误: %s", got)
	}
	if got := clientStatusText(1, "en"); got != "Active" {
		t.Errorf("en 前缀应命中英文: %s", got)
	}
	if got := clientStatusText(9, "zh-CN"); got != "未知" {
		t.Errorf("未知状态兜底错误: %s", got)
	}
}

func TestNilIfEmptyAndStrVal(t *testing.T) {
	if p := nilIfEmpty(""); p != nil {
		t.Error("空串应转 nil")
	}
	if p := nilIfEmpty("a@b.c"); p == nil || *p != "a@b.c" {
		t.Error("非空串应保留")
	}
	if strVal(nil) != "" {
		t.Error("nil 应转空串")
	}
	v := "x"
	if strVal(&v) != "x" {
		t.Error("指针应解引用")
	}
}

func TestTruncateTo(t *testing.T) {
	if got := truncateTo("abc", 5); got != "abc" {
		t.Errorf("未超长不应截断: %s", got)
	}
	if got := truncateTo("abcdef", 3); got != "abc" {
		t.Errorf("超长应截断: %s", got)
	}
}

func TestIsDuplicateKeyErr(t *testing.T) {
	if isDuplicateKeyErr(nil) {
		t.Error("nil 不是重复键")
	}
	if !isDuplicateKeyErr(errors.New("Error 1062 (23000): Duplicate entry 'a' for key 'idx_clients_username'")) {
		t.Error("1062/Duplicate entry 应识别")
	}
	if isDuplicateKeyErr(errors.New("connection refused")) {
		t.Error("普通错误不应误判")
	}
}
