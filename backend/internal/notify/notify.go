// Package notify 是短信/邮件通知基础设施层。
//
// 设计（见 docs/architecture.md ADR-009）：
//   - Sender 是通道抽象接口，批 1 提供 Smsbao（短信宝）与 SMTP 两个实现；
//   - Manager 是唯一业务入口：Send(scene, channel, lang, receiver, vars)
//     加载模板 → 渲染占位符 → 从 settings 表实时构造 provider → 发送；
//   - SendRaw 绕过模板直发（后台"发送测试"使用）；
//   - provider 配置每次发送实时读库，后台保存立即生效，无需重启；
//   - 批 1 不接业务触发，批 2+ 注入 *Manager 直接调用 Send 即可。
package notify

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"gorm.io/gorm"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/model"
)

// Message 是一次发送的载荷。
type Message struct {
	To      string // 手机号（sms）或邮箱地址（email）
	Subject string // 邮件主题（sms 忽略）
	Content string // 已渲染的最终内容（短信不含签名，由 provider 自动加【签名】前缀）
}

// Sender 是通知通道统一接口。
type Sender interface {
	Send(ctx context.Context, msg *Message) error
}

// 语义化哨兵错误：service 层用 errors.Is 映射为契约错误码。
var (
	ErrNotConfigured      = errors.New("通道未配置（请先在后台填写账号信息）")
	ErrTemplateNotFound   = errors.New("通知模板不存在")
	ErrTemplateDisabled   = errors.New("通知模板已停用")
	ErrUnsupportedChannel = errors.New("不支持的通知通道")
)

// Manager 负责模板加载、渲染与 provider 调度。
type Manager struct {
	db *gorm.DB
}

// NewManager 构造 Manager。
func NewManager(db *gorm.DB) *Manager {
	return &Manager{db: db}
}

// Send 是业务统一入口：按 场景+通道+语言 加载模板 → 渲染占位符 → 发送。
// vars 为占位符键值对（见契约文档 8.3）；{site_name} 未提供时自动注入。
func (m *Manager) Send(ctx context.Context, scene, channel, lang, receiver string, vars map[string]string) error {
	if channel != model.ChannelSMS && channel != model.ChannelEmail {
		return fmt.Errorf("%w: %s", ErrUnsupportedChannel, channel)
	}
	if receiver == "" {
		return errors.New("接收人不能为空")
	}
	tpl, err := m.loadTemplate(scene, channel, lang)
	if err != nil {
		return err
	}
	renderVars := make(map[string]string, len(vars)+1)
	for k, v := range vars {
		renderVars[k] = v
	}
	if _, ok := renderVars["site_name"]; !ok {
		// 自动注入站点名（尽力而为：读取失败不阻断发送）
		if siteName, found, e := model.GetSetting(m.db, model.SettingSiteName); e == nil && found {
			renderVars["site_name"] = siteName
		}
	}
	subject := Render(tpl.Subject, renderVars)
	content := Render(tpl.Content, renderVars)
	return m.SendRaw(ctx, channel, receiver, subject, content)
}

// SendRaw 绕过模板直发（后台"发送测试"使用）。
func (m *Manager) SendRaw(ctx context.Context, channel, to, subject, content string) error {
	switch channel {
	case model.ChannelSMS:
		p, err := m.SmsbaoProvider()
		if err != nil {
			return err
		}
		return p.Send(ctx, &Message{To: to, Content: content})
	case model.ChannelEmail:
		p, err := m.SMTPProvider()
		if err != nil {
			return err
		}
		return p.Send(ctx, &Message{To: to, Subject: subject, Content: content})
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedChannel, channel)
	}
}

// QuerySMSBalance 查询短信宝余额（已发送条数, 剩余条数）。
func (m *Manager) QuerySMSBalance(ctx context.Context) (sent, remain int, err error) {
	p, err := m.SmsbaoProvider()
	if err != nil {
		return 0, 0, err
	}
	return p.QueryBalance(ctx)
}

// SmsbaoProvider 从 settings 表实时构造短信宝 provider。
func (m *Manager) SmsbaoProvider() (*Smsbao, error) {
	settings, err := model.GetSettingsMap(m.db,
		model.SettingSmsbaoUsername, model.SettingSmsbaoPasswordMD5, model.SettingSmsbaoSign)
	if err != nil {
		return nil, fmt.Errorf("读取短信配置失败: %w", err)
	}
	username := settings[model.SettingSmsbaoUsername]
	passwordMD5 := settings[model.SettingSmsbaoPasswordMD5]
	if username == "" || passwordMD5 == "" {
		return nil, ErrNotConfigured
	}
	return NewSmsbao(SmsbaoConfig{
		Username:    username,
		PasswordMD5: passwordMD5,
		Sign:        settings[model.SettingSmsbaoSign],
	}), nil
}

// SMTPProvider 从 settings 表实时构造 SMTP provider。
func (m *Manager) SMTPProvider() (*SMTP, error) {
	settings, err := model.GetSettingsMap(m.db,
		model.SettingSMTPHost, model.SettingSMTPPort, model.SettingSMTPUsername,
		model.SettingSMTPPassword, model.SettingSMTPFrom, model.SettingSMTPTLS)
	if err != nil {
		return nil, fmt.Errorf("读取邮件配置失败: %w", err)
	}
	host := settings[model.SettingSMTPHost]
	from := settings[model.SettingSMTPFrom]
	if host == "" || from == "" {
		return nil, ErrNotConfigured
	}
	port, _ := strconv.Atoi(settings[model.SettingSMTPPort])
	if port <= 0 || port > 65535 {
		port = 25
	}
	useTLS, _ := strconv.ParseBool(settings[model.SettingSMTPTLS])
	return NewSMTP(SMTPConfig{
		Host:     host,
		Port:     port,
		Username: settings[model.SettingSMTPUsername],
		Password: settings[model.SettingSMTPPassword],
		From:     from,
		TLS:      useTLS,
	}), nil
}

// loadTemplate 加载模板：精确匹配 (scene, channel, lang)；缺失时回退 zh-CN。
func (m *Manager) loadTemplate(scene, channel, lang string) (*model.NotifyTemplate, error) {
	var tpl model.NotifyTemplate
	err := m.db.Where("scene = ? AND channel = ? AND lang = ?", scene, channel, lang).First(&tpl).Error
	if errors.Is(err, gorm.ErrRecordNotFound) && lang != model.LangZhCN {
		err = m.db.Where("scene = ? AND channel = ? AND lang = ?", scene, channel, model.LangZhCN).First(&tpl).Error
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: scene=%s channel=%s lang=%s", ErrTemplateNotFound, scene, channel, lang)
	}
	if err != nil {
		return nil, fmt.Errorf("查询模板失败: %w", err)
	}
	if !tpl.Enabled {
		return nil, fmt.Errorf("%w: scene=%s channel=%s lang=%s", ErrTemplateDisabled, scene, channel, lang)
	}
	return &tpl, nil
}
