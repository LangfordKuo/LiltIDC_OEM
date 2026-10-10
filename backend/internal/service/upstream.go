package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/model"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/app"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/errcode"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/upstream"
)

// UpstreamService 处理上游魔方财务对接业务（契约文档第 9 章）。
type UpstreamService struct {
	app *app.App
}

// NewUpstream 构造上游服务。
func NewUpstream(a *app.App) *UpstreamService {
	return &UpstreamService{app: a}
}

// HostSyncPath 是上游推送接收端路径（展示给管理员登记到上游）。
const HostSyncPath = "/api/upstream/host/sync"

// UpstreamConfigView 是上游配置对外视图（api_key 不回显明文，只回显是否已设置）。
type UpstreamConfigView struct {
	Hostname        string `json:"hostname"`
	Username        string `json:"username"`
	APIKeySet       bool   `json:"api_key_set"`
	DownstreamToken string `json:"downstream_token"`
	SyncPath        string `json:"sync_path"` // 回调路径；前端拼 window.location.origin 展示完整地址
	SiteURL         string `json:"site_url"`  // 本站对外地址（批 3-β：setdownstream 登记的 downstream_url）
}

// GetConfig 读取上游配置。downstream_token 为空时自动生成并落库
// （幂等的懒初始化：token 必须存在才能登记到上游，免去管理员先点一次生成）。
func (s *UpstreamService) GetConfig(ctx context.Context) (*UpstreamConfigView, error) {
	db := s.requireDB()
	m, err := model.GetSettingsMap(db.WithContext(ctx),
		model.SettingUpstreamHostname, model.SettingUpstreamUsername,
		model.SettingUpstreamAPIKey, model.SettingDownstreamToken, model.SettingSiteURL)
	if err != nil {
		return nil, errcode.Internal("读取上游配置失败").WithErr(err)
	}
	token := m[model.SettingDownstreamToken]
	if token == "" {
		token, err = s.resetDownstreamTokenLocked(ctx, db)
		if err != nil {
			return nil, err
		}
	}
	return &UpstreamConfigView{
		Hostname:        m[model.SettingUpstreamHostname],
		Username:        m[model.SettingUpstreamUsername],
		APIKeySet:       m[model.SettingUpstreamAPIKey] != "",
		DownstreamToken: token,
		SyncPath:        HostSyncPath,
		SiteURL:         m[model.SettingSiteURL],
	}, nil
}

// SaveUpstreamConfigRequest 是保存上游配置请求（api_key 留空 = 不修改，范式同 notify）。
type SaveUpstreamConfigRequest struct {
	Hostname string `json:"hostname"`
	Username string `json:"username"`
	APIKey   string `json:"api_key"`
	// SiteURL 是本站对外地址（批 3-β；下游登记用。留空 = 不修改；
	// 显式提交空串无法区分「不修改」，如需清空改回退默认请直接改 settings）。
	SiteURL *string `json:"site_url"`
}

// SaveConfig 保存上游配置并使缓存的上游会话失效（新凭据下次请求即生效）。
func (s *UpstreamService) SaveConfig(ctx context.Context, req *SaveUpstreamConfigRequest) error {
	db := s.requireDB()
	hostname := strings.TrimRight(strings.TrimSpace(req.Hostname), "/")
	if hostname != "" {
		u, err := url.Parse(hostname)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errcode.InvalidParam("上游地址必须是完整 URL（http:// 或 https:// 开头），如 https://lyew.com")
		}
	}
	username := strings.TrimSpace(req.Username)
	if strings.ContainsAny(username, " \t") {
		return errcode.InvalidParam("上游账号不能包含空格（手机号请勿加区号）")
	}
	kvs := map[string]string{
		model.SettingUpstreamHostname: hostname,
		model.SettingUpstreamUsername: username,
	}
	if key := strings.TrimSpace(req.APIKey); key != "" {
		kvs[model.SettingUpstreamAPIKey] = key
	}
	if req.SiteURL != nil {
		// 本站地址（下游登记的 downstream_url）：须为完整 http/https URL，存储去尾斜杠
		siteURL := strings.TrimRight(strings.TrimSpace(*req.SiteURL), "/")
		if siteURL != "" {
			su, err := url.Parse(siteURL)
			if err != nil || (su.Scheme != "http" && su.Scheme != "https") || su.Host == "" {
				return errcode.InvalidParam("本站地址必须是完整 URL（http:// 或 https:// 开头），如 https://idc.example.com")
			}
		}
		kvs[model.SettingSiteURL] = siteURL
	}
	if err := model.SetSettingsMap(db.WithContext(ctx), kvs); err != nil {
		return errcode.Internal("保存上游配置失败").WithErr(err)
	}
	// 凭据可能变化：作废缓存的 Client（含旧 jwt），下次请求按新凭据重建
	if mgr := s.app.GetUpstream(); mgr != nil {
		mgr.Invalidate()
	}
	slog.Info("上游配置已更新", "hostname", hostname, "username", username)
	return nil
}

// ResetDownstreamToken 重置下游密钥（旧 token 立即失效，需重新登记到上游）。
func (s *UpstreamService) ResetDownstreamToken(ctx context.Context) (string, error) {
	return s.resetDownstreamTokenLocked(ctx, s.requireDB())
}

func (s *UpstreamService) resetDownstreamTokenLocked(ctx context.Context, db *gorm.DB) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", errcode.Internal("生成下游密钥失败").WithErr(err)
	}
	token := hex.EncodeToString(buf)
	if err := model.SetSetting(db.WithContext(ctx), model.SettingDownstreamToken, token); err != nil {
		return "", errcode.Internal("保存下游密钥失败").WithErr(err)
	}
	slog.Info("下游密钥已生成/重置")
	return token, nil
}

// UpstreamTestResult 是连接测试结果（契约文档 9.2：失败也返回 200，connected=false + msg）。
type UpstreamTestResult struct {
	Connected bool                   `json:"connected"`
	Credit    string                 `json:"credit"`
	Currency  *upstream.CurrencyInfo `json:"currency"`
	Msg       string                 `json:"msg"`
}

// TestConnection 真实调用上游：登录 + /cart/credit（只读），成功时带回余额。
func (s *UpstreamService) TestConnection(ctx context.Context) (*UpstreamTestResult, error) {
	client, err := s.client(ctx)
	if err != nil {
		if errors.Is(err, upstream.ErrNotConfigured) {
			return &UpstreamTestResult{Connected: false, Msg: err.Error()}, nil
		}
		return nil, err
	}
	credit, err := client.GetCredit(ctx) // GetCredit 内部会自动登录
	if err != nil {
		return &UpstreamTestResult{Connected: false, Msg: readableUpstreamError(err)}, nil
	}
	msg := "连接成功"
	if strings.TrimSpace(credit.Credit) == "" {
		// 连通但拿不到余额：如实标注部分成功，避免「连接成功却数据全空」的误导
		//（批 2 P0 教训：data 信封未解包时正是这种误导性成功）
		msg = "已连接，但上游未返回余额数据（credit 为空），请检查上游响应形状或账号状态"
	}
	return &UpstreamTestResult{
		Connected: true,
		Credit:    credit.Credit,
		Currency:  &credit.Currency,
		Msg:       msg,
	}, nil
}

// CreditView 是余额查询响应。
type CreditView struct {
	Credit   string                `json:"credit"`
	Currency upstream.CurrencyInfo `json:"currency"`
}

// GetCredit 查询上游余额。
func (s *UpstreamService) GetCredit(ctx context.Context) (*CreditView, error) {
	client, err := s.client(ctx)
	if err != nil {
		return nil, s.mapUpstreamError(err)
	}
	credit, err := client.GetCredit(ctx)
	if err != nil {
		return nil, s.mapUpstreamError(err)
	}
	return &CreditView{Credit: credit.Credit, Currency: credit.Currency}, nil
}

// ---- 商品拉取 / 库存 / 导入 ----

// UpstreamProductView 是上游商品（附本地导入状态）。
type UpstreamProductView struct {
	ID              int64          `json:"id"`
	Type            string         `json:"type"`
	Name            string         `json:"name"`
	Description     string         `json:"description"`
	PayMode         string         `json:"pay_mode"` // recurring / onetime
	Prices          model.PriceMap `json:"prices"`   // 上游成本价（真源 prodetail.product_pricings，-1 停售已过滤）
	Imported        bool           `json:"imported"`
	LocalProductID  uint           `json:"local_product_id"`
	LocalName       string         `json:"local_name"`
	LocalSalePrices model.PriceMap `json:"local_sale_prices"`
	LocalStatus     int8           `json:"local_status"`
}

// UpstreamGroupView 是上游商品分组。
type UpstreamGroupView struct {
	ID       int64                 `json:"id"`
	Name     string                `json:"name"`
	Products []UpstreamProductView `json:"products"`
}

// GetProducts 拉取上游商品树并附本地导入状态（已导入的标出映射与售价）。
//
// 成本价真源是 /api/product/prodetail（批 2 第二个 P0 教训：真实上游
// /cart/all 的 pay_type 里没有任何价格，只有付费模式与周期时长定义）。
func (s *UpstreamService) GetProducts(ctx context.Context) ([]UpstreamGroupView, error) {
	db := s.requireDB()
	client, err := s.client(ctx)
	if err != nil {
		return nil, s.mapUpstreamError(err)
	}
	groups, err := client.GetProducts(ctx)
	if err != nil {
		return nil, s.mapUpstreamError(err)
	}

	// 批量拉 prodetail 填充成本价（价格缺失的商品展示为空，不阻断整树）
	pids := make([]int64, 0, 64)
	for _, g := range groups {
		for _, p := range g.Products {
			pids = append(pids, p.ID)
		}
	}
	details, err := fetchProductDetails(ctx, client, pids)
	if err != nil {
		return nil, s.mapUpstreamError(err)
	}

	// 本地映射索引：upstream_pid → Product
	var locals []model.Product
	if err := db.WithContext(ctx).Find(&locals).Error; err != nil {
		return nil, errcode.Internal("查询本地商品失败").WithErr(err)
	}
	byPid := make(map[int64]*model.Product, len(locals))
	for i := range locals {
		byPid[locals[i].UpstreamPid] = &locals[i]
	}

	out := make([]UpstreamGroupView, 0, len(groups))
	for _, g := range groups {
		gv := UpstreamGroupView{ID: g.ID, Name: g.Name, Products: make([]UpstreamProductView, 0, len(g.Products))}
		for _, p := range g.Products {
			prices := model.PriceMap{}
			if d, ok := details[p.ID]; ok && d.CostPrices != nil {
				prices = d.CostPrices
			}
			v := UpstreamProductView{
				ID: p.ID, Type: p.Type, Name: p.Name,
				Description: p.Description, PayMode: p.PayMode, Prices: prices,
			}
			if local, ok := byPid[p.ID]; ok {
				v.Imported = true
				v.LocalProductID = local.ID
				v.LocalName = local.Name
				v.LocalStatus = local.Status
				if sp, e := model.ParsePriceMap(local.SalePrices); e == nil {
					v.LocalSalePrices = sp
				}
			}
			gv.Products = append(gv.Products, v)
		}
		out = append(out, gv)
	}
	return out, nil
}

// fetchProductDetails 分批（≤50/批，防 URL 过长）拉取 prodetail 并合并结果。
// 成本价与库存的同源数据（批 2 第二个 P0 修复后的唯一价格真源）。
func fetchProductDetails(ctx context.Context, client *upstream.Client, pids []int64) (map[int64]*upstream.ProductDetail, error) {
	merged := make(map[int64]*upstream.ProductDetail, len(pids))
	const batchSize = 50
	for start := 0; start < len(pids); start += batchSize {
		end := min(start+batchSize, len(pids))
		batch, err := client.GetProductDetail(ctx, pids[start:end])
		if err != nil {
			return nil, err
		}
		for k, v := range batch {
			merged[k] = v
		}
	}
	return merged, nil
}

// StockView 是库存查询结果项。
type StockView struct {
	UpstreamPid  int64  `json:"upstream_pid"`
	Name         string `json:"name"`
	StockControl bool   `json:"stock_control"`
	Qty          int    `json:"qty"`
}

// GetStock 批量查询上游商品库存（proinfo）。
func (s *UpstreamService) GetStock(ctx context.Context, pids []int64) ([]StockView, error) {
	if len(pids) == 0 {
		return nil, errcode.InvalidParam("pids 不能为空")
	}
	if len(pids) > 200 {
		return nil, errcode.InvalidParam("单次最多查询 200 个商品")
	}
	client, err := s.client(ctx)
	if err != nil {
		return nil, s.mapUpstreamError(err)
	}
	infos, err := client.GetProductInfo(ctx, pids)
	if err != nil {
		return nil, s.mapUpstreamError(err)
	}
	out := make([]StockView, 0, len(infos))
	for _, info := range infos {
		out = append(out, StockView{
			UpstreamPid: info.ID, Name: info.Name,
			StockControl: info.StockControl, Qty: info.Qty,
		})
	}
	return out, nil
}

// ImportItem 是单条导入项（契约文档 9.4）。
type ImportItem struct {
	UpstreamPid int64          `json:"upstream_pid"`
	Name        string         `json:"name"`        // 留空默认上游名
	GroupID     uint           `json:"group_id"`    // >0 选已有分组
	GroupName   string         `json:"group_name"`  // GroupID=0 时按名称新建/复用
	SalePrices  model.PriceMap `json:"sale_prices"` // 周期 → 自定义售价（必填，每项 > 0）
	Status      *int8          `json:"status"`      // 缺省 1=上架
}

// ImportRequest 是批量导入请求。
type ImportRequest struct {
	Items []ImportItem `json:"items"`
}

// ImportResult 是导入结果。
type ImportResult struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
}

// ImportProducts 批量导入上游商品（幂等：upstream_pid 已存在则更新映射与售价）。
// 成本价/描述/类型从上游实时拉取（不信任前端提交），库存顺带经 proinfo 回填。
func (s *UpstreamService) ImportProducts(ctx context.Context, req *ImportRequest) (*ImportResult, error) {
	db := s.requireDB()
	if len(req.Items) == 0 {
		return nil, errcode.InvalidParam("items 不能为空")
	}
	if len(req.Items) > 200 {
		return nil, errcode.InvalidParam("单次最多导入 200 个商品")
	}

	// 1. 参数与售价校验（先全部校验再落库，避免半成功）
	pids := make([]int64, 0, len(req.Items))
	pidSeen := make(map[int64]bool, len(req.Items))
	for i := range req.Items {
		item := &req.Items[i]
		if item.UpstreamPid <= 0 {
			return nil, errcode.InvalidParam("第 %d 条：upstream_pid 必须为正整数", i+1)
		}
		if pidSeen[item.UpstreamPid] {
			return nil, errcode.InvalidParam("第 %d 条：upstream_pid=%d 重复提交", i+1, item.UpstreamPid)
		}
		pidSeen[item.UpstreamPid] = true
		pids = append(pids, item.UpstreamPid)
		if len(item.SalePrices) == 0 {
			return nil, errcode.InvalidParam("商品 %d：sale_prices 不能为空（至少一个计费周期）", item.UpstreamPid)
		}
		for cycle, price := range item.SalePrices {
			f, err := strconv.ParseFloat(strings.TrimSpace(price), 64)
			if err != nil || f <= 0 {
				return nil, errcode.InvalidParam("商品 %d 周期 %s：售价必须是大于 0 的数字（得到 %q）", item.UpstreamPid, cycle, price)
			}
		}
		if item.GroupID == 0 && strings.TrimSpace(item.GroupName) == "" {
			return nil, errcode.InvalidParam("商品 %d：必须选择已有分组（group_id）或提供新分组名（group_name）", item.UpstreamPid)
		}
		if item.Status != nil && *item.Status != 0 && *item.Status != 1 {
			return nil, errcode.InvalidParam("商品 %d：status 只能是 0（下架）或 1（上架）", item.UpstreamPid)
		}
	}

	// 2. 拉上游商品树，建立 pid → 上游商品索引
	client, err := s.client(ctx)
	if err != nil {
		return nil, s.mapUpstreamError(err)
	}
	groups, err := client.GetProducts(ctx)
	if err != nil {
		return nil, s.mapUpstreamError(err)
	}
	upstreamIndex := make(map[int64]upstream.ParsedProduct)
	for _, g := range groups {
		for _, p := range g.Products {
			upstreamIndex[p.ID] = p
		}
	}
	for _, pid := range pids {
		if _, ok := upstreamIndex[pid]; !ok {
			return nil, errcode.InvalidParam("上游商品 %d 不存在（可能已下架，请刷新商品列表）", pid)
		}
	}

	// 3. 批量拉 prodetail：成本价与库存同源真源（批 2 第二个 P0 修复；
	//    失败不阻断导入——售价已校验通过，成本/库存记空并记日志）
	details := make(map[int64]*upstream.ProductDetail)
	if d, e := fetchProductDetails(ctx, client, pids); e == nil {
		details = d
	} else {
		slog.Warn("导入时拉取上游商品详情失败（不阻断，成本与库存按空记录）", "err", e)
	}

	// 4. 逐条 upsert
	result := &ImportResult{}
	for i := range req.Items {
		item := &req.Items[i]
		up := upstreamIndex[item.UpstreamPid]

		groupID, err := s.resolveGroup(ctx, db, item)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = up.Name
		}
		status := model.ProductStatusOn
		if item.Status != nil {
			status = *item.Status
		}
		// 成本价与库存取自 prodetail（可能缺失 → 空成本/0 库存）
		cost := model.PriceMap{}
		stockQty, stockControl := 0, false
		if detail, ok := details[item.UpstreamPid]; ok && detail != nil {
			if detail.CostPrices != nil {
				cost = detail.CostPrices
			}
			stockQty, stockControl = detail.Qty, detail.StockControl
		}

		var existing model.Product
		err = db.WithContext(ctx).Where("upstream_pid = ?", item.UpstreamPid).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			row := model.Product{
				GroupID:      groupID,
				UpstreamPid:  item.UpstreamPid,
				Name:         name,
				Description:  up.Description,
				Type:         up.Type,
				CostPrices:   cost.Marshal(),
				SalePrices:   item.SalePrices.Marshal(),
				StockQty:     stockQty,
				StockControl: stockControl,
				Status:       status,
			}
			if e := db.WithContext(ctx).Create(&row).Error; e != nil {
				return nil, errcode.Internal("创建商品 %d 失败: %v", item.UpstreamPid, e)
			}
			result.Created++
			continue
		}
		if err != nil {
			return nil, errcode.Internal("查询商品映射失败").WithErr(err)
		}
		// 幂等更新：映射、售价、分组、状态；成本/描述/类型随上游最新
		updates := map[string]any{
			"group_id":      groupID,
			"name":          name,
			"description":   up.Description,
			"type":          up.Type,
			"cost_prices":   cost.Marshal(),
			"sale_prices":   item.SalePrices.Marshal(),
			"stock_qty":     stockQty,
			"stock_control": stockControl,
			"status":        status,
		}
		if e := db.WithContext(ctx).Model(&existing).Updates(updates).Error; e != nil {
			return nil, errcode.Internal("更新商品 %d 失败: %v", item.UpstreamPid, e)
		}
		result.Updated++
	}
	slog.Info("上游商品导入完成", "created", result.Created, "updated", result.Updated)
	return result, nil
}

// resolveGroup 解析导入项的分组：GroupID>0 校验存在；否则按 GroupName FirstOrCreate。
func (s *UpstreamService) resolveGroup(ctx context.Context, db *gorm.DB, item *ImportItem) (uint, error) {
	if item.GroupID > 0 {
		var group model.ProductGroup
		if err := db.WithContext(ctx).First(&group, item.GroupID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return 0, errcode.InvalidParam("分组 %d 不存在", item.GroupID)
			}
			return 0, errcode.Internal("查询分组失败").WithErr(err)
		}
		return group.ID, nil
	}
	name := strings.TrimSpace(item.GroupName)
	if len([]rune(name)) > 128 {
		return 0, errcode.InvalidParam("分组名不超过 128 字符")
	}
	group := model.ProductGroup{Name: name, Status: 1}
	if err := db.WithContext(ctx).
		Where(model.ProductGroup{Name: name}).
		Attrs(model.ProductGroup{Status: 1}).
		FirstOrCreate(&group).Error; err != nil {
		return 0, errcode.Internal("创建分组 %q 失败: %v", name, err)
	}
	return group.ID, nil
}

// GetUpstreamHosts 手动拉取上游主机列表（对账用只读数据）。
func (s *UpstreamService) GetUpstreamHosts(ctx context.Context, page, limit int) (*upstream.HostList, error) {
	client, err := s.client(ctx)
	if err != nil {
		return nil, s.mapUpstreamError(err)
	}
	list, err := client.GetHosts(ctx, page, limit)
	if err != nil {
		return nil, s.mapUpstreamError(err)
	}
	return list, nil
}

// ---- 内部助手 ----

// client 返回可复用的上游客户端（凭据来自 settings；未配置返回 ErrNotConfigured）。
func (s *UpstreamService) client(ctx context.Context) (*upstream.Client, error) {
	mgr := s.app.GetUpstream()
	if mgr == nil {
		return nil, errcode.Internal("上游模块未就绪")
	}
	return mgr.Client(ctx)
}

// mapUpstreamError 把 upstream 层错误映射为契约错误码。
func (s *UpstreamService) mapUpstreamError(err error) error {
	switch {
	case errors.Is(err, upstream.ErrNotConfigured):
		return errcode.New(400, errcode.CodeUpstreamNotConfigured, err.Error())
	case errors.Is(err, upstream.ErrAuthFailed):
		return errcode.New(502, errcode.CodeUpstreamAuthFailed, readableUpstreamError(err))
	default:
		return errcode.New(502, errcode.CodeUpstreamRequestFailed, readableUpstreamError(err))
	}
}

// readableUpstreamError 输出可读错误文本（保留底层原因）。
func readableUpstreamError(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("上游请求失败: %v", err)
}

// requireDB 返回数据库句柄。
func (s *UpstreamService) requireDB() *gorm.DB {
	return s.app.GetDB()
}
