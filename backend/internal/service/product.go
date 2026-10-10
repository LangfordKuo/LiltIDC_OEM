package service

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/model"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/app"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/errcode"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/upstream"
)

// ProductService 处理本地商品管理业务（契约文档第 10 章）。
type ProductService struct {
	app *app.App
}

// NewProduct 构造商品服务。
func NewProduct(a *app.App) *ProductService {
	return &ProductService{app: a}
}

// ProductView 是商品列表/详情的对外视图（价格已解析为对象）。
type ProductView struct {
	ID           uint           `json:"id"`
	GroupID      uint           `json:"group_id"`
	GroupName    string         `json:"group_name"`
	UpstreamPid  int64          `json:"upstream_pid"`
	Name         string         `json:"name"`
	Type         string         `json:"type"`
	CostPrices   model.PriceMap `json:"cost_prices"`
	SalePrices   model.PriceMap `json:"sale_prices"`
	StockQty     int            `json:"stock_qty"`
	StockControl bool           `json:"stock_control"`
	Status       int8           `json:"status"`
	Sort         int            `json:"sort"`
	UpdatedAt    string         `json:"updated_at"`
}

// ProductListView 是商品列表响应。
type ProductListView struct {
	List  []ProductView `json:"list"`
	Total int64         `json:"total"`
}

// GroupView 是分组对外视图。
type GroupView struct {
	ID     uint   `json:"id"`
	Name   string `json:"name"`
	Sort   int    `json:"sort"`
	Status int8   `json:"status"`
}

// List 返回本地商品列表（分页 + 可选分组筛选），附分组名。
func (s *ProductService) List(ctx context.Context, groupID uint, page, pageSize int) (*ProductListView, error) {
	db := s.app.GetDB()
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	q := db.WithContext(ctx).Model(&model.Product{})
	if groupID > 0 {
		q = q.Where("group_id = ?", groupID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, errcode.Internal("统计商品失败").WithErr(err)
	}
	var rows []model.Product
	if err := q.Order("sort ASC, id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&rows).Error; err != nil {
		return nil, errcode.Internal("查询商品失败").WithErr(err)
	}
	// 分组名索引
	groupNames := make(map[uint]string)
	var groups []model.ProductGroup
	if err := db.WithContext(ctx).Find(&groups).Error; err == nil {
		for _, g := range groups {
			groupNames[g.ID] = g.Name
		}
	}
	list := make([]ProductView, 0, len(rows))
	for i := range rows {
		list = append(list, toProductView(&rows[i], groupNames[rows[i].GroupID]))
	}
	return &ProductListView{List: list, Total: total}, nil
}

// ListGroups 返回全部分组（前端筛选/编辑下拉用）。
func (s *ProductService) ListGroups(ctx context.Context) ([]GroupView, error) {
	db := s.app.GetDB()
	var rows []model.ProductGroup
	if err := db.WithContext(ctx).Order("sort ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, errcode.Internal("查询分组失败").WithErr(err)
	}
	out := make([]GroupView, 0, len(rows))
	for _, g := range rows {
		out = append(out, GroupView{ID: g.ID, Name: g.Name, Sort: g.Sort, Status: g.Status})
	}
	return out, nil
}

// UpdateProductRequest 是编辑商品请求（字段均可选，缺省不修改）。
type UpdateProductRequest struct {
	Name       *string         `json:"name"`
	GroupID    *uint           `json:"group_id"`
	SalePrices *model.PriceMap `json:"sale_prices"`
	Sort       *int            `json:"sort"`
}

// Update 编辑本地商品（名称/分组/售价/排序）。
func (s *ProductService) Update(ctx context.Context, id uint, req *UpdateProductRequest) (*ProductView, error) {
	db := s.app.GetDB()
	row, err := s.mustFind(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if req.Name == nil && req.GroupID == nil && req.SalePrices == nil && req.Sort == nil {
		return nil, errcode.InvalidParam("请求体为空：至少提交 name/group_id/sale_prices/sort 之一")
	}
	updates := map[string]any{}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" || len([]rune(name)) > 255 {
			return nil, errcode.InvalidParam("name 必填且不超过 255 字符")
		}
		updates["name"] = name
	}
	if req.GroupID != nil {
		var count int64
		if err := db.WithContext(ctx).Model(&model.ProductGroup{}).
			Where("id = ?", *req.GroupID).Count(&count).Error; err != nil || count == 0 {
			return nil, errcode.InvalidParam("分组 %d 不存在", *req.GroupID)
		}
		updates["group_id"] = *req.GroupID
	}
	if req.SalePrices != nil {
		if len(*req.SalePrices) == 0 {
			return nil, errcode.InvalidParam("sale_prices 不能为空")
		}
		for cycle, price := range *req.SalePrices {
			f, e := strconv.ParseFloat(strings.TrimSpace(price), 64)
			if e != nil || f <= 0 {
				return nil, errcode.InvalidParam("周期 %s：售价必须是大于 0 的数字（得到 %q）", cycle, price)
			}
		}
		updates["sale_prices"] = req.SalePrices.Marshal()
	}
	if req.Sort != nil {
		updates["sort"] = *req.Sort
	}
	if err := db.WithContext(ctx).Model(row).Updates(updates).Error; err != nil {
		return nil, errcode.Internal("更新商品失败").WithErr(err)
	}
	row, err = s.mustFind(ctx, db, id)
	if err != nil {
		return nil, err
	}
	groupName := ""
	var group model.ProductGroup
	if e := db.WithContext(ctx).First(&group, row.GroupID).Error; e == nil {
		groupName = group.Name
	}
	view := toProductView(row, groupName)
	return &view, nil
}

// UpdateStatusRequest 是上下架请求。
type UpdateStatusRequest struct {
	Status *int8 `json:"status"`
}

// UpdateStatus 上下架商品。
func (s *ProductService) UpdateStatus(ctx context.Context, id uint, req *UpdateStatusRequest) (*ProductView, error) {
	if req.Status == nil || (*req.Status != model.ProductStatusOn && *req.Status != model.ProductStatusOff) {
		return nil, errcode.InvalidParam("status 必须是 0（下架）或 1（上架）")
	}
	db := s.app.GetDB()
	row, err := s.mustFind(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if err := db.WithContext(ctx).Model(row).Update("status", *req.Status).Error; err != nil {
		return nil, errcode.Internal("更新状态失败").WithErr(err)
	}
	row.Status = *req.Status
	view := toProductView(row, "")
	return &view, nil
}

// SyncStockResult 是库存同步结果。
type SyncStockResult struct {
	Synced int `json:"synced"` // 成功更新库存的商品数
	Total  int `json:"total"`  // 参与同步的商品数（有 upstream_pid 的全部本地商品）
}

// SyncStock 手动同步库存：取全部本地商品的 upstream_pid，
// 分批（每批 ≤50）调 proinfo 更新 stock_qty/stock_control。
// 上游不可用时整体报错；单批失败跳过并记日志（尽力而为）。
func (s *ProductService) SyncStock(ctx context.Context) (*SyncStockResult, error) {
	db := s.app.GetDB()
	var rows []model.Product
	if err := db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, errcode.Internal("查询商品失败").WithErr(err)
	}
	if len(rows) == 0 {
		return &SyncStockResult{}, nil
	}
	pids := make([]int64, 0, len(rows))
	byPid := make(map[int64]*model.Product, len(rows))
	for i := range rows {
		pids = append(pids, rows[i].UpstreamPid)
		byPid[rows[i].UpstreamPid] = &rows[i]
	}

	mgr := s.app.GetUpstream()
	if mgr == nil {
		return nil, errcode.Internal("上游模块未就绪")
	}
	client, err := mgr.Client(ctx)
	if err != nil {
		return nil, mapUpstreamClientError(err)
	}

	result := &SyncStockResult{Total: len(rows)}
	const batchSize = 50
	for start := 0; start < len(pids); start += batchSize {
		end := start + batchSize
		if end > len(pids) {
			end = len(pids)
		}
		infos, e := client.GetProductInfo(ctx, pids[start:end])
		if e != nil {
			slog.Warn("同步库存批次失败（跳过该批）", "range", []int{start, end}, "err", e)
			continue
		}
		for _, info := range infos {
			p, ok := byPid[info.ID]
			if !ok {
				continue
			}
			if e := db.WithContext(ctx).Model(p).Updates(map[string]any{
				"stock_qty":     info.Qty,
				"stock_control": info.StockControl,
			}).Error; e != nil {
				slog.Warn("更新商品库存失败", "product_id", p.ID, "err", e)
				continue
			}
			result.Synced++
		}
	}
	return result, nil
}

// mustFind 按 ID 查商品，不存在返回 product_not_found。
func (s *ProductService) mustFind(ctx context.Context, db *gorm.DB, id uint) (*model.Product, error) {
	var row model.Product
	err := db.WithContext(ctx).First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.New(404, errcode.CodeProductNotFound, "商品不存在")
	}
	if err != nil {
		return nil, errcode.Internal("查询商品失败").WithErr(err)
	}
	return &row, nil
}

// mapUpstreamClientError 映射上游客户端错误（product 服务复用）。
func mapUpstreamClientError(err error) error {
	switch {
	case errors.Is(err, upstream.ErrNotConfigured):
		return errcode.New(400, errcode.CodeUpstreamNotConfigured, err.Error())
	case errors.Is(err, upstream.ErrAuthFailed):
		return errcode.New(502, errcode.CodeUpstreamAuthFailed, readableUpstreamError(err))
	default:
		return errcode.New(502, errcode.CodeUpstreamRequestFailed, readableUpstreamError(err))
	}
}

// toProductView 转换模型为对外视图（价格 JSON 解析失败时给空映射，不阻断列表）。
func toProductView(p *model.Product, groupName string) ProductView {
	cost, err := model.ParsePriceMap(p.CostPrices)
	if err != nil {
		cost = model.PriceMap{}
	}
	sale, err := model.ParsePriceMap(p.SalePrices)
	if err != nil {
		sale = model.PriceMap{}
	}
	return ProductView{
		ID: p.ID, GroupID: p.GroupID, GroupName: groupName,
		UpstreamPid: p.UpstreamPid, Name: p.Name, Type: p.Type,
		CostPrices: cost, SalePrices: sale,
		StockQty: p.StockQty, StockControl: p.StockControl,
		Status: p.Status, Sort: p.Sort,
		UpdatedAt: p.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}
