package model

import "time"

// scene 枚举（见契约文档 8.1；批 1 只建模板，不触发业务发送）。
const (
	SceneRegisterVerify = "register_verify" // 注册验证码
	SceneBillNotify     = "bill_notify"     // 账单通知
)

// channel 枚举。
const (
	ChannelSMS   = "sms"
	ChannelEmail = "email"
)

// lang 枚举。
const (
	LangZhCN = "zh-CN"
	LangEnUS = "en-US"
)

// NotifyTemplate 是通知模板表 notify_templates 的模型。
// (scene, channel, lang) 三元组唯一。
type NotifyTemplate struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Scene     string    `gorm:"size:64;not null;uniqueIndex:idx_scene_channel_lang,priority:1" json:"scene"`
	Channel   string    `gorm:"size:16;not null;uniqueIndex:idx_scene_channel_lang,priority:2" json:"channel"`
	Lang      string    `gorm:"size:16;not null;uniqueIndex:idx_scene_channel_lang,priority:3" json:"lang"`
	Subject   string    `gorm:"size:255" json:"subject"` // 邮件主题（短信忽略）
	Content   string    `gorm:"type:text;not null" json:"content"`
	Enabled   bool      `gorm:"not null;default:true" json:"enabled"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 固定表名。
func (NotifyTemplate) TableName() string { return "notify_templates" }

// ValidScenes 是全部合法 scene（参数校验用）。
var ValidScenes = []string{SceneRegisterVerify, SceneBillNotify}

// ValidChannels 是全部合法 channel。
var ValidChannels = []string{ChannelSMS, ChannelEmail}

// ValidLangs 是全部合法 lang。
var ValidLangs = []string{LangZhCN, LangEnUS}
