// Package upstream 实现上游魔方财务（zjmf）对接客户端。
//
// 协议要点（Langford 已用真实测试账号实测，见 docs/api-contract.md 第 9 章）：
//   - 登录：POST {hostname}/zjmf_api_login，JSON {"username","password"} →
//     {"jwt":"...","status":200,"msg":"鉴权成功"}；手机号不加区号；
//   - 后续请求头 Authorization: Bearer <jwt>；jwt 缓存复用（并发安全），
//     失效时上游返回 {"status":405}，重登一次再重试（最多一次，防死循环）；
//   - 运行时凭据真源是数据库 settings（SessionManager 按指纹复用 Client，
//     后台改配置立即生效），config.toml [upstream] 仅为安装/启动种子。
//
// ⚠ 批 2 只允许调用只读端点（登录、/cart/all、/cart/credit、
// /api/product/proinfo、/cart/get_product_config、/host/list）；
// 写操作端点（下单/开通/续费/退款等）批 3 才启用。
package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// 默认单次上游请求超时（config [upstream] timeout_seconds 可配）。
const DefaultTimeout = 15 * time.Second

// 上游响应体大小上限（/cart/all 商品树可能较大，8MB 足够且防失控）。
const maxResponseBytes = 8 << 20

// 哨兵错误（service 层用 errors.Is 映射为契约错误码）。
var (
	ErrReauth        = errors.New("上游要求重新登录")
	ErrAuthFailed    = errors.New("上游登录失败（账号或 API 密钥错误）")
	ErrNotConfigured = errors.New("上游未配置（请先在后台填写上游地址/账号/密钥）")
)

// Credentials 是一次上游会话所需的凭据。
type Credentials struct {
	Hostname string        // 上游站点地址，如 https://lyew.com
	Username string        // 上游账号（手机号不加区号）
	APIKey   string        // 上游 API 密钥（作为登录密码）
	Timeout  time.Duration // 单次请求超时
}

// Client 是上游魔方财务客户端。
// 零值不可用；并发安全（jwt 缓存与登录经 mutex 串行化）。
type Client struct {
	cred Credentials
	http *http.Client

	mu  sync.Mutex
	jwt string // 缓存的登录 jwt；空表示尚未登录/已失效
}

// NewClient 构造客户端（不立即登录，首次请求或显式 Login 时登录）。
func NewClient(cred Credentials) *Client {
	timeout := cred.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &Client{
		cred: cred,
		http: &http.Client{Timeout: timeout},
	}
}

// loginResponse 是 /zjmf_api_login 的响应。
type loginResponse struct {
	JWT    string `json:"jwt"`
	Status int    `json:"status"`
	Msg    string `json:"msg"`
}

// Login 登录上游并缓存 jwt（并发调用串行化）。
func (c *Client) Login(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loginLocked(ctx)
}

// JWT 返回当前缓存的 jwt（测试与诊断用；空表示未登录）。
func (c *Client) JWT() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.jwt
}

func (c *Client) loginLocked(ctx context.Context) error {
	if c.cred.Hostname == "" || c.cred.Username == "" || c.cred.APIKey == "" {
		return ErrNotConfigured
	}
	body, _ := json.Marshal(map[string]string{
		"username": c.cred.Username,
		"password": c.cred.APIKey,
	})
	endpoint := strings.TrimRight(c.cred.Hostname, "/") + "/zjmf_api_login"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("构造登录请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("上游登录请求失败（%s）: %w", endpoint, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("上游登录返回 HTTP %d（若为 WAF/网关拦截页请检查上游防火墙设置）", resp.StatusCode)
	}
	var lr loginResponse
	if err := json.Unmarshal(data, &lr); err != nil {
		return fmt.Errorf("上游登录响应不是合法 JSON（可能被网关拦截）: %w", err)
	}
	if lr.JWT == "" {
		msg := lr.Msg
		if msg == "" {
			msg = "响应中没有 jwt 字段"
		}
		return fmt.Errorf("%w: %s", ErrAuthFailed, msg)
	}
	c.jwt = lr.JWT
	return nil
}

// userAgent 是上游请求的 UA 标识（部分上游 WAF 会拦空 UA）。
const userAgent = "LiltIDC-OEM/0.2 (+upstream-client)"

// do 执行上游 API 请求（GET 无请求体；成功状态码 0/200）：
// 自动登录、jwt 失效（status=405）时重登重试一次。out 为 nil 时忽略响应体。
func (c *Client) do(ctx context.Context, method, path string, query url.Values, out any) error {
	return c.doBody(ctx, method, path, query, "", "", []int{0, 200}, out)
}

// doBody 执行上游 API 请求的完整形态（批 3-β 写端点扩展）：
//   - body 非空时作为请求体发送（contentType 如 application/x-www-form-urlencoded，
//     魔方互联调用 zjmfCurl/commonCurl 的 POST 一律 form 编码，本系统同形发送）；
//   - success 是允许的成功状态码集合（读端点 {0,200}；写端点含 1001——魔方
//     "购买成功/支付完成" 语义，见 cart.go）；status 缺失容忍为 0（ADR-020）；
//   - 业务拒绝返回 *BizError（携带上游 status/msg 原文，service 层区分补偿路径）；
//   - jwt 失效（status=405）重登重试一次（防死循环，语义与批 2 完全一致）。
func (c *Client) doBody(ctx context.Context, method, path string, query url.Values, body, contentType string, success []int, out any) error {
	// 无缓存 jwt 时先登录（失败直接报错，省一次必 405 的请求）
	c.mu.Lock()
	hasJWT := c.jwt != ""
	c.mu.Unlock()
	if !hasJWT {
		if err := c.Login(ctx); err != nil {
			return err
		}
	}
	err := c.doOnce(ctx, method, path, query, body, contentType, success, out)
	if errors.Is(err, ErrReauth) {
		// jwt 失效：重登一次再重试（仅一次，防死循环）
		c.mu.Lock()
		c.jwt = ""
		loginErr := c.loginLocked(ctx)
		c.mu.Unlock()
		if loginErr != nil {
			return loginErr
		}
		err = c.doOnce(ctx, method, path, query, body, contentType, success, out)
	}
	return err
}

// doOnce 执行单次请求（不带重登逻辑）。
func (c *Client) doOnce(ctx context.Context, method, path string, query url.Values, body, contentType string, success []int, out any) error {
	c.mu.Lock()
	jwt := c.jwt
	c.mu.Unlock()

	u := strings.TrimRight(c.cred.Hostname, "/") + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
	if err != nil {
		return fmt.Errorf("构造上游请求失败: %w", err)
	}
	if jwt != "" {
		req.Header.Set("Authorization", "Bearer "+jwt)
	}
	if body != "" && contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("上游请求失败（%s %s）: %w", method, path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("上游返回 HTTP %d（%s %s）", resp.StatusCode, method, path)
	}

	// envelope 解析（一次 Unmarshal 同时取 status/msg 与 data 载荷）：
	//   - {"status":405} = jwt 失效 → ErrReauth；其它非成功码 status = 业务拒绝（*BizError）；
	//   - 魔方真实形状：业务端点 payload 嵌在 "data" 键下（仅登录 jwt 在顶层，
	//     登录走 loginLocked 独立路径）——有 data（非 null）则解包 data，
	//     否则解整个响应体（兼容顶层 payload 形状与缺省 status 的端点）。
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("上游响应不是合法 JSON（%s %s，可能被网关拦截）: %w", method, path, err)
	}
	status := 0
	if s, ok := raw["status"]; ok {
		_ = json.Unmarshal(s, &status) // status 非数字时容忍为 0（放行）
	}
	if status == 405 {
		return ErrReauth
	}
	if !isSuccessStatus(status, success) {
		msg := ""
		if m, ok := raw["msg"]; ok {
			_ = json.Unmarshal(m, &msg)
		}
		if msg == "" {
			msg = "无错误描述"
		}
		return &BizError{Path: path, Status: status, Msg: msg}
	}
	if out != nil {
		payload := data
		if d, ok := raw["data"]; ok && len(d) > 0 && string(d) != "null" {
			payload = d // data 信封解包（data:null 容错为整包）
		}
		if err := json.Unmarshal(payload, out); err != nil {
			return fmt.Errorf("解析上游响应失败（%s %s）: %w", method, path, err)
		}
	}
	return nil
}

// isSuccessStatus 判断信封 status 是否属于成功集合（0 恒视为成功——
// 部分端点成功响应不含 status 字段，ADR-020 宽松判定保持不变）。
func isSuccessStatus(status int, success []int) bool {
	if status == 0 {
		return true
	}
	for _, s := range success {
		if status == s {
			return true
		}
	}
	return false
}
