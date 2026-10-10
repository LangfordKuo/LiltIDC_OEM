package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/model"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/notify"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/app"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/captcha"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/errcode"
)

// VerifyService 处理验证码（短信/邮件）与图形验证码。
//
// 存储：settings 表（契约 12.9）——验证码存 SHA-256 哈希（明文不落库不入日志）；
// 频控：同 target+purpose 60s/1条、24h/10条；同 IP 当日 ≥5 条强制图形码、≥30 条拒绝。
// 校验失败不消耗验证码，但连续错 5 次作废（防在线爆破）；注册成功后显式 Consume。
type VerifyService struct {
	app *app.App
}

// NewVerify 构造验证码服务。
func NewVerify(a *app.App) *VerifyService {
	return &VerifyService{app: a}
}

// 验证码策略常量（契约 12.0）。
const (
	verifyCodeTTL       = 10 * time.Minute // 验证码有效期
	verifyResendGap     = 60 * time.Second // 同 target 重发间隔
	verifyDailyLimit    = 10               // 同 target 24h 上限
	verifyMaxAttempts   = 5                // 同一验证码最多试错次数（防在线爆破）
	ipDailyCaptchaAfter = 5                // 同 IP 当日发送达到该数后强制图形码
	ipDailyHardLimit    = 30               // 同 IP 当日发送硬上限
	captchaTTL          = 5 * time.Minute  // 图形码有效期
)

// purpose → notify 模板 scene 映射（批 3-α 仅 register；reset 留批 3-β）。
var purposeScene = map[string]string{
	"register": model.SceneRegisterVerify,
}

// CaptchaView 是图形验证码响应。
type CaptchaView struct {
	CaptchaID string `json:"captcha_id"`
	Image     string `json:"image"` // data:image/png;base64,...
	ExpiresIn int    `json:"expires_in"`
}

// NewCaptcha 生成图形验证码（答案哈希存 settings，5 分钟过期）。
func (s *VerifyService) NewCaptcha(ctx context.Context) (*CaptchaView, error) {
	db := s.app.GetDB()
	answer, dataURI, err := captcha.Generate()
	if err != nil {
		return nil, errcode.Internal("生成图形验证码失败").WithErr(err)
	}
	id, err := randomHex(16)
	if err != nil {
		return nil, errcode.Internal("生成验证码 ID 失败").WithErr(err)
	}
	expires := time.Now().Add(captchaTTL).Unix()
	val := sha256Hex(strings.ToLower(answer)) + "|" + strconv.FormatInt(expires, 10)
	if err := model.SetSetting(db.WithContext(ctx), "captcha_"+id, val); err != nil {
		return nil, errcode.Internal("保存图形验证码失败").WithErr(err)
	}
	return &CaptchaView{CaptchaID: id, Image: dataURI, ExpiresIn: int(captchaTTL.Seconds())}, nil
}

// IssueCode 生成并发送验证码（含全部频控与图形码逻辑；发送失败不消耗配额）。
// lang 用于模板选择（zh-CN/en-US）；ip 用于日频控。
func (s *VerifyService) IssueCode(ctx context.Context, purpose, channel, to, lang, ip, captchaID, captchaAnswer string) error {
	scene, ok := purposeScene[purpose]
	if !ok {
		return errcode.InvalidParam("purpose 非法（当前仅支持 register）")
	}
	db := s.app.GetDB()
	now := time.Now()

	// ---- 1. IP 频控与图形码门槛 ----
	ipKey := "vcode_ip_" + ip
	_, ipCount := s.readDayCounter(ctx, ipKey, now)
	if ipCount >= ipDailyHardLimit {
		return errcode.New(429, errcode.CodeRateLimited, "今日验证码发送次数已达上限，请明天再试")
	}
	if ipCount >= ipDailyCaptchaAfter && (captchaID == "" || captchaAnswer == "") {
		return errcode.New(400, errcode.CodeCaptchaRequired, "发送次数较多，请先完成图形验证码")
	}
	if captchaID != "" {
		if err := s.verifyCaptcha(ctx, captchaID, captchaAnswer); err != nil {
			return err
		}
	}

	// ---- 2. target 频控（60s 重发间隔 + 24h 上限） ----
	rlKey := fmt.Sprintf("vcode_rl_%s_%s:%s", purpose, channel, to)
	lastSent, _, dayCount := s.readRL(ctx, rlKey, now)
	if wait := verifyResendGap - now.Sub(lastSent); !lastSent.IsZero() && wait > 0 {
		return errcode.New(429, errcode.CodeRateLimited, fmt.Sprintf("发送过于频繁，请 %d 秒后重试", int(wait.Seconds())+1))
	}
	if dayCount >= verifyDailyLimit {
		return errcode.New(429, errcode.CodeRateLimited, "该账号今日验证码发送次数已达上限（10 次），请明天再试")
	}

	// ---- 3. 生成 6 位数字码并经 notify 模板发送（失败不消耗配额） ----
	code, err := randomDigits(6)
	if err != nil {
		return errcode.Internal("生成验证码失败").WithErr(err)
	}
	mgr := s.app.GetNotify()
	if mgr == nil {
		return errcode.Internal("通知模块未就绪")
	}
	if err := mgr.Send(ctx, scene, channel, lang, to, map[string]string{"code": code}); err != nil {
		// notify 哨兵错误映射（与通知测试发送同一套语义）
		return s.mapNotifySendError(err, channel)
	}

	// ---- 4. 发送成功后落库：验证码（哈希）+ 频控计数 ----
	codeKey := fmt.Sprintf("verify_code_%s_%s:%s", purpose, channel, to)
	codeVal := fmt.Sprintf("%s|%d|%d|0", sha256Hex(code), now.Add(verifyCodeTTL).Unix(), now.Unix())
	if err := model.SetSetting(db.WithContext(ctx), codeKey, codeVal); err != nil {
		slog.Error("验证码落库失败（已发送但无法校验，用户需重发）", "err", err)
		return errcode.Internal("验证码保存失败，请重新发送")
	}
	s.writeRL(ctx, rlKey, now, dayCount)
	s.writeDayCounter(ctx, ipKey, now, ipCount)
	slog.Info("验证码已发送", "purpose", purpose, "channel", channel, "lang", lang) // 不记录 to 与 code
	return nil
}

// VerifyCode 校验验证码（成功不消耗；失败计次，连续 verifyMaxAttempts 次作废）。
func (s *VerifyService) VerifyCode(ctx context.Context, purpose, channel, to, code string) error {
	db := s.app.GetDB()
	codeKey := fmt.Sprintf("verify_code_%s_%s:%s", purpose, channel, to)
	val, ok, err := model.GetSetting(db.WithContext(ctx), codeKey)
	if err != nil {
		return errcode.Internal("读取验证码失败").WithErr(err)
	}
	invalid := errcode.New(400, errcode.CodeVerifyCodeInvalid, "验证码错误或已过期")
	if !ok || val == "" {
		return invalid
	}
	parts := strings.Split(val, "|")
	if len(parts) != 4 {
		return invalid
	}
	expire, _ := strconv.ParseInt(parts[1], 10, 64)
	attempts, _ := strconv.Atoi(parts[3])
	if time.Now().Unix() > expire || attempts >= verifyMaxAttempts {
		_ = s.deleteSetting(ctx, codeKey) // 过期/爆破限额 → 作废
		return invalid
	}
	if parts[0] != sha256Hex(strings.TrimSpace(code)) {
		// 失败不消耗但计次；达到上限作废（防 10 分钟窗口内在线爆破 10^6）
		parts[3] = strconv.Itoa(attempts + 1)
		if attempts+1 >= verifyMaxAttempts {
			_ = s.deleteSetting(ctx, codeKey)
		} else {
			_ = model.SetSetting(db.WithContext(ctx), codeKey, strings.Join(parts, "|"))
		}
		return invalid
	}
	return nil
}

// ConsumeCode 使验证码失效（注册成功后调用，防重放）。
func (s *VerifyService) ConsumeCode(ctx context.Context, purpose, channel, to string) {
	key := fmt.Sprintf("verify_code_%s_%s:%s", purpose, channel, to)
	_ = s.deleteSetting(ctx, key)
}

// verifyCaptcha 校验图形验证码（校验即焚：无论对错先删除）。
func (s *VerifyService) verifyCaptcha(ctx context.Context, id, answer string) error {
	db := s.app.GetDB()
	key := "captcha_" + id
	val, ok, err := model.GetSetting(db.WithContext(ctx), key)
	_ = s.deleteSetting(ctx, key) // 校验即焚
	invalid := errcode.New(400, errcode.CodeCaptchaInvalid, "图形验证码错误或已过期")
	if err != nil || !ok || val == "" {
		return invalid
	}
	parts := strings.Split(val, "|")
	if len(parts) != 2 {
		return invalid
	}
	expire, _ := strconv.ParseInt(parts[1], 10, 64)
	if time.Now().Unix() > expire {
		return invalid
	}
	if parts[0] != sha256Hex(strings.ToLower(strings.TrimSpace(answer))) {
		return invalid
	}
	return nil
}

// ---- settings 计数读写助手 ----

// dayStartUnix 返回当天 0 点的 unix 秒（本地时区）。
func dayStartUnix(now time.Time) int64 {
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, now.Location()).Unix()
}

// readDayCounter 读 IP 当日计数（"dayStart|count"；跨天视为 0，不落库——写在成功发送后）。
func (s *VerifyService) readDayCounter(ctx context.Context, key string, now time.Time) (dayStart int64, count int) {
	db := s.app.GetDB()
	val, ok, err := model.GetSetting(db.WithContext(ctx), key)
	today := dayStartUnix(now)
	if err != nil || !ok || val == "" {
		return today, 0
	}
	parts := strings.Split(val, "|")
	if len(parts) != 2 {
		return today, 0
	}
	stored, _ := strconv.ParseInt(parts[0], 10, 64)
	n, _ := strconv.Atoi(parts[1])
	if stored != today { // 跨天重置
		return today, 0
	}
	return today, n
}

// writeDayCounter 计数 +1 落库（发送成功后调用）。
func (s *VerifyService) writeDayCounter(ctx context.Context, key string, now time.Time, count int) {
	db := s.app.GetDB()
	val := fmt.Sprintf("%d|%d", dayStartUnix(now), count+1)
	if err := model.SetSetting(db.WithContext(ctx), key, val); err != nil {
		slog.Warn("频控计数写入失败（不阻断）", "key", key, "err", err)
	}
}

// readRL 读 target 频控记录（"lastSent|dayStart|dayCount"）。
func (s *VerifyService) readRL(ctx context.Context, key string, now time.Time) (lastSent time.Time, dayStart int64, dayCount int) {
	db := s.app.GetDB()
	today := dayStartUnix(now)
	val, ok, err := model.GetSetting(db.WithContext(ctx), key)
	if err != nil || !ok || val == "" {
		return time.Time{}, today, 0
	}
	parts := strings.Split(val, "|")
	if len(parts) != 3 {
		return time.Time{}, today, 0
	}
	ls, _ := strconv.ParseInt(parts[0], 10, 64)
	ds, _ := strconv.ParseInt(parts[1], 10, 64)
	dc, _ := strconv.Atoi(parts[2])
	if ds != today { // 跨天重置日计数
		ds, dc = today, 0
	}
	return time.Unix(ls, 0), ds, dc
}

// writeRL 更新 target 频控记录（发送成功后调用）。
func (s *VerifyService) writeRL(ctx context.Context, key string, now time.Time, dayCount int) {
	db := s.app.GetDB()
	val := fmt.Sprintf("%d|%d|%d", now.Unix(), dayStartUnix(now), dayCount+1)
	if err := model.SetSetting(db.WithContext(ctx), key, val); err != nil {
		slog.Warn("频控记录写入失败（不阻断）", "key", key, "err", err)
	}
}

// deleteSetting 删除 settings 键（不存在也不报错）。
func (s *VerifyService) deleteSetting(ctx context.Context, key string) error {
	db := s.app.GetDB()
	return db.WithContext(ctx).Where("`key` = ?", key).Delete(&model.Setting{}).Error
}

// mapNotifySendError 把 notify 发送错误映射为契约错误码（与通知测试发送同语义）。
func (s *VerifyService) mapNotifySendError(err error, channel string) error {
	switch {
	case errors.Is(err, notify.ErrNotConfigured):
		return errcode.New(400, errcode.CodeNotifyNotConfigured, "验证码通道未配置："+err.Error())
	case errors.Is(err, notify.ErrTemplateNotFound):
		return errcode.New(404, errcode.CodeTemplateNotFound, err.Error())
	case errors.Is(err, notify.ErrTemplateDisabled):
		return errcode.New(400, errcode.CodeTemplateDisabled, "验证码模板已停用，请在后台启用后重试")
	case channel == model.ChannelSMS:
		return errcode.New(502, errcode.CodeSMSSendFailed, "验证码短信发送失败："+err.Error())
	default:
		return errcode.New(502, errcode.CodeEmailSendFailed, "验证码邮件发送失败："+err.Error())
	}
}

// ---- 通用小工具 ----

// sha256Hex 计算 SHA-256 十六进制（验证码/图形码哈希存储用）。
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// randomHex 生成 n 字节加密随机数的十六进制串。
func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// randomDigits 生成 n 位加密随机数字串（首位可为 0）。
func randomDigits(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	var b strings.Builder
	for _, v := range buf {
		b.WriteByte('0' + byte(int(v)%10))
	}
	return b.String(), nil
}
