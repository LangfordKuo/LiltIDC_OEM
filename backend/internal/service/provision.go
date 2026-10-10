package service

// 开通转发 worker（契约文档 13.4/13.8，ADR-026/029）：
//
// 形态取舍：**每订单一个 goroutine + 信号量限流（4 并发）+ 内存 inflight 去重**，
// 不引入持久化任务队列——DB 状态（orders.status / order_items.provision_status）
// 就是队列真源，goroutine 只是驱动器；进程重启由 Start() 的恢复扫描兜底
// （Paid 订单重新 Trigger：已有上游主机 ID 的条目继续轮询，没有的重新结算）。
// 单机部署（本项目形态）下该方案最简单且崩溃可恢复；多实例部署需要分布式锁，留待需要时引入。
//
// 轮询为主、推送为辅：开通结果以 GET /host/header 轮询为准（10s × 30 次 ≈ 5 分钟），
// host/sync 推送仅作生产环境的加速补充——本机/内网部署上游推不进来，轮询保证闭环。
//
// 补偿路径（ADR-029）：
//   - 上游**明确业务拒绝**（add_to_shop/settle/apply_credit 返回 *upstream.BizError，
//     如余额不足/商品下架/库存不足）→ 本地余额自动原路退回（refund 流水）→ 订单 Refunded；
//   - **网络性未知错误**（超时/连接失败/协议异常）→ 款不动，订单 ProvisionFailed，
//     后台人工对账上游后「重试开通」或「取消并退款」——网络错误时上游可能已受理，
//     自动退款会造成钱退了机器还在开的双向资损。

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/model"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/app"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/errcode"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/money"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/upstream"
)

// 开通轮询参数（任务约定：间隔 10s、上限 30 次 ≈ 5 分钟）。
const (
	provisionPollInterval = 10 * time.Second
	provisionPollMaxRound = 30
	// 未支付订单超时自动取消：每 5 分钟扫描，Pending 超 24h 取消（ADR-026）。
	pendingSweepInterval = 5 * time.Minute
	pendingOrderTimeout  = 24 * time.Hour
	// 并发开通 worker 上限（信号量容量）。
	maxConcurrentProvision = 4
)

// ProvisionService 是开通转发 worker。
type ProvisionService struct {
	app  *app.App
	sem  chan struct{}
	mu   sync.Mutex
	infl map[uint]bool // 在途订单去重（防重复触发同一单）
}

// NewProvision 构造开通服务。
func NewProvision(a *app.App) *ProvisionService {
	return &ProvisionService{
		app:  a,
		sem:  make(chan struct{}, maxConcurrentProvision),
		infl: map[uint]bool{},
	}
}

// ProvisionInfo 是 order_items.provision_info 的 JSON 结构（开通全过程快照）。
type ProvisionInfo struct {
	Hostname         string  `json:"hostname,omitempty"`           // 下单生成的主机名（提交上游）
	Password         string  `json:"password,omitempty"`           // 下单生成的主机密码（明文，与 hosts 表同策略）
	CurrencyID       int64   `json:"currency_id,omitempty"`        // 结算所用上游货币 ID
	HostIDs          []int64 `json:"host_ids,omitempty"`           // 上游主机 ID 全集（qty>1 多台）
	IP               string  `json:"ip,omitempty"`                 // 首台独享 IP（开通回填）
	AssignedIPs      string  `json:"assigned_ips,omitempty"`       // 附加 IP
	Username         string  `json:"username,omitempty"`           // 上游回填主机用户名
	HostPassword     string  `json:"host_password,omitempty"`      // 上游回填主机密码（通常同 Password）
	NextDueDate      string  `json:"next_due_date,omitempty"`      // 到期时间（上游 unix 秒 → 本地文本）
	DomainStatus     string  `json:"domain_status,omitempty"`      // 最新主机状态
	UpstreamOrderNum string  `json:"upstream_order_num,omitempty"` // 上游订单号（ordernum）
	SettledAt        string  `json:"settled_at,omitempty"`         // settle 成功时间
	ActivatedAt      string  `json:"activated_at,omitempty"`       // 开通成功时间
	Attempts         int     `json:"attempts,omitempty"`           // 开通尝试次数
	Error            string  `json:"error,omitempty"`              // 最近一次失败原因（如实记录）
}

// provisionOutcome 是单条目结算转发的结果分类。
type provisionOutcome int

const (
	outcomeOK        provisionOutcome = iota // 已受理（进入轮询）或已开通
	outcomeBizReject                         // 上游明确业务拒绝 → 自动退款
	outcomeNetError                          // 网络性未知错误 → 冻结资金转人工
)

// Start 启动后台任务（幂等，app.StartBackground 的 sync.Once 保证只执行一次）：
// 恢复扫描（重启前 Paid 的订单重新驱动）+ 未支付超时取消定时器。
func (s *ProvisionService) Start() {
	go s.resumeOnStartup()
	go s.sweepLoop()
	slog.Info("开通转发 worker 已启动",
		"poll_interval", provisionPollInterval.String(), "poll_max_round", provisionPollMaxRound,
		"pending_timeout", pendingOrderTimeout.String())
}

// Trigger 异步触发某订单的开通转发（inflight 去重 + 信号量限流）。
func (s *ProvisionService) Trigger(orderID uint) {
	s.mu.Lock()
	if s.infl[orderID] {
		s.mu.Unlock()
		slog.Info("订单开通已在途，跳过重复触发", "order_id", orderID)
		return
	}
	s.infl[orderID] = true
	s.mu.Unlock()
	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.infl, orderID)
			s.mu.Unlock()
		}()
		s.sem <- struct{}{}
		defer func() { <-s.sem }()
		s.provisionOrder(orderID)
	}()
}

// provisionOrder 驱动一个订单的完整开通流程（结算转发 → 轮询 → 终态落库）。
func (s *ProvisionService) provisionOrder(orderID uint) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("开通 worker panic（订单保持 Paid，重启后恢复扫描接管）",
				"order_id", orderID, "panic", r)
		}
	}()
	ctx := context.Background()
	db := s.app.GetDB()
	if db == nil {
		slog.Error("开通 worker：数据库未就绪", "order_id", orderID)
		return
	}
	var order model.Order
	if err := db.WithContext(ctx).First(&order, orderID).Error; err != nil {
		slog.Error("开通 worker：订单不存在", "order_id", orderID, "err", err)
		return
	}
	if order.Status != model.OrderStatusPaid {
		slog.Info("开通 worker：订单非 Paid 状态，跳过", "order_no", order.OrderNo, "status", order.Status)
		return
	}
	var items []model.OrderItem
	if err := db.WithContext(ctx).Where("order_id = ?", orderID).
		Order("id ASC").Find(&items).Error; err != nil {
		slog.Error("开通 worker：查询条目失败", "order_id", orderID, "err", err)
		return
	}

	client, err := s.upstreamClient(ctx)
	if err != nil {
		s.failOrder(ctx, &order, &items, "上游不可用："+err.Error(), false)
		return
	}
	// 结算需要上游货币 ID（/cart/credit 的 currency.id；lyew.com 实测 CNY=1）
	currencyID, err := s.fetchCurrencyID(ctx, client)
	if err != nil {
		s.failOrder(ctx, &order, &items, "获取上游货币失败："+err.Error(), false)
		return
	}

	// ---- 阶段 1：结算转发（pending 条目 → add_to_shop + settle + apply_credit） ----
	for i := range items {
		it := &items[i]
		if it.ProvisionStatus != model.ProvisionPending {
			continue // provisioning=重试时继续轮询；active/failed 不动
		}
		outcome, reason := s.provisionItem(ctx, client, &order, it, currencyID)
		switch outcome {
		case outcomeBizReject:
			// 明确业务拒绝：自动退款（refund 流水）→ Refunded 终态
			s.refundOrder(ctx, &order, &items, reason)
			return
		case outcomeNetError:
			// 网络性未知错误：款不动，转人工
			s.failOrder(ctx, &order, &items, reason, false)
			return
		}
	}

	// ---- 阶段 2：轮询开通结果（provisioning 条目） ----
	s.pollItems(ctx, client, &order, &items)

	// ---- 阶段 3：终态判定 ----
	s.finalize(ctx, &order, &items)
}

// provisionItem 转发单个条目到上游（add_to_shop 预检 → settle 内联结算 → apply_credit 余额支付）。
// 成功时条目已转 provisioning（或免费单直接开通）并持久化上游 ID。
func (s *ProvisionService) provisionItem(ctx context.Context, client *upstream.Client,
	order *model.Order, it *model.OrderItem, currencyID int64) (provisionOutcome, string) {
	db := s.app.GetDB()
	info := loadProvisionInfo(it)
	info.Attempts++
	now := time.Now().Local()

	// 商品配置（可配置项默认值 + 主机名/密码规则）——尽力而为，失败不阻断
	configJSON, cfgErr := client.GetProductConfig(ctx, it.UpstreamPid)
	if cfgErr != nil {
		slog.Warn("拉取上游商品配置失败（按无配置项/默认规则继续）",
			"upstream_pid", it.UpstreamPid, "err", cfgErr)
	}
	configOptions := upstream.DefaultConfigOptions(configJSON)
	rules := parseOrderFormRules(configJSON)
	if info.Hostname == "" && rules.HostRequired {
		info.Hostname = generateHostname(rules)
	}
	if info.Password == "" && rules.PasswordRequired {
		info.Password = generateHostPassword()
	}
	info.CurrencyID = currencyID
	s.saveInfo(ctx, it, info) // 先持久化意图（崩溃后可审计我们提交了什么）

	// 1. add_to_shop 预检（库存/周期/绑定校验；残留购物车由 settle 的内联 cart_data 清空）
	addParams := upstream.AddToShopParams{
		Pid: it.UpstreamPid, BillingCycle: it.Cycle, Qty: it.Qty,
		CurrencyID: currencyID, Host: info.Hostname, Password: info.Password,
		ConfigOption: configOptions,
	}
	if err := client.AddToShop(ctx, addParams); err != nil {
		var biz *upstream.BizError
		if errors.As(err, &biz) {
			return outcomeBizReject, fmt.Sprintf("上游加购拒绝（status=%d）: %s", biz.Status, biz.Msg)
		}
		return outcomeNetError, "上游加购失败（网络性错误）: " + err.Error()
	}

	// 2. settle 内联结算（不带 downstream_* 参数——上游对共享 token 有 whereLike
	//    幂等回滚陷阱，登记改在开通完成后走 setdownstream，见 ADR-026）
	settleRes, err := client.Settle(ctx, upstream.SettleParams{
		Pid: it.UpstreamPid, BillingCycle: it.Cycle, Qty: it.Qty,
		CurrencyID: currencyID, Host: info.Hostname, Password: info.Password,
		ConfigOptions: configOptions, Notes: "LiltIDC_OEM " + order.OrderNo,
	})
	if err != nil {
		var biz *upstream.BizError
		if errors.As(err, &biz) {
			return outcomeBizReject, fmt.Sprintf("上游结算拒绝（status=%d）: %s", biz.Status, biz.Msg)
		}
		return outcomeNetError, "上游结算失败（网络性错误，上游可能已受理，请先对账再重试）: " + err.Error()
	}
	// 结算成功：立即持久化上游账单 ID（缩小「settle 成功但未落库」崩溃窗口，ADR-029）
	if settleRes.InvoiceID > 0 {
		it.UpstreamOrderID = settleRes.InvoiceID
		info.SettledAt = now.Format("2006-01-02 15:04:05")
		s.saveInfo(ctx, it, info)
		if err := db.WithContext(ctx).Model(&model.OrderItem{}).Where("id = ?", it.ID).
			Update("upstream_order_id", settleRes.InvoiceID).Error; err != nil {
			slog.Error("持久化上游账单 ID 失败", "item_id", it.ID, "err", err)
		}
	}

	hostIDs := settleRes.HostIDs
	if settleRes.InvoiceID > 0 && !settleRes.PaidFree {
		// 3. apply_credit 余额支付（成功状态码 1001；支付成功即触发上游自动开通）
		acRes, err := client.ApplyCredit(ctx, settleRes.InvoiceID)
		if err != nil {
			var biz *upstream.BizError
			if errors.As(err, &biz) {
				return outcomeBizReject, fmt.Sprintf(
					"上游余额支付拒绝（status=%d）: %s（上游账单 #%d 未支付，如有残留主机请人工对账）",
					biz.Status, biz.Msg, settleRes.InvoiceID)
			}
			return outcomeNetError, fmt.Sprintf(
				"上游余额支付失败（网络性错误，账单 #%d 状态未知，请先对账再重试）: %v",
				settleRes.InvoiceID, err)
		}
		hostIDs = acRes.HostIDs
	}
	if len(hostIDs) == 0 {
		return outcomeNetError, fmt.Sprintf("上游结算未返回账单或主机 ID（invoiceid=%d），无法跟踪开通",
			settleRes.InvoiceID)
	}
	it.UpstreamHostID = hostIDs[0]
	info.HostIDs = hostIDs
	s.saveInfo(ctx, it, info)
	if err := db.WithContext(ctx).Model(&model.OrderItem{}).Where("id = ?", it.ID).
		Updates(map[string]any{"upstream_host_id": it.UpstreamHostID,
			"provision_status": model.ProvisionProvisioning}).Error; err != nil {
		slog.Error("更新条目开通状态失败", "item_id", it.ID, "err", err)
		return outcomeNetError, "本地落库失败: " + err.Error()
	}
	it.ProvisionStatus = model.ProvisionProvisioning
	slog.Info("上游结算+支付成功，进入开通轮询", "order_no", order.OrderNo,
		"item_id", it.ID, "invoice_id", settleRes.InvoiceID, "host_ids", hostIDs)
	return outcomeOK, ""
}

// pollItems 轮询 provisioning 条目的主机状态（10s × 30 次）。
// Active → 回填 provision_info + upsert hosts + setdownstream 登记（失败不阻断）；
// Cancelled/Fraud/Deleted/Terminated → 条目失败；超时 → 条目失败（可后台重试）。
func (s *ProvisionService) pollItems(ctx context.Context, client *upstream.Client,
	order *model.Order, items *[]model.OrderItem) {
	type tracked struct {
		idx      int
		hostIDs  []int64
		done     bool
		fatalMsg string
	}
	trackedItems := []*tracked{}
	for i := range *items {
		it := &(*items)[i]
		if it.ProvisionStatus != model.ProvisionProvisioning {
			continue
		}
		info := loadProvisionInfo(it)
		ids := info.HostIDs
		if len(ids) == 0 && it.UpstreamHostID > 0 {
			ids = []int64{it.UpstreamHostID}
		}
		if len(ids) == 0 {
			continue
		}
		trackedItems = append(trackedItems, &tracked{idx: i, hostIDs: ids})
	}
	if len(trackedItems) == 0 {
		return
	}
	fatalStatuses := map[string]bool{
		model.HostStatusCancelled: true, model.HostStatusFraud: true,
		model.HostStatusDeleted: true, "Terminated": true,
	}
	for round := 1; round <= provisionPollMaxRound; round++ {
		allDone := true
		for _, tr := range trackedItems {
			if tr.done || tr.fatalMsg != "" {
				continue
			}
			it := &(*items)[tr.idx]
			headers := make([]*upstream.HostHeader, 0, len(tr.hostIDs))
			fatal := ""
			transientErr := ""
			for _, hid := range tr.hostIDs {
				header, err := client.GetHostHeader(ctx, hid)
				if err != nil {
					var biz *upstream.BizError
					if errors.As(err, &biz) && biz.Status == 406 {
						// 主机不存在：开通早期上游可能尚未建 host 行，按瞬态处理继续轮询
						transientErr = fmt.Sprintf("host_id=%d: %s", hid, biz.Msg)
						continue
					}
					transientErr = fmt.Sprintf("host_id=%d 查询失败: %v", hid, err)
					continue
				}
				if fatalStatuses[header.DomainStatus] {
					fatal = fmt.Sprintf("上游主机状态为 %s（host_id=%d）", header.DomainStatus, hid)
					break
				}
				headers = append(headers, header)
			}
			if fatal != "" {
				tr.fatalMsg = fatal
				continue
			}
			if len(headers) == len(tr.hostIDs) && allActive(headers) {
				// 开通成功：回填 + hosts upsert + 下游登记
				s.completeItem(ctx, client, order, it, tr.hostIDs, headers)
				tr.done = true
				continue
			}
			allDone = false
			if transientErr != "" {
				slog.Debug("开通轮询瞬态错误（继续）", "order_no", order.OrderNo, "err", transientErr)
			}
		}
		// 全部条目到达终态（成功或致命失败）则提前结束
		finished := true
		for _, tr := range trackedItems {
			if !tr.done && tr.fatalMsg == "" {
				finished = false
				break
			}
		}
		if finished {
			allDone = true
		}
		if allDone {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(provisionPollInterval):
		}
	}
	// 收尾：未完成的条目按超时/致命原因置 failed
	for _, tr := range trackedItems {
		if tr.done {
			continue
		}
		it := &(*items)[tr.idx]
		reason := fmt.Sprintf("开通轮询超时（%d 次 × %s）：上游主机未到 Active，可在后台重试",
			provisionPollMaxRound, provisionPollInterval)
		if tr.fatalMsg != "" {
			reason = tr.fatalMsg
		}
		s.markItemFailed(ctx, it, reason)
	}
}

// allActive 判断全部主机是否 Active。
func allActive(headers []*upstream.HostHeader) bool {
	for _, h := range headers {
		if h.DomainStatus != model.HostStatusActive {
			return false
		}
	}
	return len(headers) > 0
}

// completeItem 开通成功收尾：回填 provision_info、逐台 upsert hosts、登记下游。
func (s *ProvisionService) completeItem(ctx context.Context, client *upstream.Client,
	order *model.Order, it *model.OrderItem, hostIDs []int64, headers []*upstream.HostHeader) {
	db := s.app.GetDB()
	info := loadProvisionInfo(it)
	first := headers[0]
	info.IP = first.DedicatedIP
	info.AssignedIPs = strings.Join(first.AssignedIPs, ",")
	info.Username = first.Username
	info.HostPassword = first.Password
	info.DomainStatus = first.DomainStatus
	info.UpstreamOrderNum = first.OrderNum
	if int64(first.NextDueDate) > 0 {
		info.NextDueDate = time.Unix(int64(first.NextDueDate), 0).Local().Format("2006-01-02 15:04:05")
	}
	info.ActivatedAt = time.Now().Local().Format("2006-01-02 15:04:05")
	info.Error = ""

	// hosts 表逐台 upsert（upstream_host_id 关联；product_id/client_id 回填），
	// 随后 setdownstream 把「派生 32 位推送密钥 + 本站回调地址 + 本地 host id」登记到上游
	//（生产环境上游后续变更会推送 host/sync；本机收不到推送，失败只记日志不阻断——任务约定）。
	for i, header := range headers {
		hostID := hostIDs[i]
		localID, err := s.upsertHost(ctx, hostID, header, it.ProductID, order.ClientID)
		if err != nil {
			slog.Error("开通成功但 hosts 落库失败（人工检查）", "host_id", hostID, "err", err)
			continue
		}
		s.registerDownstream(ctx, client, hostID, it.UpstreamPid, int64(localID))
	}

	it.ProvisionStatus = model.ProvisionActive
	s.saveInfo(ctx, it, info)
	if err := db.WithContext(ctx).Model(&model.OrderItem{}).Where("id = ?", it.ID).
		Update("provision_status", model.ProvisionActive).Error; err != nil {
		slog.Error("更新条目开通状态失败", "item_id", it.ID, "err", err)
	}
	slog.Info("条目开通成功", "order_no", order.OrderNo, "item_id", it.ID,
		"upstream_host_ids", hostIDs, "ip", info.IP)
	// TODO(批 4)：开通成功通知会员（notify 场景 provision_success 邮件/短信模板，
	// 本批模板未建，占位不发；bill_notify 账单场景本批同样不发）。
}

// upsertHost 按 host/header 数据 upsert 本地 hosts 表（范式同 hostsync：
// 更新时保留既有 product/client 关联；raw 存 header 快照）。返回本地 hosts.id。
func (s *ProvisionService) upsertHost(ctx context.Context, hostID int64,
	header *upstream.HostHeader, productID, clientID uint) (uint, error) {
	db := s.app.GetDB()
	rawJSON, _ := json.Marshal(header)
	raw := string(rawJSON)
	if len(raw) > 60000 {
		raw = raw[:60000]
	}
	nextDue := ""
	if int64(header.NextDueDate) > 0 {
		nextDue = time.Unix(int64(header.NextDueDate), 0).Local().Format("2006-01-02 15:04:05")
	}
	port := ""
	if int(header.Port) != 0 {
		port = strconv.Itoa(int(header.Port))
	}
	pid, cid := productID, clientID
	now := time.Now().Local()
	fields := map[string]any{
		"domain": header.Domain, "username": header.Username, "password": header.Password,
		"dedicated_ip": header.DedicatedIP, "assigned_ips": strings.Join(header.AssignedIPs, ","),
		"port": port, "os": header.OS, "next_due_date": nextDue,
		"domain_status": header.DomainStatus, "raw": raw, "synced_at": now,
	}
	var existing model.Host
	err := db.WithContext(ctx).Where("upstream_host_id = ?", hostID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row := model.Host{
			UpstreamHostID: hostID, ProductID: &pid, ClientID: &cid,
			Domain: header.Domain, Username: header.Username, Password: header.Password,
			DedicatedIP: header.DedicatedIP, AssignedIPs: strings.Join(header.AssignedIPs, ","),
			Port: port, OS: header.OS, NextDueDate: nextDue,
			DomainStatus: header.DomainStatus, Raw: raw, SyncedAt: now,
		}
		if e := db.WithContext(ctx).Create(&row).Error; e != nil {
			return 0, e
		}
		return row.ID, nil
	}
	if err != nil {
		return 0, err
	}
	// 更新时保留既有 product/client 关联（hostsync 同款语义）；缺失才回填
	if existing.ProductID == nil {
		fields["product_id"] = pid
	}
	if existing.ClientID == nil {
		fields["client_id"] = cid
	}
	if e := db.WithContext(ctx).Model(&existing).Updates(fields).Error; e != nil {
		return 0, e
	}
	return existing.ID, nil
}

// registerDownstream 把下游信息登记到上游主机（POST /host/setdownstream）。
// 失败只记日志不阻断（本机部署上游推不进来属预期；生产环境可后台手工登记兜底）。
func (s *ProvisionService) registerDownstream(ctx context.Context, client *upstream.Client,
	upstreamHostID, upstreamPid, localHostID int64) {
	db := s.app.GetDB()
	settings, err := model.GetSettingsMap(db.WithContext(ctx),
		model.SettingDownstreamToken, model.SettingSiteURL)
	if err != nil {
		slog.Warn("读取下游登记配置失败（跳过 setdownstream）", "err", err)
		return
	}
	token := upstream.PushToken32(settings[model.SettingDownstreamToken])
	if len(token) != 32 {
		slog.Warn("downstream_token 派生后不是 32 位（上游 strlen==32 硬校验），跳过登记；请先在后台生成密钥",
			"len", len(token))
		return
	}
	siteURL := strings.TrimSpace(settings[model.SettingSiteURL])
	if siteURL == "" {
		cfg := s.app.Cfg()
		addr := cfg.Server.Addr
		if addr == "" || addr == "0.0.0.0" {
			addr = "127.0.0.1"
		}
		siteURL = fmt.Sprintf("http://%s:%d", addr, cfg.Server.Port)
	}
	if err := client.SetDownstream(ctx, upstreamHostID, upstreamPid, localHostID, siteURL, token); err != nil {
		slog.Warn("下游登记失败（不阻断；生产环境请检查站点地址可达性或在上游后台手工登记）",
			"upstream_host_id", upstreamHostID, "err", err)
		return
	}
	slog.Info("下游登记成功", "upstream_host_id", upstreamHostID,
		"downstream_id", localHostID, "url", siteURL)
}

// ---- 终态与补偿 ----

// finalize 按条目终态推进订单状态：全部 active → Active；存在 failed → ProvisionFailed。
func (s *ProvisionService) finalize(ctx context.Context, order *model.Order, items *[]model.OrderItem) {
	db := s.app.GetDB()
	allActive, anyFailed := true, false
	for i := range *items {
		it := &(*items)[i]
		// 重读落库状态（pollItems 已就地更新内存态，这里双保险）
		var fresh model.OrderItem
		if err := db.WithContext(ctx).First(&fresh, it.ID).Error; err == nil {
			it.ProvisionStatus = fresh.ProvisionStatus
		}
		if it.ProvisionStatus != model.ProvisionActive {
			allActive = false
		}
		if it.ProvisionStatus == model.ProvisionFailed {
			anyFailed = true
		}
	}
	switch {
	case allActive:
		s.transitOrder(ctx, order, model.OrderStatusActive, "")
		slog.Info("订单开通完成", "order_no", order.OrderNo)
	case anyFailed:
		s.transitOrder(ctx, order, model.OrderStatusProvisionFailed, "")
	default:
		// 仍有 pending/provisioning（不应发生；保守转 ProvisionFailed 交人工）
		s.transitOrder(ctx, order, model.OrderStatusProvisionFailed, "开通流程未完整结束，转人工检查")
	}
}

// failOrder 网络性失败：条目置 failed、订单转 ProvisionFailed（款不动）；
// refunded=false 时不退款（资金冻结待人工），reason 如实进 remark 与日志。
func (s *ProvisionService) failOrder(ctx context.Context, order *model.Order,
	items *[]model.OrderItem, reason string, refunded bool) {
	for i := range *items {
		it := &(*items)[i]
		if it.ProvisionStatus == model.ProvisionPending || it.ProvisionStatus == model.ProvisionProvisioning {
			s.markItemFailed(ctx, it, reason)
		}
	}
	target := model.OrderStatusProvisionFailed
	note := "开通失败：" + reason
	if refunded {
		target = model.OrderStatusRefunded
	}
	s.transitOrder(ctx, order, target, note)
	slog.Error("订单开通失败", "order_no", order.OrderNo, "target", target, "reason", reason)
}

// refundOrder 上游明确业务拒绝的自动补偿：本地余额原路退回（refund 流水，同事务）
// → 订单 Refunded 终态；拒绝原因如实进 remark（不许吞）。
func (s *ProvisionService) refundOrder(ctx context.Context, order *model.Order,
	items *[]model.OrderItem, reason string) {
	db := s.app.GetDB()
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked, e := lockOrderForUpdate(ctx, tx, order.ID, 0)
		if e != nil {
			return e
		}
		if locked.Status != model.OrderStatusPaid {
			// 已被其它路径推进（如管理员取消退款）：不重复退款
			slog.Warn("退款跳过：订单状态已变更", "order_no", order.OrderNo, "status", locked.Status)
			order.Status = locked.Status
			return nil
		}
		refundCents, e := money.ParseCents(locked.BalanceUsed)
		if e != nil {
			return errcode.Internal("订单支付金额数据损坏").WithErr(e)
		}
		if refundCents > 0 {
			client, e := lockClientForUpdate(ctx, tx, locked.ClientID)
			if e != nil {
				return e
			}
			if _, e := creditBalanceTx(ctx, tx, client, refundCents,
				model.BalanceTypeRefund, model.BalanceRelatedOrder, locked.ID,
				"订单自动退款 "+locked.OrderNo+"（"+truncateTo(reason, 120)+"）", nil); e != nil {
				return e
			}
		}
		now := time.Now().Local()
		remark := appendRemark(locked.Remark, "上游拒绝订单，余额已自动原路退回："+reason)
		if e := tx.WithContext(ctx).Model(&model.Order{}).Where("id = ?", locked.ID).
			Updates(map[string]any{"status": model.OrderStatusRefunded,
				"remark": remark, "updated_at": now}).Error; e != nil {
			return errcode.Internal("更新订单退款状态失败").WithErr(e)
		}
		order.Status = model.OrderStatusRefunded
		order.Remark = remark
		return nil
	})
	if err != nil {
		slog.Error("自动退款失败（订单转 ProvisionFailed 待人工）", "order_no", order.OrderNo, "err", err)
		s.failOrder(ctx, order, items, reason+"；且自动退款失败:"+err.Error(), false)
		return
	}
	for i := range *items {
		it := &(*items)[i]
		if it.ProvisionStatus == model.ProvisionPending || it.ProvisionStatus == model.ProvisionProvisioning {
			s.markItemFailed(ctx, it, reason)
		}
	}
	slog.Error("上游拒绝订单，已自动退款", "order_no", order.OrderNo, "reason", reason)
}

// transitOrder 订单状态迁移（白名单校验；非法迁移拒绝并记日志——状态机硬约束）。
func (s *ProvisionService) transitOrder(ctx context.Context, order *model.Order, to, note string) {
	if order.Status == to {
		return
	}
	if !model.CanTransition(order.Status, to) {
		slog.Error("非法订单状态迁移被拒绝", "order_no", order.OrderNo,
			"from", order.Status, "to", to)
		return
	}
	db := s.app.GetDB()
	updates := map[string]any{"status": to, "updated_at": time.Now().Local()}
	if note != "" {
		updates["remark"] = appendRemark(order.Remark, note)
	}
	if err := db.WithContext(ctx).Model(&model.Order{}).
		Where("id = ? AND status = ?", order.ID, order.Status). // 乐观防并发双写
		Updates(updates).Error; err != nil {
		slog.Error("更新订单状态失败", "order_no", order.OrderNo, "to", to, "err", err)
		return
	}
	if note != "" {
		order.Remark = updates["remark"].(string)
	}
	order.Status = to
}

// markItemFailed 条目置 failed 并把原因写进 provision_info。
func (s *ProvisionService) markItemFailed(ctx context.Context, it *model.OrderItem, reason string) {
	db := s.app.GetDB()
	info := loadProvisionInfo(it)
	info.Error = reason
	it.ProvisionStatus = model.ProvisionFailed
	s.saveInfo(ctx, it, info)
	if err := db.WithContext(ctx).Model(&model.OrderItem{}).Where("id = ?", it.ID).
		Update("provision_status", model.ProvisionFailed).Error; err != nil {
		slog.Error("更新条目失败状态出错", "item_id", it.ID, "err", err)
	}
}

// ---- 定时任务 ----

// sweepLoop 未支付订单超时自动取消（每 5 分钟扫 Pending 超 24h，ADR-026）。
func (s *ProvisionService) sweepLoop() {
	s.sweepPending() // 启动先扫一次
	ticker := time.NewTicker(pendingSweepInterval)
	defer ticker.Stop()
	for range ticker.C {
		s.sweepPending()
	}
}

// sweepPending 执行一轮超时取消（未支付取消无退款——钱还没扣）。
func (s *ProvisionService) sweepPending() {
	db := s.app.GetDB()
	if db == nil {
		return
	}
	ctx := context.Background()
	cutoff := time.Now().Local().Add(-pendingOrderTimeout)
	now := time.Now().Local()
	res := db.WithContext(ctx).Model(&model.Order{}).
		Where("status = ? AND created_at < ?", model.OrderStatusPending, cutoff).
		Updates(map[string]any{"status": model.OrderStatusCancelled,
			"cancel_time": now, "updated_at": now,
			"remark": "超时未支付自动取消"})
	if res.Error != nil {
		slog.Error("超时订单自动取消失败", "err", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		slog.Info("超时未支付订单已自动取消", "count", res.RowsAffected)
	}
}

// resumeOnStartup 启动恢复扫描：重启前 Paid 的订单重新驱动开通
// （provisioning 条目继续轮询；pending 条目重新结算——崩溃窗口极小，
// 重复下单风险与对账 SOP 见 ADR-029）。延迟 3 秒等待依赖就绪。
func (s *ProvisionService) resumeOnStartup() {
	time.Sleep(3 * time.Second)
	db := s.app.GetDB()
	if db == nil {
		return
	}
	ctx := context.Background()
	var orders []model.Order
	if err := db.WithContext(ctx).
		Where("status = ?", model.OrderStatusPaid).Find(&orders).Error; err != nil {
		slog.Error("启动恢复扫描失败", "err", err)
		return
	}
	for i := range orders {
		slog.Info("启动恢复：重新驱动 Paid 订单开通", "order_no", orders[i].OrderNo)
		s.Trigger(orders[i].ID)
	}
}

// ---- 内部助手 ----

// upstreamClient 取上游会话客户端。
func (s *ProvisionService) upstreamClient(ctx context.Context) (*upstream.Client, error) {
	mgr := s.app.GetUpstream()
	if mgr == nil {
		return nil, errors.New("上游模块未就绪")
	}
	return mgr.Client(ctx)
}

// fetchCurrencyID 从 /cart/credit 取货币 ID（结算 currencyid 参数）。
func (s *ProvisionService) fetchCurrencyID(ctx context.Context, client *upstream.Client) (int64, error) {
	credit, err := client.GetCredit(ctx)
	if err != nil {
		return 0, err
	}
	id := int64(credit.Currency.ID)
	if id <= 0 {
		return 0, errors.New("上游未返回货币 ID（currency.id 缺失）")
	}
	return id, nil
}

// loadProvisionInfo 解析条目 provision_info（空/损坏容错为新对象）。
func loadProvisionInfo(it *model.OrderItem) *ProvisionInfo {
	info := &ProvisionInfo{}
	if strings.TrimSpace(it.ProvisionInfo) != "" {
		if err := json.Unmarshal([]byte(it.ProvisionInfo), info); err != nil {
			slog.Warn("provision_info 解析失败（重建）", "item_id", it.ID, "err", err)
			info = &ProvisionInfo{}
		}
	}
	return info
}

// saveInfo 持久化 provision_info（序列化失败只记日志——信息快照不阻断主流程）。
func (s *ProvisionService) saveInfo(ctx context.Context, it *model.OrderItem, info *ProvisionInfo) {
	b, err := json.Marshal(info)
	if err != nil {
		slog.Error("provision_info 序列化失败", "item_id", it.ID, "err", err)
		return
	}
	it.ProvisionInfo = string(b)
	db := s.app.GetDB()
	if err := db.WithContext(ctx).Model(&model.OrderItem{}).Where("id = ?", it.ID).
		Update("provision_info", it.ProvisionInfo).Error; err != nil {
		slog.Error("provision_info 落库失败", "item_id", it.ID, "err", err)
	}
}

// ---- 主机名/密码生成（规则来自上游商品配置 products.host / products.password） ----

// orderFormRules 是商品的主机名/密码输入规则（get_product_config 的 products 字段）。
type orderFormRules struct {
	HostRequired     bool
	HostPrefix       string
	HostMinLen       int
	PasswordRequired bool
}

// parseOrderFormRules 解析下单表单规则（products.host/password 是双重编码的 JSON 字符串；
// 解析失败容错为「不要求」——上游 settle 对缺省 host/password 有空值兜底）。
func parseOrderFormRules(configJSON json.RawMessage) orderFormRules {
	rules := orderFormRules{}
	if len(configJSON) == 0 {
		return rules
	}
	var parsed struct {
		Products struct {
			Host     string `json:"host"`
			Password string `json:"password"`
		} `json:"products"`
	}
	if err := json.Unmarshal(configJSON, &parsed); err != nil {
		return rules
	}
	var hostCfg struct {
		Show   string `json:"show"`
		Prefix string `json:"prefix"`
		Rule   struct {
			LenNum string `json:"len_num"`
		} `json:"rule"`
	}
	if err := json.Unmarshal([]byte(parsed.Products.Host), &hostCfg); err == nil {
		rules.HostRequired = hostCfg.Show == "1"
		rules.HostPrefix = hostCfg.Prefix
		rules.HostMinLen, _ = strconv.Atoi(hostCfg.Rule.LenNum)
	}
	var pwdCfg struct {
		Show string `json:"show"`
	}
	if err := json.Unmarshal([]byte(parsed.Products.Password), &pwdCfg); err == nil {
		rules.PasswordRequired = pwdCfg.Show == "1"
	}
	return rules
}

// generateHostname 生成主机名：前缀 + 随机大写字母数字（满足上游 len_num 最小长度；
// 大写+数字组合对 upper/num 规则最稳，lyew.com pid=1 实测通过加购校验）。
func generateHostname(rules orderFormRules) string {
	minLen := rules.HostMinLen
	if minLen < 8 {
		minLen = 8 // 兜底：前缀 + 至少 4 位随机
	}
	randLen := minLen - len(rules.HostPrefix)
	if randLen < 4 {
		randLen = 4
	}
	if randLen > 12 {
		randLen = 12
	}
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // 去 I/O/0/1 防人工抄录混淆
	return rules.HostPrefix + randomString(charset, randLen)
}

// generateHostPassword 生成主机密码：12 位，含大写+小写+数字（上游 checkHostPassword
// 对三类字符事实上强制要求，6-20 位；无特殊字符——special=0 规则兼容），
// 与 Lilt2026abc 同形（lyew.com 实测通过加购校验）。
func generateHostPassword() string {
	const upper = "ABCDEFGHJKLMNPQRSTUVWXYZ"
	const lower = "abcdefghijkmnpqrstuvwxyz"
	const digits = "23456789"
	const all = upper + lower + digits
	pw := []byte{
		upper[randInt(len(upper))],
		lower[randInt(len(lower))],
		digits[randInt(len(digits))],
		digits[randInt(len(digits))],
	}
	for i := 0; i < 8; i++ {
		pw = append(pw, all[randInt(len(all))])
	}
	// Fisher-Yates 洗牌（避免固定前缀模式）
	for i := len(pw) - 1; i > 0; i-- {
		j := randInt(i + 1)
		pw[i], pw[j] = pw[j], pw[i]
	}
	return string(pw)
}

// randomString 从字符集生成 n 位加密随机串。
func randomString(charset string, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = charset[randInt(len(charset))]
	}
	return string(b)
}

// randInt 返回 [0,n) 的加密随机整数。
func randInt(n int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		// crypto/rand 失败极罕见；退化为时间种子（可用性优先，密码学强度降级记日志）
		slog.Error("crypto/rand 失败，随机数退化为时间种子", "err", err)
		return int(time.Now().UnixNano() % int64(n))
	}
	return int(v.Int64())
}
