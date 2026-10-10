package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeUpstream 是可编程的假上游（zjmf 协议同形）：
//   - /zjmf_api_login：账号密码正确则签发第 N 代 jwt（loginCount 自增）；
//   - 业务端点：Authorization 必须携带「当前代」jwt，否则回 {"status":405}（模拟失效）。
type fakeUpstream struct {
	server       *httptest.Server
	username     string
	password     string
	currentJWT   string
	loginCount   atomic.Int32
	bizCount     atomic.Int32
	bizAlways405 bool
	bizHandler   func(w http.ResponseWriter, r *http.Request) // 通过鉴权后的业务响应
	bizDelay     time.Duration
}

func newFakeUpstream() *fakeUpstream {
	f := &fakeUpstream{username: "16600000000", password: "test-key"}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/zjmf_api_login":
			f.loginCount.Add(1)
			body, _ := io.ReadAll(r.Body)
			var req struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}
			_ = json.Unmarshal(body, &req)
			w.Header().Set("Content-Type", "application/json")
			if req.Username == f.username && req.Password == f.password {
				f.currentJWT = "jwt-gen-" + string(rune('0'+f.loginCount.Load()))
				// 真实形状：jwt 在顶层（唯一例外），带 is_aff 噪声字段
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jwt": f.currentJWT, "status": 200, "msg": "鉴权成功", "is_aff": "1",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jwt": "", "status": 403, "msg": "用户名或密码错误",
			})
		default:
			f.bizCount.Add(1)
			if f.bizDelay > 0 {
				time.Sleep(f.bizDelay)
			}
			w.Header().Set("Content-Type", "application/json")
			auth := r.Header.Get("Authorization")
			if f.bizAlways405 || auth != "Bearer "+f.currentJWT || f.currentJWT == "" {
				_ = json.NewEncoder(w).Encode(map[string]any{"status": 405, "msg": "请登陆后再试"})
				return
			}
			if f.bizHandler != nil {
				f.bizHandler(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": 200})
		}
	}))
	return f
}

func (f *fakeUpstream) close() { f.server.Close() }

func (f *fakeUpstream) client() *Client {
	return NewClient(Credentials{
		Hostname: f.server.URL,
		Username: f.username,
		APIKey:   f.password,
		Timeout:  5 * time.Second,
	})
}

func TestLoginSuccessAndJWTCache(t *testing.T) {
	f := newFakeUpstream()
	defer f.close()
	c := f.client()

	if err := c.Login(context.Background()); err != nil {
		t.Fatalf("Login 失败: %v", err)
	}
	if c.JWT() == "" {
		t.Fatal("登录后 jwt 应被缓存")
	}
	first := c.JWT()

	// 再发业务请求：应复用缓存 jwt，不再登录
	if err := c.do(context.Background(), "GET", "/cart/credit", nil, nil); err != nil {
		t.Fatalf("业务请求失败: %v", err)
	}
	if got := f.loginCount.Load(); got != 1 {
		t.Errorf("jwt 缓存未命中：登录次数应为 1，实际 %d", got)
	}
	if c.JWT() != first {
		t.Error("jwt 缓存被意外更换")
	}
}

func TestLoginWrongPassword(t *testing.T) {
	f := newFakeUpstream()
	defer f.close()
	c := NewClient(Credentials{Hostname: f.server.URL, Username: f.username, APIKey: "wrong-key"})

	err := c.Login(context.Background())
	if err == nil {
		t.Fatal("密码错误应登录失败")
	}
	if !errors.Is(err, ErrAuthFailed) {
		t.Errorf("应返回 ErrAuthFailed，得到: %v", err)
	}
	if !strings.Contains(err.Error(), "用户名或密码错误") {
		t.Errorf("错误信息应携带上游 msg: %v", err)
	}
}

func TestLoginNotConfigured(t *testing.T) {
	c := NewClient(Credentials{})
	if err := c.Login(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("空凭据应返回 ErrNotConfigured，得到: %v", err)
	}
}

func TestDoReauthOnceSuccess(t *testing.T) {
	// 场景：已登录持有 gen1 jwt → 上游使其失效 → do 收 405 → 重登（gen2）→ 重试成功
	f := newFakeUpstream()
	defer f.close()
	c := f.client()
	ctx := context.Background()

	if err := c.Login(ctx); err != nil {
		t.Fatalf("首次登录失败: %v", err)
	}
	f.currentJWT = "invalidated" // 模拟上游侧 jwt 失效

	var credit Credit
	// bizHandler 返回真实信封形状的 credit 数据（payload 嵌在 data 键下）
	f.bizHandler = func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": 200,
			"msg":    "请求成功",
			"data": map[string]any{
				"credit":   "57664.50",
				"currency": map[string]string{"code": "CNY", "prefix": "¥", "suffix": "元"},
			},
			"is_aff": "1",
		})
	}
	if err := c.do(ctx, "GET", "/cart/credit", nil, &credit); err != nil {
		t.Fatalf("405 重登重试应成功: %v", err)
	}
	if credit.Credit != "57664.50" || credit.Currency.Code != "CNY" {
		t.Errorf("credit 解析错误: %+v", credit)
	}
	if got := f.loginCount.Load(); got != 2 {
		t.Errorf("登录次数应为 2（首登+重登），实际 %d", got)
	}
}

func TestDoReauthLoopProtection(t *testing.T) {
	// 场景：业务端点始终 405 → 重登一次后仍 405 → 必须放弃（防死循环）
	f := newFakeUpstream()
	defer f.close()
	f.bizAlways405 = true
	c := f.client()

	err := c.do(context.Background(), "GET", "/cart/credit", nil, nil)
	if err == nil {
		t.Fatal("持续 405 应最终报错")
	}
	if !errors.Is(err, ErrReauth) {
		t.Errorf("应返回 ErrReauth，得到: %v", err)
	}
	if got := f.loginCount.Load(); got != 2 {
		t.Errorf("登录应恰好 2 次（首登+重登一次），实际 %d", got)
	}
	if got := f.bizCount.Load(); got != 2 {
		t.Errorf("业务请求应恰好 2 次（重试一次），实际 %d", got)
	}
}

func TestDoAutoLoginOnEmptyJWT(t *testing.T) {
	// 未显式 Login 直接 do：应自动登录后成功
	f := newFakeUpstream()
	defer f.close()
	c := f.client()
	if err := c.do(context.Background(), "GET", "/cart/credit", nil, nil); err != nil {
		t.Fatalf("do 应自动登录: %v", err)
	}
	if got := f.loginCount.Load(); got != 1 {
		t.Errorf("应自动登录 1 次，实际 %d", got)
	}
}

func TestRequestTimeout(t *testing.T) {
	f := newFakeUpstream()
	defer f.close()
	f.bizDelay = 400 * time.Millisecond
	c := NewClient(Credentials{
		Hostname: f.server.URL, Username: f.username, APIKey: f.password,
		Timeout: 100 * time.Millisecond, // 小于业务延迟
	})
	// 登录本身也会被超时覆盖（http.Client.Timeout 全流程生效）
	err := c.do(context.Background(), "GET", "/cart/credit", nil, nil)
	if err == nil {
		t.Fatal("超时应报错")
	}
	if !strings.Contains(err.Error(), "上游") {
		t.Errorf("超时错误应可读: %v", err)
	}
}

func TestDoHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	c := NewClient(Credentials{Hostname: srv.URL, Username: "u", APIKey: "k"})
	// 登录即 502
	err := c.Login(context.Background())
	if err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Errorf("应报 HTTP 502，得到: %v", err)
	}
}

func TestDoNonJSONResponse(t *testing.T) {
	// 模拟 WAF 拦截返回 HTML 页
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<!DOCTYPE html><html>WAF blocked</html>"))
	}))
	defer srv.Close()
	c := NewClient(Credentials{Hostname: srv.URL, Username: "u", APIKey: "k"})
	err := c.Login(context.Background())
	if err == nil || !strings.Contains(err.Error(), "不是合法 JSON") {
		t.Errorf("HTML 响应应报可读错误，得到: %v", err)
	}
}

func TestDoBusinessError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/zjmf_api_login" {
			_ = json.NewEncoder(w).Encode(map[string]any{"jwt": "j", "status": 200})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": 400, "msg": "参数错误"})
	}))
	defer srv.Close()
	c := NewClient(Credentials{Hostname: srv.URL, Username: "u", APIKey: "k"})
	err := c.do(context.Background(), "GET", "/cart/credit", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "参数错误") {
		t.Errorf("业务错误应携带上游 msg，得到: %v", err)
	}
}
