package notify

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// 短信宝官方接口地址（生产固定；测试可经 SmsbaoConfig.BaseURL 注入 httptest 地址）。
const SmsbaoDefaultBaseURL = "https://api.smsbao.com"

// SmsbaoConfig 是短信宝 provider 配置。
type SmsbaoConfig struct {
	Username    string // 短信宝账号
	PasswordMD5 string // 密码 MD5（32 位十六进制）
	Sign        string // 短信签名，发送时自动前缀【签名】
	BaseURL     string // 留空使用官方地址；单元测试注入 mock 地址
}

// Smsbao 是短信宝 HTTP 通道的 Sender 实现。
//
// 接口约定（见契约文档 8.5）：
//   - 发送: GET {base}/sms?u=&p=&m=&c=  返回纯文本 "0" 为成功，其它为错误码；
//   - 余额: GET {base}/query?u=&p=     第一行 "0" 成功，第二行 "已发送条数,剩余条数"。
type Smsbao struct {
	cfg    SmsbaoConfig
	client *http.Client
}

// NewSmsbao 构造短信宝 provider。
func NewSmsbao(cfg SmsbaoConfig) *Smsbao {
	if cfg.BaseURL == "" {
		cfg.BaseURL = SmsbaoDefaultBaseURL
	}
	return &Smsbao{
		cfg:    cfg,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// Send 发送短信。内容不含签名时自动前缀【签名】。
func (s *Smsbao) Send(ctx context.Context, msg *Message) error {
	content := msg.Content
	if s.cfg.Sign != "" {
		prefix := "【" + s.cfg.Sign + "】"
		if !strings.HasPrefix(content, prefix) {
			content = prefix + content
		}
	}
	u := s.cfg.BaseURL + "/sms?" + url.Values{
		"u": {s.cfg.Username},
		"p": {s.cfg.PasswordMD5},
		"m": {msg.To},
		"c": {content},
	}.Encode()
	body, err := s.doGet(ctx, u)
	if err != nil {
		return err
	}
	return ParseSendResponse(body)
}

// QueryBalance 查询余额：返回（已发送条数, 剩余条数）。
func (s *Smsbao) QueryBalance(ctx context.Context) (sent, remain int, err error) {
	u := s.cfg.BaseURL + "/query?" + url.Values{
		"u": {s.cfg.Username},
		"p": {s.cfg.PasswordMD5},
	}.Encode()
	body, err := s.doGet(ctx, u)
	if err != nil {
		return 0, 0, err
	}
	lines := strings.Split(strings.TrimSpace(body), "\n")
	status := strings.TrimSpace(lines[0])
	if status != "0" {
		return 0, 0, fmt.Errorf("短信宝余额查询失败: %s", MapSmsbaoCode(status))
	}
	if len(lines) < 2 {
		return 0, 0, fmt.Errorf("短信宝余额查询返回格式异常: %q", body)
	}
	parts := strings.Split(strings.TrimSpace(lines[1]), ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("短信宝余额数据格式异常: %q", lines[1])
	}
	sent, err = strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("解析已发送条数失败: %q", parts[0])
	}
	remain, err = strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("解析剩余条数失败: %q", parts[1])
	}
	return sent, remain, nil
}

// doGet 执行 GET 请求并返回纯文本响应体（限制 4KB，防异常响应撑爆内存）。
func (s *Smsbao) doGet(ctx context.Context, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("构造短信宝请求失败: %w", err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求短信宝接口失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", fmt.Errorf("读取短信宝响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("短信宝接口返回 HTTP %d", resp.StatusCode)
	}
	return string(data), nil
}

// ParseSendResponse 解析发送接口响应：纯文本 "0" 为成功，其它为错误码。
func ParseSendResponse(body string) error {
	code := strings.TrimSpace(body)
	if code == "0" {
		return nil
	}
	return fmt.Errorf("短信宝发送失败: %s", MapSmsbaoCode(code))
}

// MapSmsbaoCode 把短信宝错误码映射为可读错误信息（对照表见契约文档 8.4）。
func MapSmsbaoCode(code string) string {
	switch strings.TrimSpace(code) {
	case "-1":
		return "参数错误"
	case "-2":
		return "用户名或密码错误"
	case "-3":
		return "短信内容超过长度限制"
	case "-4":
		return "手机号数量超过限制"
	case "30":
		return "密码错误"
	case "33":
		return "IP 未通过验证（需在短信宝后台报备服务器 IP）"
	case "40":
		return "手机号码格式错误"
	case "41":
		return "余额不足"
	case "42":
		return "发送过于频繁，触发限流"
	case "50":
		return "内容含敏感词，被拦截"
	case "51":
		return "同一号码发送次数超限"
	default:
		return "未知错误码: " + code
	}
}
