// Package model 定义 GORM 数据模型（表结构见 docs/api-contract.md 第 6 章）。
// 注意：MySQL 5.7 的 CHECK 约束不生效，一切业务校验在 Go 层完成。
package model

import "time"

// 管理员状态枚举（status 列）。
const (
	AdminStatusDisabled int8 = 0 // 禁用
	AdminStatusEnabled  int8 = 1 // 启用
)

// Admin 是管理后台管理员表 admins 的模型。
type Admin struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	Username     string     `gorm:"size:64;uniqueIndex;not null" json:"username"`
	PasswordHash string     `gorm:"size:100;not null" json:"-"` // bcrypt 哈希，永不外泄
	Email        string     `gorm:"size:128" json:"email"`
	RealName     string     `gorm:"size:64" json:"real_name"`
	Status       int8       `gorm:"not null;default:1" json:"status"` // 1=启用 0=禁用
	LastLoginAt  *time.Time `json:"last_login_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// TableName 固定表名（避免 GORM 默认复数化歧义）。
func (Admin) TableName() string { return "admins" }

// IsEnabled 判断账号是否可用。
func (a *Admin) IsEnabled() bool { return a.Status == AdminStatusEnabled }
