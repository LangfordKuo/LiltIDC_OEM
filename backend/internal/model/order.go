package model

import "time"

// 订单状态枚举（orders.status，契约文档 13.4 状态机）。
//
//	Pending ──支付──▶ Paid ──全部开通──▶ Active（终态）
//	Pending ──超时/手动取消──▶ Cancelled（终态，未支付无退款）
//	Paid ──settle 明确业务拒绝──▶ Refunded（终态，本地余额已自动原路退回）
//	Paid ──开通超时/失败、settle 网络性失败──▶ ProvisionFailed（款未动，待人工）
//	ProvisionFailed ──重试开通成功──▶ Active
//	ProvisionFailed ──后台取消并退款──▶ Refunded
//	ProvisionFailed ──后台重试开通──▶ Paid（重新进入开通流程）
const (
	OrderStatusPending         = "Pending"         // 待支付
	OrderStatusPaid            = "Paid"            // 已支付（开通中）
	OrderStatusActive          = "Active"          // 已开通（终态）
	OrderStatusProvisionFailed = "ProvisionFailed" // 开通失败待处理
	OrderStatusCancelled       = "Cancelled"       // 已取消（终态）
	OrderStatusRefunded        = "Refunded"        // 已退款（终态）
)

// orderTransitions 是订单状态机迁移白名单（键=当前态，值=允许的目标态集合）。
// 白名单外的一切迁移都是非法的（服务层 CanTransition 强制，单测覆盖）。
var orderTransitions = map[string]map[string]bool{
	OrderStatusPending: {
		OrderStatusPaid:      true, // 余额支付成功
		OrderStatusCancelled: true, // 超时/手动取消（未支付，无退款）
	},
	OrderStatusPaid: {
		OrderStatusActive:          true, // 全部条目开通成功
		OrderStatusProvisionFailed: true, // 开通超时/失败或 settle 网络性失败
		OrderStatusRefunded:        true, // settle 明确业务拒绝 → 自动退款
	},
	OrderStatusProvisionFailed: {
		OrderStatusPaid:     true, // 后台重试开通
		OrderStatusActive:   true, // 重试后全部条目开通（轮询直接判定）
		OrderStatusRefunded: true, // 后台取消并退款
	},
	// Active / Cancelled / Refunded 是终态：无出边
}

// CanTransition 判断订单状态迁移是否合法（白名单制，未知状态一律拒绝）。
func CanTransition(from, to string) bool {
	return orderTransitions[from][to]
}

// 订单条目开通状态枚举（order_items.provision_status）。
const (
	ProvisionPending      = "pending"      // 未开始（等待转发上游）
	ProvisionProvisioning = "provisioning" // 上游已受理，轮询开通结果中
	ProvisionActive       = "active"       // 开通成功（已回填）
	ProvisionFailed       = "failed"       // 开通失败（超时/上游拒绝）
)

// 支付方式枚举（orders.pay_method；epay 预留批 4）。
const (
	PayMethodBalance = "balance"
	PayMethodEpay    = "epay" // 预留：易支付（批 4）
)

// Order 是订单表 orders 的模型（契约文档 13.6）。
//
// 金额列为 DECIMAL(12,2)、Go 侧字符串承载：运算一律经 money 包转整数分，
// 禁止 float64（ADR-027）。BalanceUsed 记录本单实际消耗的余额（退款不清零，
// 退款事实由 balance_transactions 的 refund 流水表达）。
type Order struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	OrderNo     string     `gorm:"size:32;uniqueIndex;not null" json:"order_no"` // LIO+日期+序号，人类可读
	ClientID    uint       `gorm:"index;not null" json:"client_id"`
	Status      string     `gorm:"size:20;not null;index;default:Pending" json:"status"`
	Amount      string     `gorm:"type:decimal(12,2);not null" json:"amount"`                    // 订单总额（成交快照）
	BalanceUsed string     `gorm:"type:decimal(12,2);not null;default:0.00" json:"balance_used"` // 余额支付金额
	PayMethod   string     `gorm:"size:20;not null;default:''" json:"pay_method"`                // balance（epay 预留）
	PayTime     *time.Time `json:"pay_time"`
	CancelTime  *time.Time `json:"cancel_time"`
	Remark      string     `gorm:"size:512" json:"remark"` // 备注/上游错误如实记录
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// TableName 固定表名。
func (Order) TableName() string { return "orders" }

// OrderItem 是订单条目表 order_items 的模型（本批每单一条目：单商品立即购买；
// 表结构按多条目设计，购物车留批 4，见契约 13.6）。
//
// UnitPrice 是成交售价快照（下单时从 products.sale_prices 复制，后续改价不影响历史订单）。
// UpstreamOrderID/UpstreamHostID 是开通回填字段：上游账单 ID（settle 的 invoiceid）
// 与上游主机 ID（apply_credit/host header 的 hostid；qty>1 时记录首台，全量在 ProvisionInfo）。
// ProvisionInfo 是开通信息 JSON 快照（hostname/password/ip/username/到期时间/错误原因等）。
type OrderItem struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	OrderID         uint      `gorm:"index;not null" json:"order_id"`
	ProductID       uint      `gorm:"not null" json:"product_id"`
	ProductName     string    `gorm:"size:255" json:"product_name"` // 商品名快照
	UpstreamPid     int64     `gorm:"not null" json:"upstream_pid"`
	Cycle           string    `gorm:"size:32;not null" json:"cycle"` // 周期键（monthly 等，与上游一致）
	Qty             int       `gorm:"not null;default:1" json:"qty"`
	UnitPrice       string    `gorm:"type:decimal(12,2);not null" json:"unit_price"` // 成交售价快照
	Amount          string    `gorm:"type:decimal(12,2);not null" json:"amount"`     // unit_price × qty
	UpstreamOrderID int64     `gorm:"not null;default:0" json:"upstream_order_id"`   // 上游账单 ID（settle 回填）
	UpstreamHostID  int64     `gorm:"not null;default:0" json:"upstream_host_id"`    // 上游主机 ID（首台）
	ProvisionStatus string    `gorm:"size:20;not null;index;default:pending" json:"provision_status"`
	ProvisionInfo   string    `gorm:"type:text" json:"provision_info"` // JSON 快照（见契约 13.6）
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// TableName 固定表名。
func (OrderItem) TableName() string { return "order_items" }
