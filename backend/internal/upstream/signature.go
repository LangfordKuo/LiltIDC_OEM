package upstream

import (
	"bytes"
	"crypto/md5" // #nosec G501 -- 魔方财务官方签名算法要求 MD5，非安全用途
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Sign 计算魔方 host/sync 推送签名（官方算法，逐字实现，见契约文档 9.6）：
//
//	取 {id, token, rand_str} 三字段 → 按键名 ksort（字符串序）→
//	紧凑 JSON 序列化（无空格，键序为排序后顺序）→ MD5 → 转大写。
//
// Go 的 json.Marshal 对 map[string]string 天然按键名字典序输出且紧凑无空格，
// 与 PHP ksort + json_encode 的产物逐字节等价（单测中以手工拼接自证）。
// 键序：id < rand_str < token。
func Sign(id, token, randStr string) string {
	payload := map[string]string{
		"id":       id,
		"token":    token,
		"rand_str": randStr,
	}
	data, _ := json.Marshal(payload)
	sum := md5.Sum(data) // #nosec G401 -- 协议要求
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

// VerifySignature 验证上游推送的请求体原文（签名比对用常数时间，防时序侧信道）。
//
// id 字段用 json.Number 保留 JSON 原文（数字 12 保持 "12"），
// 与 PHP 端把 $_POST/JSON 字段转字符串后的签名材料一致。
func VerifySignature(rawBody []byte, token string) error {
	if token == "" {
		return errors.New("下游密钥（downstream_token）未配置，无法验签")
	}
	dec := json.NewDecoder(bytes.NewReader(rawBody))
	dec.UseNumber()
	var body map[string]any
	if err := dec.Decode(&body); err != nil {
		return fmt.Errorf("推送请求体不是合法 JSON: %w", err)
	}
	sig := jsonValueToString(body["signature"])
	randStr := jsonValueToString(body["rand_str"])
	idStr := jsonValueToString(body["id"])
	if sig == "" {
		return errors.New("推送缺少 signature 字段")
	}
	if randStr == "" {
		return errors.New("推送缺少 rand_str 字段")
	}
	expect := Sign(idStr, token, randStr)
	if subtle.ConstantTimeCompare([]byte(strings.ToUpper(sig)), []byte(expect)) != 1 {
		return errors.New("签名不匹配")
	}
	return nil
}

// jsonValueToString 宽松地把 JSON 值转为字符串（数字保留原文；缺失返回空串）。
func jsonValueToString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", t)
	}
}
