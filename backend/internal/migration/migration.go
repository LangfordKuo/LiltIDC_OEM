// Package migration 负责数据库连接、表结构迁移（GORM AutoMigrate）与种子数据。
//
// 迁移方案选型（AutoMigrate 而非版本化 SQL）的理由见 docs/architecture.md ADR-002：
//   - 幂等：只建缺失的表/列，重复启动安全；
//   - model 结构体是表结构的单一真源；
//   - 出现第一个破坏性变更需求时（批 2+），引入 schema_version 表 + 版本化 SQL 做增量迁移。
package migration

import (
	"fmt"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/config"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/model"
)

// Open 按配置连接 MySQL 并返回 GORM 句柄（带连接池与 Ping 验证）。
// 连接失败返回含底层原因的清晰错误，由调用方记录日志。
func Open(dbCfg config.DatabaseConfig) (*gorm.DB, error) {
	return OpenDSN(dbCfg.DSN())
}

// OpenDSN 用原始 DSN 连接（安装向导在建库后使用）。
func OpenDSN(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		// SQL 日志只记慢查询与错误，避免刷屏；release 下同样适用
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
		NowFunc: func() time.Time {
			return time.Now().Local()
		},
	})
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取底层连接失败: %w", err)
	}
	sqlDB.SetMaxOpenConns(50)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("数据库 Ping 失败: %w", err)
	}
	return db, nil
}

// AutoMigrate 建表（幂等：已存在的表只做增量列同步）。
func AutoMigrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&model.Admin{},
		&model.Setting{},
		&model.NotifyTemplate{},
	); err != nil {
		return fmt.Errorf("AutoMigrate 失败: %w", err)
	}
	return nil
}

// SeedSettings 种子写入 settings：仅当键不存在时写入（不覆盖后台已修改的值）。
func SeedSettings(db *gorm.DB, kvs map[string]string) error {
	for k, v := range kvs {
		_, exists, err := model.GetSetting(db, k)
		if err != nil {
			return fmt.Errorf("检查设置项 %s 失败: %w", k, err)
		}
		if exists {
			continue
		}
		if err := model.SetSetting(db, k, v); err != nil {
			return fmt.Errorf("写入设置项 %s 失败: %w", k, err)
		}
	}
	return nil
}

// SeedTemplates 种子写入默认通知模板：仅当 (scene, channel, lang) 不存在时写入。
func SeedTemplates(db *gorm.DB) error {
	for _, tpl := range DefaultTemplates() {
		var count int64
		if err := db.Model(&model.NotifyTemplate{}).
			Where("scene = ? AND channel = ? AND lang = ?", tpl.Scene, tpl.Channel, tpl.Lang).
			Count(&count).Error; err != nil {
			return fmt.Errorf("检查模板 %s/%s/%s 失败: %w", tpl.Scene, tpl.Channel, tpl.Lang, err)
		}
		if count > 0 {
			continue
		}
		row := tpl // 复制一份，避免 GORM 回写 ID 污染默认值切片
		if err := db.Create(&row).Error; err != nil {
			return fmt.Errorf("写入模板 %s/%s/%s 失败: %w", tpl.Scene, tpl.Channel, tpl.Lang, err)
		}
	}
	return nil
}

// DefaultTemplates 返回批 1 的默认通知模板集合（2 scene × 2 channel × 2 lang）。
// 占位符约定见 docs/api-contract.md 第 8.3 节。
func DefaultTemplates() []model.NotifyTemplate {
	return []model.NotifyTemplate{
		// ---- 注册验证码 ----
		{
			Scene: model.SceneRegisterVerify, Channel: model.ChannelSMS, Lang: model.LangZhCN,
			Content: "您的验证码是 {code}，10 分钟内有效，请勿泄露给他人。",
			Enabled: true,
		},
		{
			Scene: model.SceneRegisterVerify, Channel: model.ChannelSMS, Lang: model.LangEnUS,
			Content: "Your verification code is {code}, valid for 10 minutes. Do not share it with anyone.",
			Enabled: true,
		},
		{
			Scene: model.SceneRegisterVerify, Channel: model.ChannelEmail, Lang: model.LangZhCN,
			Subject: "【{site_name}】注册验证码",
			Content: "<p>您正在注册 {site_name} 账号。</p><p>您的验证码是：<b>{code}</b>，10 分钟内有效。</p><p>如非本人操作，请忽略本邮件。</p>",
			Enabled: true,
		},
		{
			Scene: model.SceneRegisterVerify, Channel: model.ChannelEmail, Lang: model.LangEnUS,
			Subject: "[{site_name}] Verification Code",
			Content: "<p>You are registering an account on {site_name}.</p><p>Your verification code is: <b>{code}</b>, valid for 10 minutes.</p><p>If this was not you, please ignore this email.</p>",
			Enabled: true,
		},
		// ---- 账单通知 ----
		{
			Scene: model.SceneBillNotify, Channel: model.ChannelSMS, Lang: model.LangZhCN,
			Content: "尊敬的 {username}，您的账单 {bill_no} 金额 {amount} 元将于 {due_date} 到期，请及时续费以免影响使用。",
			Enabled: true,
		},
		{
			Scene: model.SceneBillNotify, Channel: model.ChannelSMS, Lang: model.LangEnUS,
			Content: "Dear {username}, your bill {bill_no} ({amount}) is due on {due_date}. Please renew in time to avoid service interruption.",
			Enabled: true,
		},
		{
			Scene: model.SceneBillNotify, Channel: model.ChannelEmail, Lang: model.LangZhCN,
			Subject: "【{site_name}】账单到期提醒 #{bill_no}",
			Content: "<p>尊敬的 {username}：</p><p>您的账单 <b>#{bill_no}</b> 金额为 <b>{amount}</b> 元，将于 <b>{due_date}</b> 到期。</p><p>请登录 {site_name} 及时续费，以免影响服务使用。</p>",
			Enabled: true,
		},
		{
			Scene: model.SceneBillNotify, Channel: model.ChannelEmail, Lang: model.LangEnUS,
			Subject: "[{site_name}] Invoice #{bill_no} Due Reminder",
			Content: "<p>Dear {username},</p><p>Your invoice <b>#{bill_no}</b> of <b>{amount}</b> is due on <b>{due_date}</b>.</p><p>Please log in to {site_name} and renew in time to avoid service interruption.</p>",
			Enabled: true,
		},
	}
}
