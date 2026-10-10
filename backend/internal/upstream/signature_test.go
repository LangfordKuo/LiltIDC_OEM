package upstream

import (
	"crypto/md5" // #nosec G501 -- 测试魔方官方签名算法（协议要求 MD5）
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

// phpEquivalentSign 是魔方（PHP）签名算法的独立等价实现（自证用）：
// ksort 三字段数组 → json_encode（紧凑、按排序后键序）→ md5 → strtoupper。
// 故意不复用生产代码的 Sign()，以手工拼接验证键序与紧凑格式。
func phpEquivalentSign(id, token, randStr string) string {
	// PHP: $arr = ['id'=>$id, 'token'=>$token, 'rand_str'=>$randStr]; ksort($arr);
	// ksort 字符串序："id" < "rand_str" < "token"
	material := `{"id":"` + id + `","rand_str":"` + randStr + `","token":"` + token + `"}`
	sum := md5.Sum([]byte(material)) // #nosec G401 -- 协议要求
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func TestSignMatchesPHPEquivalent(t *testing.T) {
	cases := []struct{ id, token, randStr string }{
		{"12", "tk-secret", "r4nd0m"},
		{"0", "t", "x"},
		{"999999", "32位密钥0123456789abcdef0123456789", "aB3xY9"},
		{"", "tk", "r"}, // id 缺失容错场景
	}
	for _, tc := range cases {
		if got, want := Sign(tc.id, tc.token, tc.randStr), phpEquivalentSign(tc.id, tc.token, tc.randStr); got != want {
			t.Errorf("Sign(%q,%q,%q) = %s, PHP 等价实现 = %s", tc.id, tc.token, tc.randStr, got, want)
		}
	}
}

func TestSignKeyOrder(t *testing.T) {
	// 显式验证签名材料键序为 id → rand_str → token（ksort 结果）
	material := `{"id":"7","rand_str":"rs","token":"tk"}`
	sum := md5.Sum([]byte(material)) // #nosec G401
	want := strings.ToUpper(hex.EncodeToString(sum[:]))
	if got := Sign("7", "tk", "rs"); got != want {
		t.Errorf("键序不符:\n got=%s\nwant=%s", got, want)
	}
}

func TestVerifySignatureValid(t *testing.T) {
	token := "downstream-token-abc"
	// id 为 JSON 数字（上游推送常见形态）
	body := map[string]any{
		"id":        12,
		"host_id":   9001,
		"domain":    "demo.example.com",
		"rand_str":  "r4nd",
		"signature": Sign("12", token, "r4nd"), // json.Number 保留原文 "12"
	}
	raw, _ := json.Marshal(body)
	if err := VerifySignature(raw, token); err != nil {
		t.Errorf("合法签名（数字 id）应通过: %v", err)
	}

	// id 为字符串形态同样通过（签名材料一致）
	body["id"] = "12"
	body["signature"] = Sign("12", token, "r4nd")
	raw, _ = json.Marshal(body)
	if err := VerifySignature(raw, token); err != nil {
		t.Errorf("合法签名（字符串 id）应通过: %v", err)
	}
}

func TestVerifySignatureTampered(t *testing.T) {
	token := "downstream-token-abc"
	valid := Sign("12", token, "r4nd")

	cases := []struct {
		name string
		body map[string]any
	}{
		{"篡改 rand_str", map[string]any{"id": 12, "rand_str": "r4nd-TAMPERED", "signature": valid}},
		{"篡改 id", map[string]any{"id": 13, "rand_str": "r4nd", "signature": valid}},
		{"篡改 signature", map[string]any{"id": 12, "rand_str": "r4nd", "signature": strings.Repeat("A", 32)}},
		{"缺 signature", map[string]any{"id": 12, "rand_str": "r4nd"}},
		{"缺 rand_str", map[string]any{"id": 12, "signature": valid}},
		{"小写 signature 可通过（大小写不敏感）", nil}, // 特殊处理见下
	}
	for _, tc := range cases {
		if tc.body == nil {
			// 上游若回传小写签名：验证方做大写归一后应通过（宽容但安全）
			body := map[string]any{"id": 12, "rand_str": "r4nd", "signature": strings.ToLower(valid)}
			raw, _ := json.Marshal(body)
			if err := VerifySignature(raw, token); err != nil {
				t.Errorf("小写签名应归一后通过: %v", err)
			}
			continue
		}
		raw, _ := json.Marshal(tc.body)
		if err := VerifySignature(raw, token); err == nil {
			t.Errorf("%s：应验签失败", tc.name)
		}
	}
}

func TestVerifySignatureEdgeCases(t *testing.T) {
	if err := VerifySignature([]byte("not-json"), "tk"); err == nil {
		t.Error("非法 JSON 应报错")
	}
	body := map[string]any{"id": 1, "rand_str": "r", "signature": Sign("1", "tk", "r")}
	raw, _ := json.Marshal(body)
	if err := VerifySignature(raw, ""); err == nil {
		t.Error("token 未配置应报错（而不是全部拒绝/全部放行）")
	}
}
