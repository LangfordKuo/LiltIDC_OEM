package model

import (
	"encoding/json"
	"fmt"
	"time"
)

// ProductGroup 是本地商品分组表 product_groups 的模型。
type ProductGroup struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:128;uniqueIndex;not null" json:"name"` // 名称唯一：导入时按名 FirstOrCreate
	Sort      int       `gorm:"not null;default:0" json:"sort"`
	Status    int8      `gorm:"not null;default:1" json:"status"` // 1=启用 0=停用
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 固定表名。
func (ProductGroup) TableName() string { return "product_groups" }

// Product 是本地商品表 products 的模型（从上游导入 + 自定义售价）。
//
// upstream_pid 唯一索引：一个上游商品至多映射一个本地商品，
// 重复导入 = 更新映射与售价（幂等），见 ADR-022。
//
// 价格字段存 JSON 文本（map[周期]价格字符串）而非 MySQL JSON 类型：
// MySQL 5.7 JSON 支持有限（契约文档第 6 章约定），金额用字符串避免浮点精度问题。
type Product struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	GroupID      uint      `gorm:"index;not null" json:"group_id"`
	UpstreamPid  int64     `gorm:"uniqueIndex;not null" json:"upstream_pid"`
	Name         string    `gorm:"size:255;not null" json:"name"`
	Description  string    `gorm:"type:text" json:"description"` // HTML（已做实体解码）
	Type         string    `gorm:"size:64" json:"type"`          // 上游商品 type 原样
	CostPrices   string    `gorm:"type:text" json:"cost_prices"` // PriceMap JSON：上游成本价
	SalePrices   string    `gorm:"type:text" json:"sale_prices"` // PriceMap JSON：自定义售价
	StockQty     int       `gorm:"not null;default:0" json:"stock_qty"`
	StockControl bool      `gorm:"not null;default:false" json:"stock_control"` // 上游是否启用库存控制
	Status       int8      `gorm:"not null;default:1" json:"status"`            // 1=上架 0=下架
	Sort         int       `gorm:"not null;default:0" json:"sort"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TableName 固定表名。
func (Product) TableName() string { return "products" }

// 商品状态枚举（status 列）。
const (
	ProductStatusOff int8 = 0 // 下架
	ProductStatusOn  int8 = 1 // 上架
)

// BillingCycles 是计费周期枚举的规范顺序，与上游 product_pricings 的周期键
// **完全一致**（lyew.com 实测 17 键，含 semiannually 无下划线形态——本地规范键
// 统一采用上游键名，免映射，见 ADR-022）。解析时不依赖此表（白名单在
// upstream.CyclePriceKeys），此表仅用于展示排序与文档。
var BillingCycles = []string{
	"onetime",      // 一次性
	"hour",         // 时付
	"day",          // 日付
	"ontrial",      // 试用
	"monthly",      // 月付
	"quarterly",    // 季付
	"semiannually", // 半年付（上游键无下划线）
	"annually",     // 年付
	"biennially",   // 两年付
	"triennially",  // 三年付
	"fourly",       // 四年付
	"fively",       // 五年付
	"sixly",        // 六年付
	"sevenly",      // 七年付
	"eightly",      // 八年付
	"ninely",       // 九年付
	"tenly",        // 十年付
}

// PriceMap 是 周期→价格 映射；价格用字符串（十进制金额文本）避免浮点误差。
type PriceMap map[string]string

// Marshal 序列化为紧凑 JSON 文本（存库用）；空映射存 "{}"。
func (p PriceMap) Marshal() string {
	if len(p) == 0 {
		return "{}"
	}
	b, err := json.Marshal(p)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// ParsePriceMap 解析存库的 JSON 文本；空串/非法 JSON 返回空映射与错误。
func ParsePriceMap(s string) (PriceMap, error) {
	if s == "" {
		return PriceMap{}, nil
	}
	m := PriceMap{}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, fmt.Errorf("价格 JSON 解析失败: %w", err)
	}
	return m, nil
}

// SortedCycles 按 BillingCycles 规范顺序返回周期键（未知周期排在末尾、按字母序），
// 供 API 输出与前端展示稳定排序。
func (p PriceMap) SortedCycles() []string {
	known := make(map[string]int, len(BillingCycles))
	for i, c := range BillingCycles {
		known[c] = i
	}
	cycles := make([]string, 0, len(p))
	for c := range p {
		cycles = append(cycles, c)
	}
	// 简单插入排序（周期数 <= 10，无需 sort 包的复杂度）
	for i := 1; i < len(cycles); i++ {
		for j := i; j > 0; j-- {
			ki, oki := known[cycles[j]]
			kp, okp := known[cycles[j-1]]
			// 都在规范表：按表序；只有一个在：在表内的靠前；都不在：字母序
			less := false
			switch {
			case oki && okp:
				less = ki < kp
			case oki && !okp:
				less = true
			case !oki && okp:
				less = false
			default:
				less = cycles[j] < cycles[j-1]
			}
			if less {
				cycles[j], cycles[j-1] = cycles[j-1], cycles[j]
			}
		}
	}
	return cycles
}
