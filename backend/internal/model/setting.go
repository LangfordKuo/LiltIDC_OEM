package model

import (
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Setting 是通用 key/value 设置表 settings 的模型。
// 品牌信息与通知通道配置的运行时真源（config.toml 仅为安装时种子，见 ADR-009）。
type Setting struct {
	Key       string    `gorm:"column:key;primaryKey;size:128" json:"key"` // key 是 MySQL 保留字，GORM 会自动反引号包裹
	Value     string    `gorm:"type:text" json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 固定表名。
func (Setting) TableName() string { return "settings" }

// 设置键常量（批 1 使用的全部键，见契约文档 6.2）。
const (
	SettingSiteName     = "site_name"
	SettingLogo         = "logo"
	SettingPrimaryColor = "primary_color"

	SettingSmsbaoUsername    = "smsbao_username"
	SettingSmsbaoPasswordMD5 = "smsbao_password_md5"
	SettingSmsbaoSign        = "smsbao_sign"

	SettingSMTPHost     = "smtp_host"
	SettingSMTPPort     = "smtp_port"
	SettingSMTPUsername = "smtp_username"
	SettingSMTPPassword = "smtp_password"
	SettingSMTPFrom     = "smtp_from"
	SettingSMTPTLS      = "smtp_tls"

	// ---- 批 2：上游对接（运行时真源，config.toml [upstream] 仅种子，见 ADR-019） ----
	SettingUpstreamHostname = "upstream_hostname" // 上游站点地址，如 https://lyew.com
	SettingUpstreamUsername = "upstream_username" // 上游账号（手机号不加区号）
	SettingUpstreamAPIKey   = "upstream_api_key"  // 上游 API 密钥（登录密码）
	SettingDownstreamToken  = "downstream_token"  // 下游密钥：上游 host/sync 推送验签用
)

// GetSetting 读取单个设置；键不存在时返回 ("", false, nil)。
func GetSetting(db *gorm.DB, key string) (string, bool, error) {
	var s Setting
	err := db.Where("`key` = ?", key).First(&s).Error
	if err == gorm.ErrRecordNotFound {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return s.Value, true, nil
}

// SetSetting 写入（upsert）单个设置。
func SetSetting(db *gorm.DB, key, value string) error {
	row := Setting{Key: key, Value: value, UpdatedAt: time.Now()}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&row).Error
}

// GetSettingsMap 批量读取设置，返回 key→value 映射（缺失的键不在 map 中）。
func GetSettingsMap(db *gorm.DB, keys ...string) (map[string]string, error) {
	var rows []Setting
	q := db
	if len(keys) > 0 {
		q = db.Where("`key` IN ?", keys)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	m := make(map[string]string, len(rows))
	for _, r := range rows {
		m[r.Key] = r.Value
	}
	return m, nil
}

// SetSettingsMap 批量 upsert 设置。
func SetSettingsMap(db *gorm.DB, kvs map[string]string) error {
	now := time.Now()
	rows := make([]Setting, 0, len(kvs))
	for k, v := range kvs {
		rows = append(rows, Setting{Key: k, Value: v, UpdatedAt: now})
	}
	if len(rows) == 0 {
		return nil
	}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&rows).Error
}

// GetSettingInt 读取整型设置；键不存在或解析失败返回 fallback。
func GetSettingInt(db *gorm.DB, key string, fallback int) int {
	v, ok, err := GetSetting(db, key)
	if err != nil || !ok {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

// GetSettingBool 读取布尔设置；键不存在或解析失败返回 fallback。
func GetSettingBool(db *gorm.DB, key string, fallback bool) bool {
	v, ok, err := GetSetting(db, key)
	if err != nil || !ok {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
