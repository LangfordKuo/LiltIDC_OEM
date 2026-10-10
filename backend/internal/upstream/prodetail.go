package upstream

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/model"
)

// CyclePriceKeys 是上游 product_pricings 的周期价格键白名单（lyew.com 实测全量）。
// 本地规范键与上游键**完全一致**（含 semiannually 无下划线形态），无需映射，见 ADR-022。
var CyclePriceKeys = []string{
	"onetime", "hour", "day", "ontrial",
	"monthly", "quarterly", "semiannually", "annually",
	"biennially", "triennially", "fourly", "fively",
	"sixly", "sevenly", "eightly", "ninely", "tenly",
}

// SetupFeeKeys 是上游 product_pricings 的安装费键白名单（lyew.com 实测全量）。
// 批 2 解析进 ProductDetail.SetupFees 但**不导入**本地售价体系（结构预留，批 3+ 按需启用）。
var SetupFeeKeys = []string{
	"osetupfee", "hsetupfee", "dsetupfee", "ontrialfee",
	"msetupfee", "qsetupfee", "ssetupfee", "asetupfee",
	"bsetupfee", "tsetupfee", "foursetupfee", "fivesetupfee",
	"sixsetupfee", "sevensetupfee", "eightsetupfee", "ninesetupfee", "tensetupfee",
}

// ProductDetail 是 /api/product/prodetail 单商品的解析结果。
//
// ⚠ 成本价唯一真源（批 2 第二个 P0 教训）：真实上游 /cart/all 的 pay_type 里
// **没有价格**（只有付费模式与周期时长定义），价格在本端点 data.detail[pid].product_pricings[0]。
type ProductDetail struct {
	ID           int64          `json:"id"`
	Name         string         `json:"name"`
	PayMode      string         `json:"pay_mode"` // recurring / onetime（pay_type 解析）
	CostPrices   model.PriceMap `json:"cost_prices"`
	SetupFees    model.PriceMap `json:"setup_fees"`
	CurrencyCode string         `json:"currency_code"` // product_pricings[0].code，如 CNY
	StockControl bool           `json:"stock_control"`
	Qty          int            `json:"qty"`
}

// GetProductDetail 批量查询商品详情与价格（GET /api/product/prodetail?pids[]=1&pids[]=2）。
//
// 响应 data.detail 为 {pid字符串: 商品对象} map；容错（lyew.com 实测）：
//   - 不存在的 pid：detail 中键整个缺失，或 product_pricings 为 null → 空价格，不报错；
//   - product_pricings[0] 混杂非价格键（id/type/currency/relid/code）→ 白名单提取；
//   - 价格为 "-1.00"/"-1" 表示该周期停售 → 过滤；"0.00" 免费保留。
func (c *Client) GetProductDetail(ctx context.Context, pids []int64) (map[int64]*ProductDetail, error) {
	out := make(map[int64]*ProductDetail, len(pids))
	if len(pids) == 0 {
		return out, nil
	}
	// pids[] 重复键、未编码原样发送（与 GetProductInfo 同一取舍，见该处注释）
	parts := make([]string, 0, len(pids))
	for _, pid := range pids {
		parts = append(parts, "pids[]="+strconv.FormatInt(pid, 10))
	}
	path := "/api/product/prodetail?" + strings.Join(parts, "&")

	var raw struct {
		Detail map[string]struct {
			ID              FlexInt64        `json:"id"` // 防御 PHP stringify 漂移（同 flexint.go）
			Name            string           `json:"name"`
			PayType         string           `json:"pay_type"`
			StockControl    any              `json:"stock_control"`
			Qty             any              `json:"qty"`
			ProductPricings []map[string]any `json:"product_pricings"` // null → nil slice，天然容错
		} `json:"detail"`
	}
	if err := c.do(ctx, "GET", path, nil, &raw); err != nil {
		return nil, err
	}

	for key, item := range raw.Detail {
		pid, err := strconv.ParseInt(key, 10, 64)
		if err != nil {
			continue // 非数字键容错跳过
		}
		d := &ProductDetail{
			ID:           int64(item.ID),
			Name:         item.Name,
			PayMode:      ParsePayMode(item.PayType),
			CostPrices:   model.PriceMap{},
			SetupFees:    model.PriceMap{},
			StockControl: anyToBool(item.StockControl),
			Qty:          anyToInt(item.Qty),
		}
		if d.ID == 0 {
			d.ID = pid // id 字段缺失时以 detail 键兜底
		}
		if len(item.ProductPricings) > 0 && item.ProductPricings[0] != nil {
			pricing := item.ProductPricings[0]
			if code, ok := pricing["code"].(string); ok {
				d.CurrencyCode = code
			}
			// 白名单提取：周期价格与安装费分离，非价格键（id/type/currency/relid/code）天然排除
			for _, k := range CyclePriceKeys {
				if v, exists := pricing[k]; exists {
					if price := normalizePrice(v); price != "" {
						d.CostPrices[k] = price
					}
				}
			}
			for _, k := range SetupFeeKeys {
				if v, exists := pricing[k]; exists {
					if fee := normalizePrice(v); fee != "" {
						d.SetupFees[k] = fee
					}
				}
			}
		}
		out[pid] = d
	}
	return out, nil
}

// normalizePrice 把 product_pricings 的价格值宽松转为文本：
// 字符串/数字均接受；**负数（-1.00 停售约定）与非数字返回空串（过滤）**；"0.00" 免费保留。
func normalizePrice(v any) string {
	var s string
	switch t := v.(type) {
	case string:
		s = strings.TrimSpace(t)
	case float64:
		s = strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		s = t.String()
	default:
		return ""
	}
	if s == "" {
		return ""
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 {
		return ""
	}
	return s
}
