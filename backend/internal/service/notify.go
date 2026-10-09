package service

import (
	"context"
	"crypto/md5" // #nosec G501 -- 短信宝官方接口要求密码以 MD5 传输，非安全用途
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/model"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/notify"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/app"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/errcode"
)

// NotifyService 处理通知配置、模板管理与测试发送业务（契约文档第 5 章）。
type NotifyService struct {
	app *app.App
}

// NewNotify 构造通知服务。
func NewNotify(a *app.App) *NotifyService {
	return &NotifyService{app: a}
}

// 校验正则
var (
	md5Re      = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)
	phoneRe    = regexp.MustCompile(`^[0-9,+\-]{5,500}$`)
	emailRe    = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	usernameRe = regexp.MustCompile(`^[A-Za-z0-9_.@-]{1,128}$`)
)

// SmsbaoSettings 是短信宝配置对外视图（密码不回传，只回传是否已设置）。
type SmsbaoSettings struct {
	Username    string `json:"username"`
	PasswordSet bool   `json:"password_set"`
	Sign        string `json:"sign"`
}

// SMTPSettings 是 SMTP 配置对外视图。
type SMTPSettings struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	PasswordSet bool   `json:"password_set"`
	From        string `json:"from"`
	TLS         bool   `json:"tls"`
}

// NotifySettings 是通知配置聚合视图（契约文档 5.1）。
type NotifySettings struct {
	Smsbao SmsbaoSettings `json:"smsbao"`
	SMTP   SMTPSettings   `json:"smtp"`
}

// GetSettings 读取通知配置（运行时真源为 settings 表）。
func (s *NotifyService) GetSettings(ctx context.Context) (*NotifySettings, error) {
	db := s.requireDB()
	if db == nil {
		return nil, errcode.Internal("数据库未就绪")
	}
	m, err := model.GetSettingsMap(db.WithContext(ctx),
		model.SettingSmsbaoUsername, model.SettingSmsbaoPasswordMD5, model.SettingSmsbaoSign,
		model.SettingSMTPHost, model.SettingSMTPPort, model.SettingSMTPUsername,
		model.SettingSMTPPassword, model.SettingSMTPFrom, model.SettingSMTPTLS)
	if err != nil {
		return nil, errcode.Internal("读取通知配置失败").WithErr(err)
	}
	port, _ := strconv.Atoi(m[model.SettingSMTPPort])
	if port <= 0 {
		port = 25
	}
	tlsEnabled, _ := strconv.ParseBool(m[model.SettingSMTPTLS])
	return &NotifySettings{
		Smsbao: SmsbaoSettings{
			Username:    m[model.SettingSmsbaoUsername],
			PasswordSet: m[model.SettingSmsbaoPasswordMD5] != "",
			Sign:        m[model.SettingSmsbaoSign],
		},
		SMTP: SMTPSettings{
			Host:        m[model.SettingSMTPHost],
			Port:        port,
			Username:    m[model.SettingSMTPUsername],
			PasswordSet: m[model.SettingSMTPPassword] != "",
			From:        m[model.SettingSMTPFrom],
			TLS:         tlsEnabled,
		},
	}, nil
}

// SaveNotifySettingsRequest 是保存通知配置请求（契约文档 5.2）。
// 密码字段留空 = 不修改；短信宝密码支持提交明文（自动 MD5）或 32 位十六进制 MD5。
type SaveNotifySettingsRequest struct {
	Smsbao *struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Sign     string `json:"sign"`
	} `json:"smsbao"`
	SMTP *struct {
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
		From     string `json:"from"`
		TLS      bool   `json:"tls"`
	} `json:"smtp"`
}

// SaveSettings 保存通知配置（仅更新提交的分节；密码留空保持不变）。
func (s *NotifyService) SaveSettings(ctx context.Context, req *SaveNotifySettingsRequest) error {
	db := s.requireDB()
	if db == nil {
		return errcode.Internal("数据库未就绪")
	}
	kvs := make(map[string]string)

	if req.Smsbao != nil {
		if req.Smsbao.Username != "" && !usernameRe.MatchString(req.Smsbao.Username) {
			return errcode.InvalidParam("短信宝账号格式不正确")
		}
		if len([]rune(req.Smsbao.Sign)) > 32 {
			return errcode.InvalidParam("短信签名不超过 32 字符")
		}
		kvs[model.SettingSmsbaoUsername] = strings.TrimSpace(req.Smsbao.Username)
		kvs[model.SettingSmsbaoSign] = strings.TrimSpace(req.Smsbao.Sign)
		if pw := strings.TrimSpace(req.Smsbao.Password); pw != "" {
			kvs[model.SettingSmsbaoPasswordMD5] = toMD5(pw)
		}
	}
	if req.SMTP != nil {
		if req.SMTP.Port < 1 || req.SMTP.Port > 65535 {
			return errcode.InvalidParam("SMTP 端口必须在 1-65535 之间")
		}
		if req.SMTP.Host != "" && strings.ContainsAny(req.SMTP.Host, " \t/@") {
			return errcode.InvalidParam("SMTP 主机名格式不正确")
		}
		if req.SMTP.From != "" && !emailRe.MatchString(req.SMTP.From) {
			return errcode.InvalidParam("发件人地址必须是合法邮箱")
		}
		if req.SMTP.Username != "" && !emailRe.MatchString(req.SMTP.Username) && !usernameRe.MatchString(req.SMTP.Username) {
			return errcode.InvalidParam("SMTP 用户名格式不正确")
		}
		kvs[model.SettingSMTPHost] = strings.TrimSpace(req.SMTP.Host)
		kvs[model.SettingSMTPPort] = strconv.Itoa(req.SMTP.Port)
		kvs[model.SettingSMTPUsername] = strings.TrimSpace(req.SMTP.Username)
		kvs[model.SettingSMTPFrom] = strings.TrimSpace(req.SMTP.From)
		kvs[model.SettingSMTPTLS] = strconv.FormatBool(req.SMTP.TLS)
		if pw := req.SMTP.Password; pw != "" {
			kvs[model.SettingSMTPPassword] = pw
		}
	}
	if len(kvs) == 0 {
		return errcode.InvalidParam("请求体为空：至少提交 smsbao 或 smtp 一节")
	}
	if err := model.SetSettingsMap(db.WithContext(ctx), kvs); err != nil {
		return errcode.Internal("保存通知配置失败").WithErr(err)
	}
	slog.Info("通知配置已更新", "keys", len(kvs))
	return nil
}

// TestSMSRequest 是测试短信请求（契约文档 5.3）。
type TestSMSRequest struct {
	To      string `json:"to"`
	Content string `json:"content"`
}

// TestSMS 用当前配置发送测试短信（绕过模板，签名仍由 provider 自动添加）。
func (s *NotifyService) TestSMS(ctx context.Context, req *TestSMSRequest) error {
	mgr := s.app.GetNotify()
	if mgr == nil {
		return errcode.Internal("通知模块未就绪")
	}
	if !phoneRe.MatchString(req.To) {
		return errcode.InvalidParam("手机号格式不正确（多个号码用英文逗号分隔）")
	}
	if len([]rune(req.Content)) > 500 {
		return errcode.InvalidParam("测试短信内容不超过 500 字符")
	}
	content := req.Content
	if content == "" {
		siteName := s.siteName(ctx)
		content = fmt.Sprintf("这是来自 %s 的测试短信，收到即代表短信通道配置正确，请勿回复。", siteName)
	}
	if err := mgr.SendRaw(ctx, model.ChannelSMS, req.To, "", content); err != nil {
		return s.mapNotifyError(err, errcode.CodeSMSSendFailed)
	}
	return nil
}

// TestEmailRequest 是测试邮件请求（契约文档 5.4）。
type TestEmailRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Content string `json:"content"`
}

// TestEmail 用当前配置发送测试邮件（绕过模板）。
func (s *NotifyService) TestEmail(ctx context.Context, req *TestEmailRequest) error {
	mgr := s.app.GetNotify()
	if mgr == nil {
		return errcode.Internal("通知模块未就绪")
	}
	if !emailRe.MatchString(req.To) {
		return errcode.InvalidParam("收件人邮箱格式不正确")
	}
	siteName := s.siteName(ctx)
	subject := req.Subject
	if subject == "" {
		subject = fmt.Sprintf("【%s】测试邮件", siteName)
	}
	content := req.Content
	if content == "" {
		content = fmt.Sprintf("<p>这是一封来自 %s 的测试邮件。</p><p>收到即代表 SMTP 邮件通道配置正确。</p>", siteName)
	}
	if err := mgr.SendRaw(ctx, model.ChannelEmail, req.To, subject, content); err != nil {
		return s.mapNotifyError(err, errcode.CodeEmailSendFailed)
	}
	return nil
}

// SMSBalance 查询短信宝余额（契约文档 5.5）。
func (s *NotifyService) SMSBalance(ctx context.Context) (sent, remain int, err error) {
	mgr := s.app.GetNotify()
	if mgr == nil {
		return 0, 0, errcode.Internal("通知模块未就绪")
	}
	sent, remain, err = mgr.QuerySMSBalance(ctx)
	if err != nil {
		if errors.Is(err, notify.ErrNotConfigured) {
			return 0, 0, errcode.New(400, errcode.CodeNotifyNotConfigured, err.Error())
		}
		return 0, 0, errcode.New(502, errcode.CodeSMSBalanceQueryFailed, err.Error())
	}
	return sent, remain, nil
}

// ListTemplates 按可选过滤条件返回模板列表（契约文档 5.6）。
func (s *NotifyService) ListTemplates(ctx context.Context, scene, channel, lang string) ([]model.NotifyTemplate, error) {
	db := s.requireDB()
	if db == nil {
		return nil, errcode.Internal("数据库未就绪")
	}
	// 过滤值合法性校验（空 = 不过滤）
	if scene != "" && !contains(model.ValidScenes, scene) {
		return nil, errcode.InvalidParam("scene 非法，可选值: %s", strings.Join(model.ValidScenes, "/"))
	}
	if channel != "" && !contains(model.ValidChannels, channel) {
		return nil, errcode.InvalidParam("channel 非法，可选值: %s", strings.Join(model.ValidChannels, "/"))
	}
	if lang != "" && !contains(model.ValidLangs, lang) {
		return nil, errcode.InvalidParam("lang 非法，可选值: %s", strings.Join(model.ValidLangs, "/"))
	}
	q := db.WithContext(ctx).Model(&model.NotifyTemplate{})
	if scene != "" {
		q = q.Where("scene = ?", scene)
	}
	if channel != "" {
		q = q.Where("channel = ?", channel)
	}
	if lang != "" {
		q = q.Where("lang = ?", lang)
	}
	var list []model.NotifyTemplate
	if err := q.Order("scene ASC, channel ASC, lang ASC, id ASC").Find(&list).Error; err != nil {
		return nil, errcode.Internal("查询模板失败").WithErr(err)
	}
	return list, nil
}

// UpdateTemplateRequest 是更新模板请求（三字段均可选，缺省不修改；契约文档 5.7）。
type UpdateTemplateRequest struct {
	Subject *string `json:"subject"`
	Content *string `json:"content"`
	Enabled *bool   `json:"enabled"`
}

// UpdateTemplate 更新模板内容/主题/启用状态（scene/channel/lang 不可改）。
func (s *NotifyService) UpdateTemplate(ctx context.Context, id uint, req *UpdateTemplateRequest) (*model.NotifyTemplate, error) {
	db := s.requireDB()
	if db == nil {
		return nil, errcode.Internal("数据库未就绪")
	}
	if req.Subject == nil && req.Content == nil && req.Enabled == nil {
		return nil, errcode.InvalidParam("请求体为空：至少提交 subject/content/enabled 之一")
	}
	var tpl model.NotifyTemplate
	err := db.WithContext(ctx).First(&tpl, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.New(404, errcode.CodeTemplateNotFound, fmt.Sprintf("模板 #%d 不存在", id))
	}
	if err != nil {
		return nil, errcode.Internal("查询模板失败").WithErr(err)
	}

	updates := map[string]any{}
	if req.Subject != nil {
		subject := strings.TrimSpace(*req.Subject)
		if len([]rune(subject)) > 255 {
			return nil, errcode.InvalidParam("subject 不超过 255 字符")
		}
		updates["subject"] = subject
	}
	if req.Content != nil {
		content := *req.Content
		if strings.TrimSpace(content) == "" {
			return nil, errcode.InvalidParam("content 不能为空")
		}
		if len(content) > 60000 {
			return nil, errcode.InvalidParam("content 过长（>60000 字符）")
		}
		updates["content"] = content
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if err := db.WithContext(ctx).Model(&tpl).Updates(updates).Error; err != nil {
		return nil, errcode.Internal("更新模板失败").WithErr(err)
	}
	// 重新读取，返回更新后的完整对象（含 updated_at）
	if err := db.WithContext(ctx).First(&tpl, id).Error; err != nil {
		return nil, errcode.Internal("重新读取模板失败").WithErr(err)
	}
	return &tpl, nil
}

// mapNotifyError 把 notify 层哨兵错误映射为契约错误码。
func (s *NotifyService) mapNotifyError(err error, sendFailedCode string) error {
	switch {
	case errors.Is(err, notify.ErrNotConfigured):
		return errcode.New(400, errcode.CodeNotifyNotConfigured, err.Error())
	case errors.Is(err, notify.ErrTemplateNotFound):
		return errcode.New(404, errcode.CodeTemplateNotFound, err.Error())
	case errors.Is(err, notify.ErrTemplateDisabled):
		return errcode.New(400, errcode.CodeTemplateDisabled, err.Error())
	default:
		return errcode.New(502, sendFailedCode, err.Error())
	}
}

// siteName 读取站点名（尽力而为，失败回退配置）。
func (s *NotifyService) siteName(ctx context.Context) string {
	if db := s.requireDB(); db != nil {
		if v, ok, err := model.GetSetting(db.WithContext(ctx), model.SettingSiteName); err == nil && ok && v != "" {
			return v
		}
	}
	return s.app.Cfg().Brand.SiteName
}

// requireDB 返回数据库句柄（可能为 nil）。
func (s *NotifyService) requireDB() *gorm.DB {
	return s.app.GetDB()
}

// toMD5 计算字符串的 MD5 十六进制（小写）。
// 短信宝接口约定密码以 MD5 传输；若提交值已是 32 位十六进制则原样使用。
func toMD5(s string) string {
	if md5Re.MatchString(s) {
		return strings.ToLower(s)
	}
	sum := md5.Sum([]byte(s)) // #nosec G401 -- 接口协议要求，非安全用途
	return hex.EncodeToString(sum[:])
}

// contains 判断字符串切片是否包含目标值。
func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
