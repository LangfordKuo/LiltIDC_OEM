package service

// 开通转发纯函数单测（无 DB/网络依赖）：下单表单规则解析、主机名/密码生成、
// provision_info 容错、备注拼接与状态文案。

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/model"
)

// pid=1（lyew.com 美国一区 Cera/香港 CN2 同形）get_product_config 的真实规则字段：
// host/password 是双重编码的 JSON 字符串。
const sampleConfigJSON = `{"products":{"id":1,"name":"香港二区 CN2 A型",
 "host":"{\"show\":\"1\",\"modify\":0,\"prefix\":\"HK-\",\"rule\":{\"upper\":\"1\",\"lower\":\"0\",\"num\":\"1\",\"len_num\":\"4\"}}",
 "password":"{\"show\":\"1\",\"modify\":0,\"rule\":{\"len_num\":\"10\",\"upper\":\"1\",\"lower\":\"1\",\"num\":\"1\",\"special\":\"0\"}}"}}`

func TestParseOrderFormRules(t *testing.T) {
	rules := parseOrderFormRules(json.RawMessage(sampleConfigJSON))
	if !rules.HostRequired || rules.HostPrefix != "HK-" || rules.HostMinLen != 4 {
		t.Errorf("host 规则解析错误: %+v", rules)
	}
	if !rules.PasswordRequired {
		t.Errorf("password 规则解析错误: %+v", rules)
	}
	// 容错：空/非法 JSON、无 products 字段 → 零值规则（不 panic）
	for _, bad := range []string{"", "not-json", `{}`, `{"products":{}}`, `{"products":{"host":"bad","password":"bad"}}`} {
		r := parseOrderFormRules(json.RawMessage(bad))
		if r.HostRequired || r.PasswordRequired || r.HostPrefix != "" {
			t.Errorf("非法输入 %q 应容错为零值规则: %+v", bad, r)
		}
	}
}

func TestGenerateHostname(t *testing.T) {
	rules := parseOrderFormRules(json.RawMessage(sampleConfigJSON))
	for i := 0; i < 50; i++ {
		h := generateHostname(rules)
		if !strings.HasPrefix(h, "HK-") {
			t.Fatalf("主机名应带前缀 HK-: %q", h)
		}
		if len(h) < 8 {
			t.Fatalf("主机名长度应 ≥8（兜底最小长度）: %q", h)
		}
		for _, r := range strings.TrimPrefix(h, "HK-") {
			if !strings.ContainsRune("ABCDEFGHJKLMNPQRSTUVWXYZ23456789", r) {
				t.Fatalf("随机段应只含大写字母与数字（去混淆字符）: %q", h)
			}
		}
	}
	// 唯一性抽查（12 位随机空间足够大，重复即随机源异常）
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		h := generateHostname(rules)
		if seen[h] {
			t.Fatalf("主机名重复（随机源异常）: %q", h)
		}
		seen[h] = true
	}
	// 长前缀 + 高 len_num：总长满足要求
	long := orderFormRules{HostRequired: true, HostPrefix: "SERVER-", HostMinLen: 12}
	if h := generateHostname(long); len(h) < 12 {
		t.Errorf("应满足 len_num=12: %q", h)
	}
}

func TestGenerateHostPassword(t *testing.T) {
	const upper = "ABCDEFGHJKLMNPQRSTUVWXYZ"
	const lower = "abcdefghijkmnpqrstuvwxyz"
	const digits = "23456789"
	for i := 0; i < 100; i++ {
		p := generateHostPassword()
		if len(p) != 12 {
			t.Fatalf("密码长度应为 12: %q", p)
		}
		var nUpper, nLower, nDigit int
		for _, r := range p {
			switch {
			case strings.ContainsRune(upper, r):
				nUpper++
			case strings.ContainsRune(lower, r):
				nLower++
			case strings.ContainsRune(digits, r):
				nDigit++
			default:
				t.Fatalf("密码含规则外字符（特殊字符/混淆字符）: %q", p)
			}
		}
		// 上游 checkHostPassword 事实上强制 大写+小写+数字 三类齐备
		if nUpper < 1 || nLower < 1 || nDigit < 1 {
			t.Fatalf("密码字符类不齐备（上%d 下%d 数%d）: %q", nUpper, nLower, nDigit, p)
		}
	}
}

func TestLoadProvisionInfoTolerance(t *testing.T) {
	// 空 → 新对象
	it := &model.OrderItem{}
	if info := loadProvisionInfo(it); info == nil || info.Attempts != 0 {
		t.Error("空 provision_info 应容错为新对象")
	}
	// 损坏 JSON → 重建（不 panic、不报错阻断开通）
	it.ProvisionInfo = "{broken"
	if info := loadProvisionInfo(it); info == nil || info.Hostname != "" {
		t.Error("损坏 provision_info 应容错重建")
	}
	// 正常往返
	it.ProvisionInfo = `{"hostname":"HK-A1","password":"pw","host_ids":[3,4],"attempts":2}`
	info := loadProvisionInfo(it)
	if info.Hostname != "HK-A1" || len(info.HostIDs) != 2 || info.HostIDs[1] != 4 || info.Attempts != 2 {
		t.Errorf("provision_info 解析错误: %+v", info)
	}
}

func TestAppendRemarkTruncate(t *testing.T) {
	long := strings.Repeat("长", 300) // 900 字节
	got := appendRemark("原备注", long)
	if len(got) > 512 {
		t.Errorf("备注应截断到 512 字节内，得到 %d", len(got))
	}
	if !strings.HasPrefix(got, "原备注；") {
		t.Errorf("应以分号拼接旧备注: %q", got[:20])
	}
	if r := appendRemark("", "新"); r != "新" {
		t.Errorf("空旧备注不应带分隔符: %q", r)
	}
}

func TestOrderStatusTextBilingual(t *testing.T) {
	cases := []struct {
		status, zh, en string
	}{
		{model.OrderStatusPending, "待支付", "Pending Payment"},
		{model.OrderStatusPaid, "开通中", "Provisioning"},
		{model.OrderStatusActive, "已开通", "Active"},
		{model.OrderStatusProvisionFailed, "开通失败", "Provision Failed"},
		{model.OrderStatusCancelled, "已取消", "Cancelled"},
		{model.OrderStatusRefunded, "已退款", "Refunded"},
		{"Weird", "未知", "Unknown"},
	}
	for _, c := range cases {
		if got := OrderStatusText(c.status, "zh-CN"); got != c.zh {
			t.Errorf("%s zh 文案 = %q，期望 %q", c.status, got, c.zh)
		}
		if got := OrderStatusText(c.status, "en-US"); got != c.en {
			t.Errorf("%s en 文案 = %q，期望 %q", c.status, got, c.en)
		}
	}
}

func TestProvisionStatusTextBilingual(t *testing.T) {
	if got := ProvisionStatusText(model.ProvisionProvisioning, "zh-CN"); got != "开通中" {
		t.Errorf("zh 文案错误: %s", got)
	}
	if got := ProvisionStatusText(model.ProvisionActive, "en"); got != "Active" {
		t.Errorf("en 前缀应命中英文: %s", got)
	}
}

func TestCycleTextBilingual(t *testing.T) {
	if got := CycleText("monthly", "zh-CN"); got != "月付" {
		t.Errorf("monthly zh = %q", got)
	}
	if got := CycleText("semiannually", "en-US"); got != "Semi-annually" {
		t.Errorf("semiannually en = %q", got)
	}
	if got := CycleText("weirdcycle", "zh-CN"); got != "weirdcycle" {
		t.Errorf("未知周期应原样返回键名: %q", got)
	}
}

func TestIsKnownOrderStatus(t *testing.T) {
	for _, ok := range []string{"Pending", "Paid", "Active", "ProvisionFailed", "Cancelled", "Refunded"} {
		if !isKnownOrderStatus(ok) {
			t.Errorf("%s 应为合法状态", ok)
		}
	}
	for _, bad := range []string{"", "pending", "Unknown", "支付"} {
		if isKnownOrderStatus(bad) {
			t.Errorf("%q 应被拒绝", bad)
		}
	}
}
