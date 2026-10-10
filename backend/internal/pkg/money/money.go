// Package money 提供金额运算工具：十进制金额文本 ↔ 整数分（int64）。
//
// 设计取舍（见 docs/architecture.md ADR-027）：
//   - 数据库列一律 DECIMAL(12,2)，API/存储传输一律十进制字符串（与 PriceMap 同风格）；
//   - Go 层一切加减比较运算在**整数分**域完成——严禁 float64 参与金额运算
//     （二进制浮点无法精确表示 0.1，累加会产生分位漂移）；
//   - 解析严格：只接受 ^[+-]?\d+(\.\d{1,2})?$（超过两位小数直接拒绝，
//     不做静默舍入——金额歧义必须暴露为错误）。
package money

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrInvalidAmount 是金额文本非法错误。
var ErrInvalidAmount = errors.New("金额格式非法（须为最多两位小数的十进制数字，如 12.34）")

// maxCents 是 DECIMAL(12,2) 能表示的最大分值（9999999999.99 元）。
const maxCents int64 = 999999999999

// ParseCents 把十进制金额文本解析为整数分。
// 接受 "12"、"12.3"、"12.34"、"+12.34"、"-12.34"；拒绝空串、非数字、
// 超过两位小数与超出 DECIMAL(12,2) 范围的值。
func ParseCents(s string) (int64, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return 0, ErrInvalidAmount
	}
	neg := false
	switch t[0] {
	case '+':
		t = t[1:]
	case '-':
		neg = true
		t = t[1:]
	}
	if t == "" {
		return 0, ErrInvalidAmount
	}
	intPart, fracPart := t, ""
	if i := strings.IndexByte(t, '.'); i >= 0 {
		intPart, fracPart = t[:i], t[i+1:]
		// "5."（空小数段）与 "1.2.3"（多点）、超过两位小数一律拒绝
		if len(fracPart) == 0 || len(fracPart) > 2 || strings.IndexByte(fracPart, '.') >= 0 {
			return 0, ErrInvalidAmount
		}
	}
	if intPart == "" && fracPart == "" {
		return 0, ErrInvalidAmount
	}
	// 逐字符校验数字（不用 strconv 对整段的宽松行为）
	for _, r := range intPart {
		if r < '0' || r > '9' {
			return 0, ErrInvalidAmount
		}
	}
	for _, r := range fracPart {
		if r < '0' || r > '9' {
			return 0, ErrInvalidAmount
		}
	}
	ip, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return 0, ErrInvalidAmount
	}
	fp, err := strconv.ParseInt(fracPart+"00"[:2-len(fracPart)], 10, 64)
	if err != nil {
		return 0, ErrInvalidAmount
	}
	cents := ip*100 + fp
	if cents > maxCents {
		return 0, fmt.Errorf("%w: 超出 DECIMAL(12,2) 范围", ErrInvalidAmount)
	}
	if neg {
		cents = -cents
	}
	return cents, nil
}

// FormatCents 把整数分格式化为两位小数的十进制文本（0 → "0.00"，-520 → "-5.20"）。
func FormatCents(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
}

// IsValid 判断金额文本是否可解析（校验场景快捷判断）。
func IsValid(s string) bool {
	_, err := ParseCents(s)
	return err == nil
}

// Cmp 比较两个分值（-1/0/1）。
func Cmp(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
