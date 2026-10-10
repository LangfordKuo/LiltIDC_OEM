package model

import "time"

// Host 是主机表 hosts 的模型（批 2 最小集）。
//
// 数据来源：上游魔方财务开通/变更主机后 POST /api/upstream/host/sync 推送，
// 按 upstream_host_id upsert；raw 保留推送原文 JSON 备份（字段缺失容错、批 3 溯源）。
// product_id / client_id 批 3 接订单与会员体系后回填。
type Host struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	UpstreamHostID int64     `gorm:"uniqueIndex;not null" json:"upstream_host_id"` // 上游主机 ID（推送的 host_id）
	ProductID      *uint     `json:"product_id"`                                   // 本地商品关联（批 3 回填）
	ClientID       *uint     `json:"client_id"`                                    // 本地会员关联（批 3 回填）
	Domain         string    `gorm:"size:255" json:"domain"`
	Username       string    `gorm:"size:128" json:"username"`
	Password       string    `gorm:"size:255" json:"password"` // 上游推送为明文，原样保存（契约文档已注明）
	DedicatedIP    string    `gorm:"size:128" json:"dedicatedip"`
	AssignedIPs    string    `gorm:"size:512" json:"assignedips"`
	Port           string    `gorm:"size:16" json:"port"` // 上游可能推数字或字符串，统一存文本
	OS             string    `gorm:"size:128" json:"os"`
	OSUrl          string    `gorm:"size:512" json:"os_url"`
	NextDueDate    string    `gorm:"size:32" json:"nextduedate"` // 原样保存（上游日期文本）
	DomainStatus   string    `gorm:"size:32;index" json:"domainstatus"`
	Raw            string    `gorm:"type:text" json:"-"` // 推送原文 JSON 备份（不对外输出）
	SyncedAt       time.Time `json:"synced_at"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// TableName 固定表名。
func (Host) TableName() string { return "hosts" }

// 上游 domainstatus 枚举（/host/list 的 domainstatus 字典与推送字段共用）。
const (
	HostStatusPending   = "Pending"
	HostStatusActive    = "Active"
	HostStatusSuspended = "Suspended"
	HostStatusCancelled = "Cancelled"
	HostStatusFraud     = "Fraud"
	HostStatusDeleted   = "Deleted"
)
