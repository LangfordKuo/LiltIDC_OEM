package notify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newFakeSmsbao 启动一个假的短信宝服务器，记录最后一次请求并返回指定响应体。
func newFakeSmsbao(t *testing.T, responseBody string, statusCode int) (*httptest.Server, *http.Request) {
	t.Helper()
	var lastReq *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastReq = r.Clone(r.Context())
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(responseBody))
	}))
	t.Cleanup(srv.Close)
	return srv, lastReq
}

func TestSmsbaoSendSuccess(t *testing.T) {
	var lastURL *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastURL = r.Clone(r.Context())
		_, _ = w.Write([]byte("0"))
	}))
	defer srv.Close()

	s := NewSmsbao(SmsbaoConfig{
		Username:    "testuser",
		PasswordMD5: "e10adc3949ba59abbe56e057f20f883e",
		Sign:        "岭云互联",
		BaseURL:     srv.URL,
	})
	err := s.Send(context.Background(), &Message{To: "13800138000,13900139000", Content: "您的验证码是 1234"})
	if err != nil {
		t.Fatalf("Send 应成功: %v", err)
	}
	if lastURL == nil {
		t.Fatal("mock 服务器未收到请求")
	}
	// 校验 URL 构造：路径 + 查询参数
	if lastURL.URL.Path != "/sms" {
		t.Errorf("请求路径错误: %s", lastURL.URL.Path)
	}
	q := lastURL.URL.Query()
	if q.Get("u") != "testuser" {
		t.Errorf("u 参数错误: %s", q.Get("u"))
	}
	if q.Get("p") != "e10adc3949ba59abbe56e057f20f883e" {
		t.Errorf("p 参数错误: %s", q.Get("p"))
	}
	if q.Get("m") != "13800138000,13900139000" {
		t.Errorf("m 参数错误: %s", q.Get("m"))
	}
	// URL decode 后内容应带【签名】前缀（验证 URL 编码与签名拼接）
	if got := q.Get("c"); got != "【岭云互联】您的验证码是 1234" {
		t.Errorf("c 参数错误: %s", got)
	}
	// 原始 query 中文必须被 URL 编码
	if strings.Contains(lastURL.URL.RawQuery, "岭云") {
		t.Errorf("内容未做 URL 编码: %s", lastURL.URL.RawQuery)
	}
}

func TestSmsbaoSendSignNotDuplicated(t *testing.T) {
	var lastURL *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastURL = r.Clone(r.Context())
		_, _ = w.Write([]byte("0"))
	}))
	defer srv.Close()

	s := NewSmsbao(SmsbaoConfig{Username: "u", PasswordMD5: "p", Sign: "岭云互联", BaseURL: srv.URL})
	// 内容已带签名前缀时不应重复添加
	err := s.Send(context.Background(), &Message{To: "13800138000", Content: "【岭云互联】已有签名"})
	if err != nil {
		t.Fatalf("Send 应成功: %v", err)
	}
	if got := lastURL.URL.Query().Get("c"); got != "【岭云互联】已有签名" {
		t.Errorf("签名被重复添加: %s", got)
	}
}

func TestSmsbaoSendErrorCodes(t *testing.T) {
	cases := []struct {
		resp     string
		wantText string
	}{
		{"41", "余额不足"},
		{"-2", "用户名或密码错误"},
		{"33", "IP 未通过验证"},
		{"50", "内容含敏感词"},
		{"99", "未知错误码: 99"},
		{" 0 \n", ""}, // 带空白的成功响应也应视为成功
	}
	for _, tc := range cases {
		srv, _ := newFakeSmsbao(t, tc.resp, http.StatusOK)
		s := NewSmsbao(SmsbaoConfig{Username: "u", PasswordMD5: "p", BaseURL: srv.URL})
		err := s.Send(context.Background(), &Message{To: "13800138000", Content: "hi"})
		if tc.wantText == "" {
			if err != nil {
				t.Errorf("响应 %q 应成功，得到: %v", tc.resp, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("响应 %q 应失败", tc.resp)
			continue
		}
		if !strings.Contains(err.Error(), tc.wantText) {
			t.Errorf("响应 %q 的错误信息应包含 %q，得到: %v", tc.resp, tc.wantText, err)
		}
	}
}

func TestSmsbaoSendHTTPError(t *testing.T) {
	srv, _ := newFakeSmsbao(t, "internal error", http.StatusInternalServerError)
	s := NewSmsbao(SmsbaoConfig{Username: "u", PasswordMD5: "p", BaseURL: srv.URL})
	err := s.Send(context.Background(), &Message{To: "13800138000", Content: "hi"})
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("HTTP 500 应报错，得到: %v", err)
	}
}

func TestSmsbaoQueryBalance(t *testing.T) {
	var lastPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastPath = r.URL.Path
		_, _ = w.Write([]byte("0\n100,500"))
	}))
	defer srv.Close()

	s := NewSmsbao(SmsbaoConfig{Username: "u", PasswordMD5: "p", BaseURL: srv.URL})
	sent, remain, err := s.QueryBalance(context.Background())
	if err != nil {
		t.Fatalf("QueryBalance 失败: %v", err)
	}
	if lastPath != "/query" {
		t.Errorf("余额查询路径错误: %s", lastPath)
	}
	if sent != 100 || remain != 500 {
		t.Errorf("余额解析错误: sent=%d remain=%d", sent, remain)
	}
}

func TestSmsbaoQueryBalanceError(t *testing.T) {
	srv, _ := newFakeSmsbao(t, "30\n", http.StatusOK)
	s := NewSmsbao(SmsbaoConfig{Username: "u", PasswordMD5: "p", BaseURL: srv.URL})
	_, _, err := s.QueryBalance(context.Background())
	if err == nil || !strings.Contains(err.Error(), "密码错误") {
		t.Errorf("状态码 30 应映射为密码错误，得到: %v", err)
	}
}

func TestSmsbaoQueryBalanceBadFormat(t *testing.T) {
	srv, _ := newFakeSmsbao(t, "0\nbad-format", http.StatusOK)
	s := NewSmsbao(SmsbaoConfig{Username: "u", PasswordMD5: "p", BaseURL: srv.URL})
	if _, _, err := s.QueryBalance(context.Background()); err == nil {
		t.Error("第二行格式异常应报错")
	}
}

func TestMapSmsbaoCode(t *testing.T) {
	if got := MapSmsbaoCode("0"); got != "未知错误码: 0" {
		// "0" 在发送流程中不会走到映射函数，但映射函数本身应返回未知错误码
		t.Errorf("映射结果意外: %s", got)
	}
	if got := MapSmsbaoCode("41"); got != "余额不足" {
		t.Errorf("41 映射错误: %s", got)
	}
	if got := MapSmsbaoCode("-4"); got != "手机号数量超过限制" {
		t.Errorf("-4 映射错误: %s", got)
	}
}
