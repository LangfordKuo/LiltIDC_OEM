package model

import "time"

// 余额流水类型枚举（balance_transactions.type，契约文档 14.4）。
const (
	BalanceTypeRechargeAdmin = "recharge_admin" // 管理员人工充值（正数）
	BalanceTypeConsume       = "consume"        // 订单余额支付扣减（负数）
	BalanceTypeRefund        = "refund"         // 退款回充（正数：settle 失败自动退 / 后台取消并退款）
	BalanceTypeAdjust        = "adjust"         // 管理员人工扣款/调整（负数为主）
)

// 流水关联对象类型（related_type）。
const (
	BalanceRelatedOrder = "order" // related_id = orders.id
)

// BalanceTransaction 是余额流水表 balance_transactions 的模型（契约文档 14.4）。
//
// Amount 带符号（充值/退款为正、扣减为负），BalanceAfter 是本次变动后的余额快照
// ——两者与 clients.credit 的变更在**同一事务**内写入（ADR-028），
// 任意时刻 SUM(amount) 应与 credit 对得上（人工调账也有流水，账实可审计）。
type BalanceTransaction struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	ClientID     uint      `gorm:"index;not null" json:"client_id"`
	Type         string    `gorm:"size:32;not null;index" json:"type"`
	Amount       string    `gorm:"type:decimal(12,2);not null" json:"amount"`        // 带符号
	BalanceAfter string    `gorm:"type:decimal(12,2);not null" json:"balance_after"` // 变动后余额快照
	RelatedType  string    `gorm:"size:32;not null;default:''" json:"related_type"`  // order / 空
	RelatedID    uint      `gorm:"not null;default:0" json:"related_id"`             // 关联对象 ID
	Remark       string    `gorm:"size:255" json:"remark"`
	AdminID      *uint     `json:"admin_id"` // 人工操作者（系统自动流水为 NULL）
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
}

// TableName 固定表名。
func (BalanceTransaction) TableName() string { return "balance_transactions" }
