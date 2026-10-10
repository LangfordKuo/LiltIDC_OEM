package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/model"
)

// ---- /cart/all 原始响应结构（上游字段名照实录） ----

// rawProductGroup 是上游商品分组（id 用 FlexInt64 防御 PHP stringify 漂移）。
type rawProductGroup struct {
	ID       FlexInt64        `json:"id"`
	Name     string           `json:"name"`
	Products []rawProductItem `json:"products"`
}

// rawProductItem 是分组内的上游商品。
// 注意：pay_type 是「字符串化 JSON」，description 含 HTML 实体转义（&lt;li&gt; 等）。
type rawProductItem struct {
	ID          FlexInt64 `json:"id"`
	Type        string    `json:"type"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	PayType     string    `json:"pay_type"`
}

// ---- 解析后的对外结构 ----

// ParsedGroup 是解析后的商品分组。
type ParsedGroup struct {
	ID       int64           `json:"id"`
	Name     string          `json:"name"`
	Products []ParsedProduct `json:"products"`
}

// ParsedProduct 是解析后的上游商品。
//
// ⚠ Prices 在 client 层**恒为空**（批 2 第二个 P0 教训）：真实上游 /cart/all
// 的商品对象没有任何价格字段，pay_type 只含付费模式与周期时长定义
// （pay_hour_cycle:"720" 是小时数不是价格！）。成本价唯一真源是
// /api/product/prodetail 的 product_pricings（见 GetProductDetail），
// 由 service 组装层填充 Prices——保留此字段仅为输出结构稳定。
type ParsedProduct struct {
	ID          int64          `json:"id"`
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description"` // 已做 HTML 实体解码
	PayMode     string         `json:"pay_mode"`    // recurring / onetime（pay_type 标记解析）
	Prices      model.PriceMap `json:"prices"`      // client 层恒空；service 层从 prodetail 填充
}

// GetProducts 拉取上游商品分组树（GET /cart/all）并解析。
// 只提供 分组/商品 id/name/description/type/付费模式——不含价格（见 ParsedProduct 注释）。
func (c *Client) GetProducts(ctx context.Context) ([]ParsedGroup, error) {
	var raw struct {
		Products []rawProductGroup `json:"products"`
	}
	if err := c.do(ctx, "GET", "/cart/all", nil, &raw); err != nil {
		return nil, err
	}
	groups := make([]ParsedGroup, 0, len(raw.Products))
	for _, g := range raw.Products {
		pg := ParsedGroup{ID: int64(g.ID), Name: g.Name, Products: make([]ParsedProduct, 0, len(g.Products))}
		for _, p := range g.Products {
			pg.Products = append(pg.Products, ParsedProduct{
				ID:          int64(p.ID),
				Type:        p.Type,
				Name:        p.Name,
				Description: html.UnescapeString(p.Description), // &lt;li&gt; → <li>
				PayMode:     ParsePayMode(p.PayType),
				Prices:      model.PriceMap{}, // 恒空：价格从 prodetail 取，service 层填充
			})
		}
		groups = append(groups, pg)
	}
	return groups, nil
}

// ParsePayMode 从字符串化 JSON 的 pay_type 中解析付费模式标记。
//
// lyew.com 真实 pay_type 原文（pid=1）：
//
//	{"pay_type":"recurring","pay_hour_cycle":"720","pay_day_cycle":"30",
//	 "pay_ontrial_status":0,"pay_ontrial_cycle":"12",...}
//
// 只有顶层 "pay_type" 字段有意义（recurring=周期续费 / onetime=一次性）；
// pay_hour_cycle/pay_day_cycle 等是**周期时长定义**（小时数/天数），
// 绝不能当价格解析（批 2 第二个 P0 缺陷位点）。解析失败容错返回空串。
func ParsePayMode(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	var meta struct {
		PayType string `json:"pay_type"`
	}
	if err := json.Unmarshal([]byte(s), &meta); err != nil {
		return ""
	}
	return meta.PayType
}

// ---- /api/product/proinfo 库存/价格 ----

// ProductStock 是单个商品的库存信息。
type ProductStock struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	StockControl bool   `json:"stock_control"` // 上游是否启用库存控制
	Qty          int    `json:"qty"`           // 剩余库存（stock_control=false 时无意义）
}

// GetProductInfo 批量查询商品库存（GET /api/product/proinfo?pids[]=1&pids[]=2）。
// 上游 stock_control/qty 类型不定（bool/0/1/字符串数字均出现），宽松转换。
func (c *Client) GetProductInfo(ctx context.Context, pids []int64) ([]ProductStock, error) {
	if len(pids) == 0 {
		return nil, nil
	}
	// 数组参数用 pids[] 重复键形式，[] 保持未编码原样发送：
	// 真实上游（PHP $_GET）对 %5B%5D 与裸 [] 都识别，但部分简单实现不做 URL
	// 解码只认裸 []（联调 mock 实测），裸形式兼容性最好；pid 为 int64 数字，
	// 手动拼接无注入风险。
	parts := make([]string, 0, len(pids))
	for _, pid := range pids {
		parts = append(parts, "pids[]="+strconv.FormatInt(pid, 10))
	}
	path := "/api/product/proinfo?" + strings.Join(parts, "&")
	var raw struct {
		Info []struct {
			ID           FlexInt64 `json:"id"`
			Name         string    `json:"name"`
			StockControl any       `json:"stock_control"`
			Qty          any       `json:"qty"`
		} `json:"info"`
	}
	if err := c.do(ctx, "GET", path, nil, &raw); err != nil {
		return nil, err
	}
	out := make([]ProductStock, 0, len(raw.Info))
	for _, item := range raw.Info {
		out = append(out, ProductStock{
			ID:           int64(item.ID),
			Name:         item.Name,
			StockControl: anyToBool(item.StockControl),
			Qty:          anyToInt(item.Qty),
		})
	}
	return out, nil
}

// GetProductConfig 拉取商品可配置项（GET /cart/get_product_config?pid=N）。
// 结构未实测：原样透传 JSON，调用方需容错（失败记日志不阻断，见 service 层）。
func (c *Client) GetProductConfig(ctx context.Context, pid int64) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("pid", strconv.FormatInt(pid, 10))
	var raw json.RawMessage
	if err := c.do(ctx, "GET", "/cart/get_product_config", q, &raw); err != nil {
		return nil, fmt.Errorf("获取商品 %d 可配置项失败: %w", pid, err)
	}
	return raw, nil
}

// anyToBool 宽松转换布尔（true/1/"1"/"true" 均视为真）。
func anyToBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	case int:
		return t != 0
	case int64:
		return t != 0
	case json.Number:
		f, _ := t.Float64()
		return f != 0
	case string:
		b, _ := strconv.ParseBool(t)
		if b {
			return true
		}
		n, _ := strconv.Atoi(t)
		return n != 0
	}
	return false
}

// anyToInt 宽松转换整数（数字/字符串数字均可）。
func anyToInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case json.Number:
		n, _ := t.Int64()
		return int(n)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(t))
		return n
	case int:
		return t
	}
	return 0
}
