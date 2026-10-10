package service

// 商城前台商品接口（契约文档 13.1）：本地已上架商品的公开只读视图。
// 免鉴权（商城可匿名浏览，购买动作在下单接口走会员 JWT）；数据只出本地库，
// 不实时穿透上游（库存以 products.stock_qty 快照展示，下单时才做实时校验）。

import (
	"context"
	"strings"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/model"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/app"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/errcode"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/money"
)

// StoreService 处理商城前台商品展示。
type StoreService struct {
	app *app.App
}

// NewStore 构造商城服务。
func NewStore(a *app.App) *StoreService {
	return &StoreService{app: a}
}

// StoreProductView 是商城商品卡片视图。
type StoreProductView struct {
	ID           uint           `json:"id"`
	Name         string         `json:"name"`
	Description  string         `json:"description"` // 上游 HTML 原文（前端提取纯文本摘要/详情页 sanitize 渲染）
	Type         string         `json:"type"`
	GroupID      uint           `json:"group_id"`
	GroupName    string         `json:"group_name"`
	SalePrices   model.PriceMap `json:"sale_prices"`
	PriceFrom    string         `json:"price_from"` // 起价（各周期最低售价）
	StockQty     int            `json:"stock_qty"`
	StockControl bool           `json:"stock_control"`
	InStock      bool           `json:"in_stock"` // !stock_control 恒 true；否则 stock_qty>0
}

// StoreGroupView 是商城分组视图（含组内上架商品）。
type StoreGroupView struct {
	ID       uint               `json:"id"`
	Name     string             `json:"name"`
	Products []StoreProductView `json:"products"`
}

// StoreList 商城商品列表（按分组组织；只含上架商品与启用分组）。
func (s *StoreService) StoreList(ctx context.Context) ([]StoreGroupView, error) {
	db := s.app.GetDB()
	var groups []model.ProductGroup
	if err := db.WithContext(ctx).
		Where("status = ?", 1).
		Order("sort ASC, id ASC").Find(&groups).Error; err != nil {
		return nil, errcode.Internal("查询商品分组失败").WithErr(err)
	}
	var products []model.Product
	if err := db.WithContext(ctx).
		Where("status = ?", model.ProductStatusOn).
		Order("sort ASC, id ASC").Find(&products).Error; err != nil {
		return nil, errcode.Internal("查询商品失败").WithErr(err)
	}
	groupNames := make(map[uint]string, len(groups))
	enabled := make(map[uint]bool, len(groups))
	for _, g := range groups {
		groupNames[g.ID] = g.Name
		enabled[g.ID] = true
	}
	out := make([]StoreGroupView, 0, len(groups))
	index := make(map[uint]int, len(groups))
	for _, g := range groups {
		index[g.ID] = len(out)
		out = append(out, StoreGroupView{ID: g.ID, Name: g.Name, Products: []StoreProductView{}})
	}
	for i := range products {
		p := &products[i]
		gi, ok := index[p.GroupID]
		if !ok || !enabled[p.GroupID] {
			continue // 分组停用/缺失的商品不展示
		}
		out[gi].Products = append(out[gi].Products, toStoreProductView(p, groupNames[p.GroupID]))
	}
	// 去掉空分组（启用但无上架商品）
	filtered := out[:0]
	for _, g := range out {
		if len(g.Products) > 0 {
			filtered = append(filtered, g)
		}
	}
	return filtered, nil
}

// StoreProductDetail 是商城商品详情视图（描述为上游 HTML 原文，前端 sanitize 后渲染）。
type StoreProductDetail struct {
	StoreProductView
	Cycles []string `json:"cycles"` // 有售价的周期键（规范排序）
}

// StoreDetail 商城商品详情（仅上架商品；下架/不存在统一 product_not_found，防探测）。
func (s *StoreService) StoreDetail(ctx context.Context, id uint) (*StoreProductDetail, error) {
	db := s.app.GetDB()
	var p model.Product
	if err := db.WithContext(ctx).First(&p, id).Error; err != nil {
		return nil, errcode.New(404, errcode.CodeProductNotFound, "商品不存在")
	}
	if p.Status != model.ProductStatusOn {
		return nil, errcode.New(404, errcode.CodeProductNotFound, "商品不存在")
	}
	groupName := ""
	var group model.ProductGroup
	if err := db.WithContext(ctx).First(&group, p.GroupID).Error; err == nil {
		groupName = group.Name
	}
	sale, _ := model.ParsePriceMap(p.SalePrices)
	detail := &StoreProductDetail{
		StoreProductView: toStoreProductView(&p, groupName),
		Cycles:           sale.SortedCycles(),
	}
	return detail, nil
}

// toStoreProductView 模型 → 商城卡片视图（起价 = 各周期最低售价；金额比较走整数分）。
func toStoreProductView(p *model.Product, groupName string) StoreProductView {
	sale, err := model.ParsePriceMap(p.SalePrices)
	if err != nil {
		sale = model.PriceMap{}
	}
	inStock := true
	if p.StockControl {
		inStock = p.StockQty > 0
	}
	return StoreProductView{
		ID: p.ID, Name: p.Name, Description: p.Description, Type: p.Type,
		GroupID: p.GroupID, GroupName: groupName,
		SalePrices: sale, PriceFrom: minPrice(sale),
		StockQty: p.StockQty, StockControl: p.StockControl, InStock: inStock,
	}
}

// minPrice 返回价格映射中的最低售价文本（整数分比较，非法值跳过；空映射返回 ""）。
func minPrice(prices model.PriceMap) string {
	minCents := int64(-1)
	minText := ""
	for _, text := range prices {
		cents, err := money.ParseCents(strings.TrimSpace(text))
		if err != nil || cents < 0 {
			continue
		}
		if minCents < 0 || cents < minCents {
			minCents, minText = cents, text
		}
	}
	return minText
}
