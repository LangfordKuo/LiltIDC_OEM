package model

import (
	"time"

	"gorm.io/gorm"
)

// 会员状态枚举（clients.status，契约文档 12.7）。
const (
	ClientStatusDisabled      int8 = 0 // 禁用（不可登录）
	ClientStatusActive        int8 = 1 // 激活（注册默认）
	ClientStatusDeleted       int8 = 2 // 软删除（列表默认隐藏，唯一字段已加后缀释放）
	ClientStatusUpstreamIssue int8 = 3 // 上游关联异常（批 3-β 回填；可登录，仅列表提示）
)

// Client 是会员表 clients 的模型（独立于管理员 admins）。
//
// email/phone 可空（NULL）：MySQL 唯一索引允许多行 NULL，与「可空但占用查重」语义吻合。
// 软删除时 status=2 + deleted_at，且 username/email/phone 追加 "#del<id>" 后缀
// 释放唯一索引（同名可重新注册），见契约 12.6。
type Client struct {
	ID           uint    `gorm:"primaryKey" json:"id"`
	Username     string  `gorm:"size:64;uniqueIndex;not null" json:"username"`
	PasswordHash string  `gorm:"size:100;not null" json:"-"` // bcrypt，永不外泄
	Email        *string `gorm:"size:128;uniqueIndex" json:"email"`
	Phone        *string `gorm:"size:20;uniqueIndex" json:"phone"`
	RealName     string  `gorm:"size:64" json:"real_name"`
	Company      string  `gorm:"size:128" json:"company"`
	Address      string  `gorm:"size:255" json:"address"`
	// Credit 是会员余额（批 3-β）。DECIMAL(12,2) 列 + Go 字符串承载：
	// 运算一律经 money 包转整数分，禁止 float64（取舍见 ADR-027）。
	Credit           string     `gorm:"type:decimal(12,2);not null;default:0.00" json:"credit"`
	Status           int8       `gorm:"not null;default:1;index" json:"status"`
	UpstreamUID      *int64     `json:"upstream_uid"`       // 上游会员 ID（批 3-β 开户回填）
	UpstreamLinkedAt *time.Time `json:"upstream_linked_at"` // 上游关联时间
	LastLoginAt      *time.Time `json:"last_login_at"`
	DeletedAt        *time.Time `json:"deleted_at"` // 软删除时间（不用 gorm.DeletedAt，避免全局 scope 影响批 1/2 查询习惯）
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// TableName 固定表名。
func (Client) TableName() string { return "clients" }

// BeforeCreate 落库前把空 Credit 归一为 "0.00"：
// MySQL 严格模式下空串写入 DECIMAL 列会报 1366，注册等创建路径不必逐一记得填零值。
func (c *Client) BeforeCreate(tx *gorm.DB) error {
	if c.Credit == "" {
		c.Credit = "0.00"
	}
	return nil
}

// CanLogin 判断会员当前状态是否允许登录（禁用/删除不可；上游异常可登录）。
func (c *Client) CanLogin() bool {
	return c.Status == ClientStatusActive || c.Status == ClientStatusUpstreamIssue
}

// IsDeleted 判断是否已软删除。
func (c *Client) IsDeleted() bool {
	return c.Status == ClientStatusDeleted
}
