package upstream

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// FlexInt / FlexInt64 是宽松整数类型：同时接受 JSON 数字、数字字符串与 null/空串。
//
// 背景（批 2 小修，lyew.com 实测坐实）：上游 PHP 对 query 参数**原样回显**，
// /host/list 的分页字段存在 int/string 类型漂移——不带参数调用 data.page=1（int），
// 带 ?page=1&limit=10 调用则 data.page="1"、data.limit="10"（string），
// 而 sum/max_page 恒为 int。解析端一律用宽松类型接收；
// Marshal 输出仍为 JSON 数字，上层 service/handler 与前端契约不变（仍是 int）。
//
// 防御性同用位点：cart/all 的分组与商品 id、proinfo/prodetail 的商品 id
// （同为 PHP 侧可能 stringify 的数字字段）。
type FlexInt int

// UnmarshalJSON 实现宽松解析。
func (f *FlexInt) UnmarshalJSON(data []byte) error {
	n, err := parseFlexInt(data)
	if err != nil {
		return err
	}
	*f = FlexInt(n)
	return nil
}

// FlexInt64 是 64 位版本（商品/分组 id 用）。
type FlexInt64 int64

// UnmarshalJSON 实现宽松解析。
func (f *FlexInt64) UnmarshalJSON(data []byte) error {
	n, err := parseFlexInt(data)
	if err != nil {
		return err
	}
	*f = FlexInt64(n)
	return nil
}

// parseFlexInt 把 JSON 标量解析为 int64：
// 数字（含 1.0 形态浮点）直接取值；数字字符串按十进制解析；
// null 与空串（PHP 未传参回显 ""）容错为 0；其余报错。
func parseFlexInt(data []byte) (int64, error) {
	s := strings.TrimSpace(string(data))
	if s == "null" {
		return 0, nil
	}
	// 整数形态
	var n int64
	if err := json.Unmarshal(data, &n); err == nil {
		return n, nil
	}
	// 浮点形态（10.0 → 10）
	var f float64
	if err := json.Unmarshal(data, &f); err == nil {
		return int64(f), nil
	}
	// 字符串形态
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		t := strings.TrimSpace(str)
		if t == "" {
			return 0, nil
		}
		v, err := strconv.ParseInt(t, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("无法把 %q 解析为整数", str)
		}
		return v, nil
	}
	return 0, fmt.Errorf("无法把 %s 解析为整数", s)
}
