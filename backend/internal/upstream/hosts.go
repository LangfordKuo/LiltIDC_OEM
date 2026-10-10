package upstream

import (
	"context"
	"net/url"
	"strconv"
)

// DomainStatusInfo 是上游主机状态字典项（/host/list 的 domainstatus）。
type DomainStatusInfo struct {
	Name  string `json:"name"`  // 显示名，如 "已激活"
	Color string `json:"color"` // 展示颜色
}

// HostList 是上游主机列表响应（GET /host/list）。
// list 内字段不固定（上游版本差异），按 map 透传给前端做只读展示。
//
// ⚠ 分页字段用 FlexInt：上游 PHP 对 query 参数原样回显，带 ?page=1&limit=10
// 调用时 page/limit 为字符串、不带参数时为 int（lyew.com 实测的类型漂移，
// 见 flexint.go）。Marshal 输出仍为 JSON 数字，前端契约不变。
type HostList struct {
	Page         FlexInt                     `json:"page"`
	Limit        FlexInt                     `json:"limit"`
	Sum          FlexInt                     `json:"sum"`
	List         []map[string]any            `json:"list"`
	DomainStatus map[string]DomainStatusInfo `json:"domainstatus"`
}

// GetHosts 拉取上游主机列表（对账用只读数据）。
func (c *Client) GetHosts(ctx context.Context, page, limit int) (*HostList, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	q := url.Values{}
	q.Set("page", strconv.Itoa(page))
	q.Set("limit", strconv.Itoa(limit))
	var list HostList
	if err := c.do(ctx, "GET", "/host/list", q, &list); err != nil {
		return nil, err
	}
	return &list, nil
}
