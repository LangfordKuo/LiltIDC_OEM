package notify

import (
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	cases := []struct {
		name string
		tpl  string
		vars map[string]string
		want string
	}{
		{
			name: "单占位符",
			tpl:  "您的验证码是 {code}，10 分钟内有效。",
			vars: map[string]string{"code": "824913"},
			want: "您的验证码是 824913，10 分钟内有效。",
		},
		{
			name: "多占位符",
			tpl:  "尊敬的 {username}，账单 {bill_no} 金额 {amount} 元将于 {due_date} 到期。",
			vars: map[string]string{"username": "张三", "bill_no": "INV-1", "amount": "12.00", "due_date": "2026-11-01"},
			want: "尊敬的 张三，账单 INV-1 金额 12.00 元将于 2026-11-01 到期。",
		},
		{
			name: "未知占位符原样保留",
			tpl:  "验证码 {code}，未知 {unknown_key}",
			vars: map[string]string{"code": "1234"},
			want: "验证码 1234，未知 {unknown_key}",
		},
		{
			name: "替换值中的花括号不被二次替换",
			tpl:  "hello {name}",
			vars: map[string]string{"name": "{name}", "code": "x"},
			want: "hello {name}",
		},
		{
			name: "无占位符模板",
			tpl:  "固定内容",
			vars: map[string]string{"code": "1"},
			want: "固定内容",
		},
		{
			name: "空 vars",
			tpl:  "{site_name}",
			vars: nil,
			want: "{site_name}",
		},
		{
			name: "同一占位符出现多次",
			tpl:  "{a}-{a}-{b}",
			vars: map[string]string{"a": "1", "b": "2"},
			want: "1-1-2",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Render(tc.tpl, tc.vars); got != tc.want {
				t.Errorf("Render 错误:\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}

func TestRenderHTMLTemplate(t *testing.T) {
	tpl := "<p>您的验证码是：<b>{code}</b></p><p>—— {site_name}</p>"
	got := Render(tpl, map[string]string{"code": "9999", "site_name": "LiltIDC"})
	if !strings.Contains(got, "<b>9999</b>") || !strings.Contains(got, "LiltIDC") {
		t.Errorf("HTML 模板渲染错误: %s", got)
	}
}
