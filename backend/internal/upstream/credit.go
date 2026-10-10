package upstream

import "context"

// CurrencyInfo 是上游货币信息。
type CurrencyInfo struct {
	ID     FlexInt `json:"id"`     // 货币 ID（批 3-β 下单链路 currencyid 参数用；lyew.com 实测 CNY=1）
	Code   string  `json:"code"`   // 如 CNY
	Prefix string  `json:"prefix"` // 如 ¥
	Suffix string  `json:"suffix"` // 如 元
}

// Credit 是上游账户余额（GET /cart/credit）。
type Credit struct {
	Credit   string       `json:"credit"` // 余额文本，如 "57664.50"（字符串金额，避免浮点误差）
	Currency CurrencyInfo `json:"currency"`
}

// GetCredit 查询上游账户余额。
func (c *Client) GetCredit(ctx context.Context) (*Credit, error) {
	var credit Credit
	if err := c.do(ctx, "GET", "/cart/credit", nil, &credit); err != nil {
		return nil, err
	}
	return &credit, nil
}
