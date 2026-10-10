package service

// 订单域（契约文档第 13 章）：下单快照 → 余额支付 → 异步开通转发上游。
//
// 关键取舍（ADR-026/027/028/029）：
//   - 单商品「立即购买」形态：每单固定一个条目（qty ≥ 1），购物车留批 4；
//   - 下单按**当前售价**生成快照（unit_price/amount 落 order_items，后续改价不影响历史订单）；
//   - 支付 = 单事务「锁订单行 → 锁会员行 → 校验余额 → 扣减 + consume 流水 → 订单转 Paid」，
//     行级锁串行化并发支付，余额不足 400 balance_insufficient，绝无透支；
//   - 支付成功后**异步**触发开通（goroutine worker，DB 状态为真源，重启可恢复）。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/model"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/app"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/errcode"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/money"
)

// OrderService 处理订单业务。
type OrderService struct {
	app       *app.App
	provision *ProvisionService
}

// NewOrder 构造订单服务（provision 用于支付成功后异步触发开通，可为 nil=不触发）。
func NewOrder(a *app.App, p *ProvisionService) *OrderService {
	return &OrderService{app: a, provision: p}
}

// maxOrderQty 是单条目数量上限（上游 allow_qty 因商品而异，本批统一收口 1-10，
// 真实库存约束由 stock_control 商品的 proinfo 实时校验兜底）。
const maxOrderQty = 10

// ---- 13.2 下单 ----

// CreateOrderRequest 是会员下单请求（单商品立即购买）。
type CreateOrderRequest struct {
	ProductID uint   `json:"product_id"`
	Cycle     string `json:"cycle"`
	Qty       int    `json:"qty"`
}

// Create 下单：商品上架/周期售价/库存校验 → 按当前售价生成快照 → Pending 待支付。
func (s *OrderService) Create(ctx context.Context, clientID uint, req *CreateOrderRequest, lang string) (*OrderView, error) {
	db := s.app.GetDB()
	if req.Qty < 1 || req.Qty > maxOrderQty {
		return nil, errcode.InvalidParam("qty 必须是 1-%d 的整数", maxOrderQty)
	}
	var product model.Product
	err := db.WithContext(ctx).First(&product, req.ProductID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.New(404, errcode.CodeProductNotFound, "商品不存在")
	}
	if err != nil {
		return nil, errcode.Internal("查询商品失败").WithErr(err)
	}
	if product.Status != model.ProductStatusOn {
		return nil, errcode.New(400, errcode.CodeProductOffSale, "商品已下架")
	}
	sale, err := model.ParsePriceMap(product.SalePrices)
	if err != nil {
		return nil, errcode.Internal("商品售价数据损坏").WithErr(err)
	}
	cycle := strings.TrimSpace(req.Cycle)
	priceText, ok := sale[cycle]
	if !ok {
		return nil, errcode.New(400, errcode.CodeCycleUnavailable,
			fmt.Sprintf("商品未配置周期 %s 的售价（可售周期: %s）", cycle, strings.Join(sale.SortedCycles(), ", ")))
	}
	unitCents, err := money.ParseCents(priceText)
	if err != nil || unitCents <= 0 {
		return nil, errcode.New(400, errcode.CodeCycleUnavailable,
			fmt.Sprintf("商品周期 %s 售价非法（%q）", cycle, priceText))
	}

	// 库存校验：stock_control 商品实时查上游 proinfo（上游不可用时回退本地快照，
	// 记日志不阻断——上游故障不应瘫痪本地商城；超卖风险由上游 settle 端二次拦截）
	if product.StockControl {
		available := product.StockQty
		if live, e := s.liveStock(ctx, product.UpstreamPid); e == nil {
			available = live
		} else {
			slog.Warn("下单实时库存查询失败（回退本地快照）", "product_id", product.ID, "err", e)
		}
		if available < req.Qty {
			return nil, errcode.New(400, errcode.CodeStockInsufficient,
				fmt.Sprintf("库存不足：剩余 %d，需要 %d", available, req.Qty))
		}
	}

	amountCents := unitCents * int64(req.Qty)

	// 订单号生成 + 落库（唯一索引冲突时重取序号，最多 5 次）
	var order model.Order
	var item model.OrderItem
	for attempt := 0; attempt < 5; attempt++ {
		orderNo, e := s.nextOrderNo(ctx, attempt)
		if e != nil {
			return nil, e
		}
		now := time.Now().Local()
		order = model.Order{
			OrderNo: orderNo, ClientID: clientID,
			Status: model.OrderStatusPending,
			Amount: money.FormatCents(amountCents), BalanceUsed: "0.00",
			CreatedAt: now, UpdatedAt: now,
		}
		item = model.OrderItem{
			ProductID: product.ID, ProductName: product.Name,
			UpstreamPid: product.UpstreamPid,
			Cycle:       cycle, Qty: req.Qty,
			UnitPrice: money.FormatCents(unitCents), Amount: money.FormatCents(amountCents),
			ProvisionStatus: model.ProvisionPending,
			CreatedAt:       now, UpdatedAt: now,
		}
		e = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if e := tx.WithContext(ctx).Create(&order).Error; e != nil {
				return e
			}
			item.OrderID = order.ID
			return tx.WithContext(ctx).Create(&item).Error
		})
		if e == nil {
			break
		}
		if !isDuplicateKeyErr(e) {
			return nil, errcode.Internal("创建订单失败").WithErr(e)
		}
		slog.Warn("订单号冲突（并发下单），重取序号", "order_no", orderNo, "attempt", attempt)
	}
	if order.ID == 0 {
		return nil, errcode.Internal("创建订单失败：订单号连续冲突，请重试")
	}
	slog.Info("会员下单成功", "client_id", clientID, "order_no", order.OrderNo,
		"product_id", product.ID, "cycle", cycle, "qty", req.Qty, "amount", order.Amount)
	view, err := s.detailView(ctx, &order, lang)
	if err != nil {
		return nil, err
	}
	return view, nil
}

// nextOrderNo 生成人类可读订单号：LIO + YYYYMMDD + 4 位日内序号（如 LIO202610100001）。
// 序号 = 当日已有订单数 + 1 + attempt（并发冲突由 order_no 唯一索引兜底重试）。
func (s *OrderService) nextOrderNo(ctx context.Context, attempt int) (string, error) {
	db := s.app.GetDB()
	now := time.Now().Local()
	prefix := "LIO" + now.Format("20060102")
	var count int64
	if err := db.WithContext(ctx).Model(&model.Order{}).
		Where("order_no LIKE ?", prefix+"%").Count(&count).Error; err != nil {
		return "", errcode.Internal("统计当日订单序号失败").WithErr(err)
	}
	seq := count + 1 + int64(attempt)
	if seq > 9999 {
		return "", errcode.Internal("当日订单序号超出 9999，请联系管理员")
	}
	return fmt.Sprintf("%s%04d", prefix, seq), nil
}

// liveStock 实时查询上游库存（proinfo）；上游未配置/失败返回错误由调用方降级。
func (s *OrderService) liveStock(ctx context.Context, upstreamPid int64) (int, error) {
	mgr := s.app.GetUpstream()
	if mgr == nil {
		return 0, errors.New("上游模块未就绪")
	}
	client, err := mgr.Client(ctx)
	if err != nil {
		return 0, err
	}
	infos, err := client.GetProductInfo(ctx, []int64{upstreamPid})
	if err != nil {
		return 0, err
	}
	for _, info := range infos {
		if info.ID == upstreamPid {
			if !info.StockControl {
				return maxOrderQty, nil // 上游关闭库存控制 = 不限
			}
			return info.Qty, nil
		}
	}
	return 0, fmt.Errorf("上游未返回商品 %d 的库存信息", upstreamPid)
}

// ---- 13.3 支付 ----

// PayOrderRequest 是支付请求。
type PayOrderRequest struct {
	Method string `json:"method"` // 本批仅 balance（epay 预留批 4）
}

// Pay 余额支付：单事务「锁订单 → 锁会员 → 校验/扣余额 + consume 流水 → 转 Paid」，
// 提交后异步触发开通转发（ProvisionFailed/Refunded 等后续状态由 worker 推进）。
func (s *OrderService) Pay(ctx context.Context, clientID, orderID uint, req *PayOrderRequest, lang string) (*OrderView, error) {
	db := s.app.GetDB()
	method := strings.TrimSpace(req.Method)
	if method == "" {
		method = model.PayMethodBalance
	}
	if method != model.PayMethodBalance {
		return nil, errcode.New(400, errcode.CodePayMethodUnsupported,
			fmt.Sprintf("支付方式 %q 暂不支持（本批仅 balance 余额支付，易支付批 4 接入）", method))
	}
	var order model.Order
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 锁订单行并校验归属与状态（他人订单按不存在处理，防枚举）
		locked, e := lockOrderForUpdate(ctx, tx, orderID, clientID)
		if e != nil {
			return e
		}
		order = *locked
		if order.Status != model.OrderStatusPending {
			return errcode.New(400, errcode.CodeOrderStateInvalid,
				fmt.Sprintf("订单当前状态为 %s，仅待支付（Pending）订单可支付", order.Status))
		}
		amountCents, e := money.ParseCents(order.Amount)
		if e != nil {
			return errcode.Internal("订单金额数据损坏").WithErr(e)
		}
		// 2. 锁会员行 → 校验余额 → 扣减 + 流水（余额不足 → balance_insufficient 回滚）
		client, e := lockClientForUpdate(ctx, tx, order.ClientID)
		if e != nil {
			return e
		}
		if _, e := debitBalanceTx(ctx, tx, client, amountCents,
			model.BalanceRelatedOrder, order.ID, "订单支付 "+order.OrderNo); e != nil {
			return e
		}
		// 3. 订单转 Paid
		now := time.Now().Local()
		updates := map[string]any{
			"status": model.OrderStatusPaid, "pay_method": method,
			"pay_time": &now, "balance_used": order.Amount, "updated_at": now,
		}
		if e := tx.WithContext(ctx).Model(&model.Order{}).
			Where("id = ?", order.ID).Updates(updates).Error; e != nil {
			return errcode.Internal("更新订单状态失败").WithErr(e)
		}
		order.Status = model.OrderStatusPaid
		order.PayMethod = method
		order.PayTime = &now
		order.BalanceUsed = order.Amount
		return nil
	})
	if err != nil {
		return nil, err
	}
	slog.Info("订单余额支付成功", "client_id", clientID, "order_no", order.OrderNo,
		"amount", order.Amount)
	// 4. 事务提交后异步触发开通（失败不回滚支付——开通补偿路径见 ADR-029）
	if s.provision != nil {
		s.provision.Trigger(order.ID)
	}
	view, err := s.detailView(ctx, &order, lang)
	if err != nil {
		return nil, err
	}
	return view, nil
}

// ---- 13.3 取消（Pending 专属；已支付不可取消，退款走后台/批 4） ----

// Cancel 取消待支付订单（超时任务的同源逻辑；已支付订单拒绝 → order_state_invalid）。
func (s *OrderService) Cancel(ctx context.Context, clientID, orderID uint, lang string) (*OrderView, error) {
	db := s.app.GetDB()
	var order model.Order
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked, e := lockOrderForUpdate(ctx, tx, orderID, clientID)
		if e != nil {
			return e
		}
		order = *locked
		if !model.CanTransition(order.Status, model.OrderStatusCancelled) {
			return errcode.New(400, errcode.CodeOrderStateInvalid,
				fmt.Sprintf("订单当前状态为 %s，仅待支付（Pending）订单可取消", order.Status))
		}
		now := time.Now().Local()
		if e := tx.WithContext(ctx).Model(&model.Order{}).Where("id = ?", order.ID).
			Updates(map[string]any{"status": model.OrderStatusCancelled,
				"cancel_time": &now, "updated_at": now}).Error; e != nil {
			return errcode.Internal("取消订单失败").WithErr(e)
		}
		order.Status = model.OrderStatusCancelled
		order.CancelTime = &now
		return nil
	})
	if err != nil {
		return nil, err
	}
	slog.Info("订单已取消", "client_id", clientID, "order_no", order.OrderNo)
	return s.detailView(ctx, &order, lang)
}

// ---- 后台订单管理（13.5） ----

// AdminRetryProvision 后台重试开通（ProvisionFailed → Paid，重新触发 worker）。
// 已有上游主机 ID 的条目转回 provisioning（继续轮询）；没有的回到 pending
// （重新走 add_to_shop + settle——⚠ 重试前管理员应先到后台「上游主机」页对账，
// 确认上游没有本单的残留账单/主机，防止 settle 网络性失败后重复下单，SOP 见 ADR-029）。
func (s *OrderService) AdminRetryProvision(ctx context.Context, orderID uint, lang string) (*OrderView, error) {
	db := s.app.GetDB()
	var order model.Order
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked, e := lockOrderForUpdate(ctx, tx, orderID, 0)
		if e != nil {
			return e
		}
		order = *locked
		if order.Status != model.OrderStatusProvisionFailed {
			return errcode.New(400, errcode.CodeOrderStateInvalid,
				fmt.Sprintf("订单当前状态为 %s，仅开通失败（ProvisionFailed）订单可重试", order.Status))
		}
		if !model.CanTransition(order.Status, model.OrderStatusPaid) {
			return errcode.New(400, errcode.CodeOrderStateInvalid, "状态机拒绝该迁移")
		}
		var items []model.OrderItem
		if e := tx.WithContext(ctx).Where("order_id = ?", order.ID).Find(&items).Error; e != nil {
			return errcode.Internal("查询订单条目失败").WithErr(e)
		}
		now := time.Now().Local()
		for i := range items {
			it := &items[i]
			if it.ProvisionStatus == model.ProvisionActive {
				continue
			}
			target := model.ProvisionPending
			if it.UpstreamHostID > 0 {
				target = model.ProvisionProvisioning // 已拿到上游主机 → 只需继续轮询
			}
			if e := tx.WithContext(ctx).Model(&model.OrderItem{}).Where("id = ?", it.ID).
				Update("provision_status", target).Error; e != nil {
				return errcode.Internal("重置条目开通状态失败").WithErr(e)
			}
		}
		remark := appendRemark(order.Remark, "管理员重试开通")
		if e := tx.WithContext(ctx).Model(&model.Order{}).Where("id = ?", order.ID).
			Updates(map[string]any{"status": model.OrderStatusPaid,
				"remark": remark, "updated_at": now}).Error; e != nil {
			return errcode.Internal("更新订单状态失败").WithErr(e)
		}
		order.Status = model.OrderStatusPaid
		order.Remark = remark
		return nil
	})
	if err != nil {
		return nil, err
	}
	slog.Info("后台重试开通", "order_no", order.OrderNo)
	if s.provision != nil {
		s.provision.Trigger(order.ID)
	}
	return s.detailView(ctx, &order, lang)
}

// AdminCancelRefund 后台取消并退款（ProvisionFailed → Refunded）：
// balance_used 原路退回余额 + refund 流水（同事务），订单终态 Refunded。
// ⚠ 上游侧可能已存在账单/主机（settle 成功但开通失败的场景），退款不动上游，
// remark 中提示人工到上游对账处理。
func (s *OrderService) AdminCancelRefund(ctx context.Context, adminID, orderID uint, remarkReq, lang string) (*OrderView, error) {
	db := s.app.GetDB()
	var order model.Order
	aid := adminID
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked, e := lockOrderForUpdate(ctx, tx, orderID, 0)
		if e != nil {
			return e
		}
		order = *locked
		if order.Status != model.OrderStatusProvisionFailed {
			return errcode.New(400, errcode.CodeOrderStateInvalid,
				fmt.Sprintf("订单当前状态为 %s，仅开通失败（ProvisionFailed）订单可取消并退款", order.Status))
		}
		if !model.CanTransition(order.Status, model.OrderStatusRefunded) {
			return errcode.New(400, errcode.CodeOrderStateInvalid, "状态机拒绝该迁移")
		}
		refundCents, e := money.ParseCents(order.BalanceUsed)
		if e != nil {
			return errcode.Internal("订单支付金额数据损坏").WithErr(e)
		}
		if refundCents > 0 {
			client, e := lockClientForUpdate(ctx, tx, order.ClientID)
			if e != nil {
				return e
			}
			note := "订单退款 " + order.OrderNo
			if r := strings.TrimSpace(remarkReq); r != "" {
				note += "：" + r
			}
			if _, e := creditBalanceTx(ctx, tx, client, refundCents,
				model.BalanceTypeRefund, model.BalanceRelatedOrder, order.ID, note, &aid); e != nil {
				return e
			}
		}
		now := time.Now().Local()
		remark := appendRemark(order.Remark,
			fmt.Sprintf("管理员取消并退款%s（上游如有残留账单/主机需人工对账处理）", remarkSuffix(remarkReq)))
		if e := tx.WithContext(ctx).Model(&model.Order{}).Where("id = ?", order.ID).
			Updates(map[string]any{"status": model.OrderStatusRefunded,
				"remark": remark, "updated_at": now}).Error; e != nil {
			return errcode.Internal("更新订单状态失败").WithErr(e)
		}
		order.Status = model.OrderStatusRefunded
		order.Remark = remark
		return nil
	})
	if err != nil {
		return nil, err
	}
	slog.Info("后台取消订单并退款", "admin_id", adminID, "order_no", order.OrderNo,
		"refund", order.BalanceUsed)
	return s.detailView(ctx, &order, lang)
}

// ---- 列表与详情 ----

// ClientList 会员端订单列表（status 可选过滤；只查自己的单）。
func (s *OrderService) ClientList(ctx context.Context, clientID uint, status string, page, pageSize int, lang string) (*OrderListView, error) {
	return s.list(ctx, clientID, status, "", page, pageSize, lang)
}

// AdminList 后台订单列表（keyword 模糊订单号/会员名；status 过滤）。
func (s *OrderService) AdminList(ctx context.Context, status, keyword string, page, pageSize int, lang string) (*OrderListView, error) {
	return s.list(ctx, 0, status, keyword, page, pageSize, lang)
}

// OrderListView 是订单列表响应。
type OrderListView struct {
	List     []OrderView `json:"list"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}

func (s *OrderService) list(ctx context.Context, clientID uint, status, keyword string, page, pageSize int, lang string) (*OrderListView, error) {
	db := s.app.GetDB()
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	q := db.WithContext(ctx).Model(&model.Order{})
	if clientID > 0 {
		q = q.Where("client_id = ?", clientID)
	}
	if st := strings.TrimSpace(status); st != "" {
		if !isKnownOrderStatus(st) {
			return nil, errcode.InvalidParam("status 非法（可选 Pending/Paid/Active/ProvisionFailed/Cancelled/Refunded）")
		}
		q = q.Where("status = ?", st)
	}
	if kw := strings.TrimSpace(keyword); kw != "" {
		like := "%" + kw + "%"
		// 订单号模糊 或 会员名模糊（子查询定位 client_id，避免 join 方言差异）
		var ids []uint
		if err := db.WithContext(ctx).Model(&model.Client{}).
			Where("username LIKE ?", like).Limit(500).Pluck("id", &ids).Error; err != nil {
			return nil, errcode.Internal("查询会员失败").WithErr(err)
		}
		cond := q.Where("order_no LIKE ?", like)
		if len(ids) > 0 {
			cond = q.Where("order_no LIKE ? OR client_id IN ?", like, ids)
		}
		q = cond
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, errcode.Internal("统计订单失败").WithErr(err)
	}
	var rows []model.Order
	if err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, errcode.Internal("查询订单失败").WithErr(err)
	}
	// 会员名索引（后台列表展示）
	usernames := map[uint]string{}
	if clientID == 0 && len(rows) > 0 {
		cids := make([]uint, 0, len(rows))
		for i := range rows {
			cids = append(cids, rows[i].ClientID)
		}
		var clients []model.Client
		if err := db.WithContext(ctx).Select("id", "username").
			Where("id IN ?", cids).Find(&clients).Error; err == nil {
			for _, c := range clients {
				usernames[c.ID] = c.Username
			}
		}
	}
	// 条目批量加载（N+1 收敛为 1 次 IN 查询）
	itemsByOrder := map[uint][]model.OrderItem{}
	if len(rows) > 0 {
		oids := make([]uint, 0, len(rows))
		for i := range rows {
			oids = append(oids, rows[i].ID)
		}
		var items []model.OrderItem
		if err := db.WithContext(ctx).Where("order_id IN ?", oids).
			Order("id ASC").Find(&items).Error; err != nil {
			return nil, errcode.Internal("查询订单条目失败").WithErr(err)
		}
		for i := range items {
			itemsByOrder[items[i].OrderID] = append(itemsByOrder[items[i].OrderID], items[i])
		}
	}
	list := make([]OrderView, 0, len(rows))
	for i := range rows {
		list = append(list, toOrderView(&rows[i], itemsByOrder[rows[i].ID], usernames[rows[i].ClientID], lang))
	}
	return &OrderListView{List: list, Total: total, Page: page, PageSize: pageSize}, nil
}

// ClientDetail 会员端订单详情（校验归属）。
func (s *OrderService) ClientDetail(ctx context.Context, clientID, orderID uint, lang string) (*OrderView, error) {
	db := s.app.GetDB()
	order, err := loadOrderWithClientCheck(ctx, db, orderID, clientID)
	if err != nil {
		return nil, err
	}
	return s.detailView(ctx, order, lang)
}

// AdminDetail 后台订单详情（附关联余额流水）。
func (s *OrderService) AdminDetail(ctx context.Context, orderID uint, lang string) (*OrderAdminDetailView, error) {
	db := s.app.GetDB()
	order, err := loadOrderWithClientCheck(ctx, db, orderID, 0)
	if err != nil {
		return nil, err
	}
	view, err := s.detailView(ctx, order, lang)
	if err != nil {
		return nil, err
	}
	detail := &OrderAdminDetailView{OrderView: *view, Transactions: []BalanceTransactionView{}}
	var client model.Client
	if e := db.WithContext(ctx).Select("id", "username").First(&client, order.ClientID).Error; e == nil {
		detail.ClientUsername = client.Username
	}
	// 关联流水（consume/refund/adjust，related=order）
	var txs []model.BalanceTransaction
	if e := db.WithContext(ctx).
		Where("related_type = ? AND related_id = ?", model.BalanceRelatedOrder, order.ID).
		Order("id ASC").Find(&txs).Error; e == nil {
		for i := range txs {
			detail.Transactions = append(detail.Transactions, toBalanceTransactionView(&txs[i], lang))
		}
	}
	return detail, nil
}

// OrderAdminDetailView 是后台订单详情视图（订单 + 关联流水）。
type OrderAdminDetailView struct {
	OrderView
	Transactions []BalanceTransactionView `json:"transactions"`
}

// ---- 会员主机列表（13.7 我的主机） ----

// ClientHostView 是会员端主机视图（不含密码——控制台展示状态/IP/到期/商品，
// 密码在订单详情 provision_info 与后台可见）。
type ClientHostView struct {
	ID             uint   `json:"id"`
	UpstreamHostID int64  `json:"upstream_host_id"`
	ProductID      *uint  `json:"product_id"`
	ProductName    string `json:"product_name"`
	Domain         string `json:"domain"`
	DomainStatus   string `json:"domain_status"`
	DedicatedIP    string `json:"dedicated_ip"`
	AssignedIPs    string `json:"assigned_ips"`
	Username       string `json:"username"`
	NextDueDate    string `json:"next_due_date"`
	SyncedAt       string `json:"synced_at"`
}

// ClientHosts 会员查询自己的主机（hosts.client_id 关联，开通链路回填）。
func (s *OrderService) ClientHosts(ctx context.Context, clientID uint, page, pageSize int) (*ClientHostListView, error) {
	db := s.app.GetDB()
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	q := db.WithContext(ctx).Model(&model.Host{}).Where("client_id = ?", clientID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, errcode.Internal("统计主机失败").WithErr(err)
	}
	var rows []model.Host
	if err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, errcode.Internal("查询主机失败").WithErr(err)
	}
	// 商品名索引
	names := map[uint]string{}
	pids := make([]uint, 0, len(rows))
	for i := range rows {
		if rows[i].ProductID != nil {
			pids = append(pids, *rows[i].ProductID)
		}
	}
	if len(pids) > 0 {
		var products []model.Product
		if err := db.WithContext(ctx).Select("id", "name").
			Where("id IN ?", pids).Find(&products).Error; err == nil {
			for _, p := range products {
				names[p.ID] = p.Name
			}
		}
	}
	list := make([]ClientHostView, 0, len(rows))
	for i := range rows {
		h := &rows[i]
		v := ClientHostView{
			ID: h.ID, UpstreamHostID: h.UpstreamHostID, ProductID: h.ProductID,
			Domain: h.Domain, DomainStatus: h.DomainStatus,
			DedicatedIP: h.DedicatedIP, AssignedIPs: h.AssignedIPs,
			Username: h.Username, NextDueDate: h.NextDueDate,
			SyncedAt: h.SyncedAt.Format("2006-01-02 15:04:05"),
		}
		if h.ProductID != nil {
			v.ProductName = names[*h.ProductID]
		}
		list = append(list, v)
	}
	return &ClientHostListView{List: list, Total: total, Page: page, PageSize: pageSize}, nil
}

// ClientHostListView 是主机列表响应。
type ClientHostListView struct {
	List     []ClientHostView `json:"list"`
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
}

// ---- 视图与内部助手 ----

// OrderView 是订单对外视图（会员端与后台共用；ClientUsername 仅后台填充）。
type OrderView struct {
	ID             uint            `json:"id"`
	OrderNo        string          `json:"order_no"`
	ClientID       uint            `json:"client_id"`
	ClientUsername string          `json:"client_username,omitempty"`
	Status         string          `json:"status"`
	StatusText     string          `json:"status_text"`
	Amount         string          `json:"amount"`
	BalanceUsed    string          `json:"balance_used"`
	PayMethod      string          `json:"pay_method"`
	PayTime        string          `json:"pay_time"`
	CancelTime     string          `json:"cancel_time"`
	Remark         string          `json:"remark"`
	CreatedAt      string          `json:"created_at"`
	Items          []OrderItemView `json:"items"`
}

// OrderItemView 是订单条目对外视图。
type OrderItemView struct {
	ID                  uint           `json:"id"`
	ProductID           uint           `json:"product_id"`
	ProductName         string         `json:"product_name"`
	UpstreamPid         int64          `json:"upstream_pid"`
	Cycle               string         `json:"cycle"`
	CycleText           string         `json:"cycle_text"`
	Qty                 int            `json:"qty"`
	UnitPrice           string         `json:"unit_price"`
	Amount              string         `json:"amount"`
	UpstreamOrderID     int64          `json:"upstream_order_id"`
	UpstreamHostID      int64          `json:"upstream_host_id"`
	ProvisionStatus     string         `json:"provision_status"`
	ProvisionStatusText string         `json:"provision_status_text"`
	ProvisionInfo       map[string]any `json:"provision_info"` // 解析后的 JSON（null 安全）
}

// detailView 加载条目并组装订单详情视图。
func (s *OrderService) detailView(ctx context.Context, order *model.Order, lang string) (*OrderView, error) {
	db := s.app.GetDB()
	var items []model.OrderItem
	if err := db.WithContext(ctx).Where("order_id = ?", order.ID).
		Order("id ASC").Find(&items).Error; err != nil {
		return nil, errcode.Internal("查询订单条目失败").WithErr(err)
	}
	view := toOrderView(order, items, "", lang)
	return &view, nil
}

// lockOrderForUpdate 事务内按行级锁加载订单；clientID>0 时校验归属
// （他人订单按不存在处理，防枚举探测）。
func lockOrderForUpdate(ctx context.Context, tx *gorm.DB, orderID, clientID uint) (*model.Order, error) {
	var order model.Order
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&order, orderID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.New(404, errcode.CodeOrderNotFound, "订单不存在")
	}
	if err != nil {
		return nil, errcode.Internal("查询订单失败").WithErr(err)
	}
	if clientID > 0 && order.ClientID != clientID {
		return nil, errcode.New(404, errcode.CodeOrderNotFound, "订单不存在")
	}
	return &order, nil
}

// loadOrderWithClientCheck 非事务加载订单（详情查询用；同样做归属校验）。
func loadOrderWithClientCheck(ctx context.Context, db *gorm.DB, orderID, clientID uint) (*model.Order, error) {
	var order model.Order
	err := db.WithContext(ctx).First(&order, orderID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.New(404, errcode.CodeOrderNotFound, "订单不存在")
	}
	if err != nil {
		return nil, errcode.Internal("查询订单失败").WithErr(err)
	}
	if clientID > 0 && order.ClientID != clientID {
		return nil, errcode.New(404, errcode.CodeOrderNotFound, "订单不存在")
	}
	return &order, nil
}

// toOrderView 模型 → 视图（状态/周期/开通状态文案按语言输出，范式同 clientStatusText）。
func toOrderView(o *model.Order, items []model.OrderItem, clientUsername, lang string) OrderView {
	view := OrderView{
		ID: o.ID, OrderNo: o.OrderNo, ClientID: o.ClientID,
		ClientUsername: clientUsername,
		Status:         o.Status, StatusText: OrderStatusText(o.Status, lang),
		Amount: o.Amount, BalanceUsed: o.BalanceUsed,
		PayMethod: o.PayMethod, Remark: o.Remark,
		CreatedAt: o.CreatedAt.Format("2006-01-02 15:04:05"),
		Items:     make([]OrderItemView, 0, len(items)),
	}
	if o.PayTime != nil {
		view.PayTime = o.PayTime.Format("2006-01-02 15:04:05")
	}
	if o.CancelTime != nil {
		view.CancelTime = o.CancelTime.Format("2006-01-02 15:04:05")
	}
	for i := range items {
		view.Items = append(view.Items, toOrderItemView(&items[i], lang))
	}
	return view
}

// toOrderItemView 条目视图转换（provision_info JSON 解析失败时输出 nil，不阻断）。
func toOrderItemView(it *model.OrderItem, lang string) OrderItemView {
	v := OrderItemView{
		ID: it.ID, ProductID: it.ProductID, ProductName: it.ProductName,
		UpstreamPid: it.UpstreamPid,
		Cycle:       it.Cycle, CycleText: CycleText(it.Cycle, lang),
		Qty: it.Qty, UnitPrice: it.UnitPrice, Amount: it.Amount,
		UpstreamOrderID: it.UpstreamOrderID, UpstreamHostID: it.UpstreamHostID,
		ProvisionStatus:     it.ProvisionStatus,
		ProvisionStatusText: ProvisionStatusText(it.ProvisionStatus, lang),
	}
	if strings.TrimSpace(it.ProvisionInfo) != "" {
		var info map[string]any
		if err := json.Unmarshal([]byte(it.ProvisionInfo), &info); err == nil {
			v.ProvisionInfo = info
		}
	}
	return v
}

// isKnownOrderStatus 判断状态过滤值是否合法。
func isKnownOrderStatus(s string) bool {
	switch s {
	case model.OrderStatusPending, model.OrderStatusPaid, model.OrderStatusActive,
		model.OrderStatusProvisionFailed, model.OrderStatusCancelled, model.OrderStatusRefunded:
		return true
	}
	return false
}

// appendRemark 追加备注（512 字节截断保护，保留最新内容）。
func appendRemark(old, add string) string {
	merged := old
	if merged != "" {
		merged += "；"
	}
	merged += add
	return truncateTo(merged, 512)
}

// remarkSuffix 拼接管理端备注后缀。
func remarkSuffix(r string) string {
	r = strings.TrimSpace(r)
	if r == "" {
		return ""
	}
	return "：" + truncateTo(r, 200)
}

// OrderStatusText 输出订单状态文案（会员端/后台/日志共用；en 前缀 → 英文）。
func OrderStatusText(status, lang string) string {
	en := strings.HasPrefix(strings.ToLower(lang), "en")
	switch status {
	case model.OrderStatusPending:
		if en {
			return "Pending Payment"
		}
		return "待支付"
	case model.OrderStatusPaid:
		if en {
			return "Provisioning"
		}
		return "开通中"
	case model.OrderStatusActive:
		if en {
			return "Active"
		}
		return "已开通"
	case model.OrderStatusProvisionFailed:
		if en {
			return "Provision Failed"
		}
		return "开通失败"
	case model.OrderStatusCancelled:
		if en {
			return "Cancelled"
		}
		return "已取消"
	case model.OrderStatusRefunded:
		if en {
			return "Refunded"
		}
		return "已退款"
	}
	if en {
		return "Unknown"
	}
	return "未知"
}

// ProvisionStatusText 输出条目开通状态文案。
func ProvisionStatusText(status, lang string) string {
	en := strings.HasPrefix(strings.ToLower(lang), "en")
	switch status {
	case model.ProvisionPending:
		if en {
			return "Pending"
		}
		return "未开始"
	case model.ProvisionProvisioning:
		if en {
			return "Provisioning"
		}
		return "开通中"
	case model.ProvisionActive:
		if en {
			return "Active"
		}
		return "已开通"
	case model.ProvisionFailed:
		if en {
			return "Failed"
		}
		return "失败"
	}
	if en {
		return "Unknown"
	}
	return "未知"
}

// cycleTexts 是计费周期中英文文案表（键与上游 product_pricings 一致）。
var cycleTexts = map[string][2]string{
	"onetime":      {"一次性", "One-time"},
	"hour":         {"时付", "Hourly"},
	"day":          {"日付", "Daily"},
	"ontrial":      {"试用", "Trial"},
	"monthly":      {"月付", "Monthly"},
	"quarterly":    {"季付", "Quarterly"},
	"semiannually": {"半年付", "Semi-annually"},
	"annually":     {"年付", "Annually"},
	"biennially":   {"两年付", "Biennially"},
	"triennially":  {"三年付", "Triennially"},
	"fourly":       {"四年付", "4 Years"},
	"fively":       {"五年付", "5 Years"},
	"sixly":        {"六年付", "6 Years"},
	"sevenly":      {"七年付", "7 Years"},
	"eightly":      {"八年付", "8 Years"},
	"ninely":       {"九年付", "9 Years"},
	"tenly":        {"十年付", "10 Years"},
}

// CycleText 输出计费周期文案（未知周期原样返回键名）。
func CycleText(cycle, lang string) string {
	pair, ok := cycleTexts[cycle]
	if !ok {
		return cycle
	}
	if strings.HasPrefix(strings.ToLower(lang), "en") {
		return pair[1]
	}
	return pair[0]
}
