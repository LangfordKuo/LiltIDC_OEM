package service

// 订单/余额域 MySQL 集成测试（批 3-β）。
//
// 运行方式：设置环境变量 LILTIDC_TEST_DSN 指向**独立测试库**后 go test；
// 未设置时整组 Skip（默认 go test ./... 不依赖数据库，保持纯单测可移植性）。
// 例（Git Bash）：
//
//	export LILTIDC_TEST_DSN='root:pw@tcp(127.0.0.1:3306)/liltidc_oem_test?charset=utf8mb4&parseTime=True&loc=Local'
//	go test ./internal/service/ -run Integration -v
//
// 覆盖（任务要求的单元级验证点）：
//   - 余额扣减并发安全：事务 + SELECT…FOR UPDATE 行锁，N 并发只允许余额覆盖的笔数成功，零透支；
//   - 同一订单并发支付恰好成功一次；
//   - 余额变更与流水同事务（笔数/金额/balance_after 链严格对账）；
//   - settle 上游明确业务拒绝 → 自动退款（refund 流水 + 余额复原 + 订单 Refunded）；
//   - settle 网络性失败 → ProvisionFailed 且款不动；
//   - 服务层状态机非法迁移拒绝（支付已支付单/取消已支付单/重试非失败单）。

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/config"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/migration"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/model"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/notify"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/app"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/errcode"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/money"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/upstream"
)

// setupIntegrationApp 连接测试库、幂等建表并清空业务数据，返回可用的 App。
func setupIntegrationApp(t *testing.T) (*app.App, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("LILTIDC_TEST_DSN")
	if dsn == "" {
		t.Skip("未设置 LILTIDC_TEST_DSN，跳过 MySQL 集成测试")
	}
	db, err := migration.OpenDSN(dsn)
	if err != nil {
		t.Fatalf("连接测试库失败: %v", err)
	}
	if err := migration.AutoMigrate(db); err != nil {
		t.Fatalf("测试库迁移失败: %v", err)
	}
	for _, table := range []string{"balance_transactions", "order_items", "orders",
		"hosts", "clients", "products", "product_groups", "settings"} {
		if err := db.Exec("TRUNCATE TABLE " + table).Error; err != nil {
			t.Fatalf("清空测试表 %s 失败: %v", table, err)
		}
	}
	cfg := &config.Config{}
	cfg.JWT.Secret = "integration-test-secret"
	cfg.JWT.ExpireHours = 24
	cfg.JWT.ClientExpireHours = 24
	cfg.Server.Addr = "127.0.0.1"
	cfg.Server.Port = 8080
	cfg.Install.Installed = true
	a := app.New(cfg, "", "test")
	a.DB.Store(db)
	a.Installed.Store(true)
	a.Notify.Store(notify.NewManager(db))
	a.Upstream.Store(upstream.NewSessionManager(db, 5*time.Second))
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	return a, db
}

// createTestClient 直接落库一个测试会员（余额 creditText）。
func createTestClient(t *testing.T, db *gorm.DB, username, creditText string) *model.Client {
	t.Helper()
	c := &model.Client{
		Username: username, PasswordHash: "x", Credit: creditText,
		Status: model.ClientStatusActive,
	}
	if err := db.Create(c).Error; err != nil {
		t.Fatalf("创建测试会员失败: %v", err)
	}
	return c
}

// createTestOrder 直接落库一张订单（含单条目；status 由调用方指定）。
func createTestOrder(t *testing.T, db *gorm.DB, clientID uint, orderNo, status, amount, balanceUsed string) *model.Order {
	t.Helper()
	o := &model.Order{
		OrderNo: orderNo, ClientID: clientID, Status: status,
		Amount: amount, BalanceUsed: balanceUsed,
	}
	if status == model.OrderStatusPaid {
		now := time.Now().Local()
		o.PayMethod = model.PayMethodBalance
		o.PayTime = &now
	}
	if err := db.Create(o).Error; err != nil {
		t.Fatalf("创建测试订单失败: %v", err)
	}
	it := &model.OrderItem{
		OrderID: o.ID, ProductID: 1, ProductName: "测试商品",
		UpstreamPid: 1, Cycle: "monthly", Qty: 1,
		UnitPrice: amount, Amount: amount,
		ProvisionStatus: model.ProvisionPending,
	}
	if err := db.Create(it).Error; err != nil {
		t.Fatalf("创建测试条目失败: %v", err)
	}
	return o
}

// waitForOrderStatus 轮询等待订单进入期望状态集合（开通 worker 是异步的）。
func waitForOrderStatus(t *testing.T, db *gorm.DB, orderID uint, want []string, timeout time.Duration) *model.Order {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var order model.Order
	for time.Now().Before(deadline) {
		if err := db.First(&order, orderID).Error; err == nil {
			for _, w := range want {
				if order.Status == w {
					return &order
				}
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("等待订单 %d 进入 %v 超时（当前 %s）", orderID, want, order.Status)
	return nil
}

// ---- 并发安全 ----

func TestIntegrationConcurrentBalanceDeduction(t *testing.T) {
	a, db := setupIntegrationApp(t)
	svc := NewOrder(a, nil) // provision=nil：支付只走本地事务，便于断言
	ctx := context.Background()

	// 余额 ¥100.00，10 张 ¥20.00 的待支付订单 → 并发支付，恰好 5 张成功，零透支
	client := createTestClient(t, db, "conc01", "100.00")
	orderIDs := make([]uint, 10)
	for i := range orderIDs {
		o := createTestOrder(t, db, client.ID, fmt.Sprintf("LIOCONC%04d", i+1),
			model.OrderStatusPending, "20.00", "0.00")
		orderIDs[i] = o.ID
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	success, insufficient, other := 0, 0, 0
	for _, oid := range orderIDs {
		wg.Add(1)
		go func(oid uint) {
			defer wg.Done()
			_, err := svc.Pay(ctx, client.ID, oid, &PayOrderRequest{Method: "balance"}, "zh-CN")
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				success++
			case isBizCode(err, errcode.CodeBalanceInsufficient):
				insufficient++
			default:
				other++
				t.Errorf("并发支付出现预期外错误: %v", err)
			}
		}(oid)
	}
	wg.Wait()

	if success != 5 {
		t.Errorf("应恰好 5 笔支付成功（100/20），得到 %d（不足 %d，其它 %d）", success, insufficient, other)
	}
	if insufficient != 5 {
		t.Errorf("应恰好 5 笔 balance_insufficient，得到 %d", insufficient)
	}
	// 终态余额 = 0.00，绝无负数（行锁串行化 + 锁内校验）
	var fresh model.Client
	if err := db.First(&fresh, client.ID).Error; err != nil {
		t.Fatalf("查询会员失败: %v", err)
	}
	if fresh.Credit != "0.00" {
		t.Errorf("终态余额应为 0.00（零透支），得到 %s", fresh.Credit)
	}
	// 流水对账：5 条 consume、合计 -100.00、balance_after 严格递减 80→0
	var txs []model.BalanceTransaction
	if err := db.Where("client_id = ? AND type = ?", client.ID, model.BalanceTypeConsume).
		Order("id ASC").Find(&txs).Error; err != nil {
		t.Fatalf("查询流水失败: %v", err)
	}
	if len(txs) != 5 {
		t.Fatalf("consume 流水应为 5 条（与成功笔数一致），得到 %d", len(txs))
	}
	sum := int64(0)
	wantAfters := []string{"80.00", "60.00", "40.00", "20.00", "0.00"}
	for i := range txs {
		c, err := money.ParseCents(txs[i].Amount)
		if err != nil {
			t.Fatalf("流水金额非法 %q: %v", txs[i].Amount, err)
		}
		if c != -2000 {
			t.Errorf("流水 %d 金额应为 -20.00，得到 %s", txs[i].ID, txs[i].Amount)
		}
		sum += c
		if txs[i].BalanceAfter != wantAfters[i] {
			t.Errorf("流水 %d balance_after = %s，期望 %s（行锁串行化后快照必须严格递减）",
				txs[i].ID, txs[i].BalanceAfter, wantAfters[i])
		}
		if txs[i].RelatedType != model.BalanceRelatedOrder || txs[i].RelatedID == 0 {
			t.Errorf("流水 %d 关联订单缺失: %+v", txs[i].ID, txs[i])
		}
	}
	if money.FormatCents(sum) != "-100.00" {
		t.Errorf("流水合计应为 -100.00，得到 %s", money.FormatCents(sum))
	}
}

func TestIntegrationConcurrentPaySameOrder(t *testing.T) {
	a, db := setupIntegrationApp(t)
	svc := NewOrder(a, nil)
	ctx := context.Background()
	client := createTestClient(t, db, "conc02", "100.00")
	o := createTestOrder(t, db, client.ID, "LIOSAME0001", model.OrderStatusPending, "20.00", "0.00")

	var wg sync.WaitGroup
	var mu sync.Mutex
	success := 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Pay(ctx, client.ID, o.ID, &PayOrderRequest{Method: "balance"}, "zh-CN")
			if err == nil {
				mu.Lock()
				success++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if success != 1 {
		t.Errorf("同一订单 20 并发支付应恰好成功 1 次，得到 %d", success)
	}
	var fresh model.Client
	_ = db.First(&fresh, client.ID)
	if fresh.Credit != "80.00" {
		t.Errorf("应只扣一次款（80.00），得到 %s", fresh.Credit)
	}
	var count int64
	_ = db.Model(&model.BalanceTransaction{}).
		Where("client_id = ? AND type = ?", client.ID, model.BalanceTypeConsume).Count(&count)
	if count != 1 {
		t.Errorf("consume 流水应恰好 1 条，得到 %d", count)
	}
}

// ---- 状态机（服务层拒绝非法迁移） ----

func TestIntegrationIllegalStateTransitions(t *testing.T) {
	a, db := setupIntegrationApp(t)
	svc := NewOrder(a, nil)
	ctx := context.Background()
	client := createTestClient(t, db, "state01", "100.00")
	paid := createTestOrder(t, db, client.ID, "LIOSTATE001", model.OrderStatusPaid, "20.00", "20.00")
	pending := createTestOrder(t, db, client.ID, "LIOSTATE002", model.OrderStatusPending, "10.00", "0.00")
	active := createTestOrder(t, db, client.ID, "LIOSTATE003", model.OrderStatusActive, "10.00", "10.00")

	// 支付已支付订单 → order_state_invalid
	if _, err := svc.Pay(ctx, client.ID, paid.ID, &PayOrderRequest{Method: "balance"}, "zh-CN"); !isBizCode(err, errcode.CodeOrderStateInvalid) {
		t.Errorf("支付 Paid 订单应拒绝 order_state_invalid，得到 %v", err)
	}
	// 取消已支付订单 → order_state_invalid（Paid 后不可取消）
	if _, err := svc.Cancel(ctx, client.ID, paid.ID, "zh-CN"); !isBizCode(err, errcode.CodeOrderStateInvalid) {
		t.Errorf("取消 Paid 订单应拒绝，得到 %v", err)
	}
	// 取消已开通订单 → 拒绝
	if _, err := svc.Cancel(ctx, client.ID, active.ID, "zh-CN"); !isBizCode(err, errcode.CodeOrderStateInvalid) {
		t.Errorf("取消 Active 订单应拒绝，得到 %v", err)
	}
	// 支付他人订单 → order_not_found（防枚举）
	other := createTestClient(t, db, "state02", "0.00")
	if _, err := svc.Pay(ctx, other.ID, pending.ID, &PayOrderRequest{Method: "balance"}, "zh-CN"); !isBizCode(err, errcode.CodeOrderNotFound) {
		t.Errorf("支付他人订单应按不存在处理，得到 %v", err)
	}
	// 后台重试非失败订单 → 拒绝
	if _, err := svc.AdminRetryProvision(ctx, pending.ID, "zh-CN"); !isBizCode(err, errcode.CodeOrderStateInvalid) {
		t.Errorf("重试 Pending 订单应拒绝，得到 %v", err)
	}
	// 后台取消退款非失败订单 → 拒绝
	if _, err := svc.AdminCancelRefund(ctx, 1, pending.ID, "", "zh-CN"); !isBizCode(err, errcode.CodeOrderStateInvalid) {
		t.Errorf("取消退款 Pending 订单应拒绝，得到 %v", err)
	}
	// 不支持的支付方式 → pay_method_unsupported
	if _, err := svc.Pay(ctx, client.ID, pending.ID, &PayOrderRequest{Method: "alipay"}, "zh-CN"); !isBizCode(err, errcode.CodePayMethodUnsupported) {
		t.Errorf("alipay 应拒绝 pay_method_unsupported，得到 %v", err)
	}
	// 余额不足 → balance_insufficient（订单保持 Pending 可再支付）
	poor := createTestClient(t, db, "state03", "1.00")
	big := createTestOrder(t, db, poor.ID, "LIOSTATE004", model.OrderStatusPending, "20.00", "0.00")
	if _, err := svc.Pay(ctx, poor.ID, big.ID, &PayOrderRequest{Method: "balance"}, "zh-CN"); !isBizCode(err, errcode.CodeBalanceInsufficient) {
		t.Errorf("余额不足应拒绝，得到 %v", err)
	}
	var untouched model.Order
	_ = db.First(&untouched, big.ID)
	if untouched.Status != model.OrderStatusPending {
		t.Errorf("支付失败订单应保持 Pending，得到 %s", untouched.Status)
	}
	var freshPoor model.Client
	_ = db.First(&freshPoor, poor.ID)
	if freshPoor.Credit != "1.00" {
		t.Errorf("支付失败不应扣款，得到 %s", freshPoor.Credit)
	}
}

// ---- 上游失败补偿路径 ----

// newMockUpstream 构造 mock 魔方上游并把凭据写进 settings。
func newMockUpstream(t *testing.T, db *gorm.DB, settleHandler func(w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/zjmf_api_login":
			_, _ = io.WriteString(w, `{"jwt":"mock-jwt","status":200,"msg":"鉴权成功"}`)
		case r.URL.Path == "/cart/credit":
			_, _ = io.WriteString(w, `{"status":200,"msg":"请求成功","data":{"credit":"57664.50","currency":{"id":1,"code":"CNY","prefix":"¥","suffix":"元"}}}`)
		case r.URL.Path == "/cart/get_product_config":
			_, _ = io.WriteString(w, `{"status":200,"data":{"config_groups":[],"products":{"host":"{\"show\":\"1\",\"prefix\":\"HK-\",\"rule\":{\"len_num\":\"4\"}}","password":"{\"show\":\"1\"}"}}}`)
		case r.URL.Path == "/cart/add_to_shop":
			_, _ = io.WriteString(w, `{"status":200,"msg":"添加成功"}`)
		case r.URL.Path == "/cart/settle":
			settleHandler(w)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"status":404,"msg":"not found"}`)
		}
	}))
	if err := model.SetSettingsMap(db, map[string]string{
		model.SettingUpstreamHostname: srv.URL,
		model.SettingUpstreamUsername: "mock-user",
		model.SettingUpstreamAPIKey:   "mock-key",
		model.SettingDownstreamToken:  "0b8b187724fae820eb7f9938862807bd7894715a1b2c3d4e5f60718293a4b5c",
	}); err != nil {
		t.Fatalf("写入 mock 上游凭据失败: %v", err)
	}
	t.Cleanup(srv.Close)
	return srv
}

func TestIntegrationSettleBizRejectAutoRefund(t *testing.T) {
	a, db := setupIntegrationApp(t)
	ctx := context.Background()
	// 上游 settle 明确业务拒绝（商品下架）→ 必须自动退款 + Refunded + 拒绝原因如实入库
	newMockUpstream(t, db, func(w http.ResponseWriter) {
		_, _ = io.WriteString(w, `{"status":406,"msg":"该商品已下架停售","is_aff":"1"}`)
	})
	client := createTestClient(t, db, "refund01", "100.00")
	orderSvc := NewOrder(a, nil)
	o := createTestOrder(t, db, client.ID, "LIOREFUND001", model.OrderStatusPending, "20.00", "0.00")
	if _, err := orderSvc.Pay(ctx, client.ID, o.ID, &PayOrderRequest{Method: "balance"}, "zh-CN"); err != nil {
		t.Fatalf("支付失败: %v", err)
	}

	prov := NewProvision(a)
	prov.Trigger(o.ID)
	final := waitForOrderStatus(t, db, o.ID,
		[]string{model.OrderStatusRefunded, model.OrderStatusProvisionFailed}, 20*time.Second)

	if final.Status != model.OrderStatusRefunded {
		t.Fatalf("业务拒绝应自动退款转 Refunded，得到 %s（remark=%s）", final.Status, final.Remark)
	}
	if !strings.Contains(final.Remark, "该商品已下架停售") {
		t.Errorf("拒绝原因必须如实记录（不许吞）: %q", final.Remark)
	}
	// 余额复原 + refund 流水
	var fresh model.Client
	_ = db.First(&fresh, client.ID)
	if fresh.Credit != "100.00" {
		t.Errorf("余额应原路退回 100.00，得到 %s", fresh.Credit)
	}
	var txs []model.BalanceTransaction
	_ = db.Where("client_id = ?", client.ID).Order("id ASC").Find(&txs)
	if len(txs) != 2 || txs[0].Type != model.BalanceTypeConsume || txs[1].Type != model.BalanceTypeRefund {
		t.Fatalf("应有 consume+refund 两条流水，得到 %+v", txs)
	}
	if txs[0].Amount != "-20.00" || txs[1].Amount != "20.00" {
		t.Errorf("流水金额错误: %s / %s", txs[0].Amount, txs[1].Amount)
	}
	if txs[1].BalanceAfter != "100.00" || txs[1].RelatedID != o.ID {
		t.Errorf("refund 流水快照/关联错误: %+v", txs[1])
	}
	// 条目 failed 且 provision_info 记录原因
	var item model.OrderItem
	_ = db.Where("order_id = ?", o.ID).First(&item)
	if item.ProvisionStatus != model.ProvisionFailed {
		t.Errorf("条目应为 failed，得到 %s", item.ProvisionStatus)
	}
	if !strings.Contains(item.ProvisionInfo, "该商品已下架停售") {
		t.Errorf("条目 provision_info 应含拒绝原因: %s", item.ProvisionInfo)
	}
}

func TestIntegrationSettleNetworkErrorKeepsMoney(t *testing.T) {
	a, db := setupIntegrationApp(t)
	ctx := context.Background()
	// 上游 settle 返回 HTTP 500（网络性未知错误）→ 款不动、订单 ProvisionFailed、无退款流水
	newMockUpstream(t, db, func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `<html>gateway boom</html>`)
	})
	client := createTestClient(t, db, "neterr01", "100.00")
	orderSvc := NewOrder(a, nil)
	o := createTestOrder(t, db, client.ID, "LIONETERR001", model.OrderStatusPending, "20.00", "0.00")
	if _, err := orderSvc.Pay(ctx, client.ID, o.ID, &PayOrderRequest{Method: "balance"}, "zh-CN"); err != nil {
		t.Fatalf("支付失败: %v", err)
	}
	prov := NewProvision(a)
	prov.Trigger(o.ID)
	final := waitForOrderStatus(t, db, o.ID,
		[]string{model.OrderStatusProvisionFailed, model.OrderStatusRefunded}, 20*time.Second)
	if final.Status != model.OrderStatusProvisionFailed {
		t.Fatalf("网络性失败应转 ProvisionFailed（冻结资金待人工），得到 %s", final.Status)
	}
	var fresh model.Client
	_ = db.First(&fresh, client.ID)
	if fresh.Credit != "80.00" {
		t.Errorf("网络性失败不应退款（上游可能已受理），余额应仍为 80.00，得到 %s", fresh.Credit)
	}
	var refundCount int64
	_ = db.Model(&model.BalanceTransaction{}).
		Where("client_id = ? AND type = ?", client.ID, model.BalanceTypeRefund).Count(&refundCount)
	if refundCount != 0 {
		t.Errorf("不应产生退款流水，得到 %d 条", refundCount)
	}
	if !strings.Contains(final.Remark, "网络性错误") {
		t.Errorf("失败原因应如实记录: %q", final.Remark)
	}
	// 后台取消并退款：ProvisionFailed → Refunded + 余额复原
	orderSvc2 := NewOrder(a, nil)
	if _, err := orderSvc2.AdminCancelRefund(ctx, 1, o.ID, "人工确认上游未受理", "zh-CN"); err != nil {
		t.Fatalf("后台取消并退款失败: %v", err)
	}
	_ = db.First(&fresh, client.ID)
	if fresh.Credit != "100.00" {
		t.Errorf("取消退款后余额应复原 100.00，得到 %s", fresh.Credit)
	}
	var refunded model.Order
	_ = db.First(&refunded, o.ID)
	if refunded.Status != model.OrderStatusRefunded {
		t.Errorf("订单应为 Refunded，得到 %s", refunded.Status)
	}
	if !strings.Contains(refunded.Remark, "人工确认上游未受理") {
		t.Errorf("管理员备注应入库: %q", refunded.Remark)
	}
	// Refunded 终态再退款 → 状态机拒绝（防双重退款）
	if _, err := orderSvc2.AdminCancelRefund(ctx, 1, o.ID, "", "zh-CN"); !isBizCode(err, errcode.CodeOrderStateInvalid) {
		t.Errorf("终态重复退款应被状态机拒绝，得到 %v", err)
	}
}

// ---- 后台人工调账 ----

func TestIntegrationAdminAdjust(t *testing.T) {
	a, db := setupIntegrationApp(t)
	ctx := context.Background()
	svc := NewBalance(a)
	client := createTestClient(t, db, "adjust01", "10.00")

	// 充值 +50.50
	view, err := svc.AdminAdjust(ctx, 1, client.ID, &AdminAdjustRequest{Amount: "+50.50", Remark: "银行转账"}, "zh-CN")
	if err != nil {
		t.Fatalf("充值失败: %v", err)
	}
	if view.Credit != "60.50" || view.Transaction.Type != model.BalanceTypeRechargeAdmin ||
		view.Transaction.Amount != "50.50" || view.Transaction.BalanceAfter != "60.50" {
		t.Errorf("充值结果错误: %+v", view)
	}
	// 扣款 -70.50 → 余额不足拒绝
	if _, err := svc.AdminAdjust(ctx, 1, client.ID, &AdminAdjustRequest{Amount: "-70.50"}, "zh-CN"); !isBizCode(err, errcode.CodeBalanceInsufficient) {
		t.Errorf("扣穿余额应拒绝，得到 %v", err)
	}
	// 扣款 -0.50 → adjust 流水
	view, err = svc.AdminAdjust(ctx, 1, client.ID, &AdminAdjustRequest{Amount: "-0.50", Remark: "手续费"}, "zh-CN")
	if err != nil || view.Credit != "60.00" || view.Transaction.Type != model.BalanceTypeAdjust {
		t.Errorf("扣款结果错误: %+v err=%v", view, err)
	}
	// 非法输入
	for _, bad := range []string{"", "abc", "0", "12.345", "+0.00"} {
		if _, err := svc.AdminAdjust(ctx, 1, client.ID, &AdminAdjustRequest{Amount: bad}, "zh-CN"); err == nil {
			t.Errorf("非法金额 %q 应被拒绝", bad)
		}
	}
	// 会员不存在
	if _, err := svc.AdminAdjust(ctx, 1, 99999, &AdminAdjustRequest{Amount: "1"}, "zh-CN"); !isBizCode(err, errcode.CodeClientNotFound) {
		t.Errorf("不存在会员应 404，得到 %v", err)
	}
	// 流水总账对账：recharge(+50.50) + adjust(-0.50)，balance_after 链一致
	var txs []model.BalanceTransaction
	_ = db.Where("client_id = ?", client.ID).Order("id ASC").Find(&txs)
	if len(txs) != 2 || txs[0].AdminID == nil || *txs[0].AdminID != 1 {
		t.Fatalf("流水/操作者记录错误: %+v", txs)
	}
	var fresh model.Client
	_ = db.First(&fresh, client.ID)
	if fresh.Credit != txs[len(txs)-1].BalanceAfter {
		t.Errorf("账实不符: credit=%s 最后流水 balance_after=%s", fresh.Credit, txs[len(txs)-1].BalanceAfter)
	}
}

// isBizCode 判断错误是否为指定业务码。
func isBizCode(err error, code string) bool {
	var biz *errcode.BizError
	if errors.As(err, &biz) {
		return biz.Code == code
	}
	return false
}
