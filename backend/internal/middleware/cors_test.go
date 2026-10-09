package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/config"
)

// newCORSEngine 构造仅含 CORS 中间件的测试引擎。
func newCORSEngine(origins []string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS(origins))
	r.GET("/api/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	return r
}

// doPreflight 发起 CORS 预检请求。
func doPreflight(r *gin.Engine, origin string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodOptions, "/api/ping", nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "content-type,authorization")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCORSDefaultWhitelist(t *testing.T) {
	// 未配置时 config.applyDefaults 会给出的默认白名单
	r := newCORSEngine(config.DefaultCorsAllowOrigins())
	for _, origin := range []string{"http://localhost:5173", "http://127.0.0.1:5173"} {
		w := doPreflight(r, origin)
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Errorf("默认白名单应允许 %s，Allow-Origin=%q（status=%d）", origin, got, w.Code)
		}
		if w.Header().Get("Access-Control-Allow-Credentials") != "true" {
			t.Errorf("%s 预检应带 Allow-Credentials", origin)
		}
	}
}

func TestCORSCustomOrigin(t *testing.T) {
	r := newCORSEngine([]string{"https://console.example.com"})

	// 配置的自定义源：预检应放行并回显
	w := doPreflight(r, "https://console.example.com")
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://console.example.com" {
		t.Errorf("自定义源预检失败，Allow-Origin=%q status=%d", got, w.Code)
	}

	// 非白名单源：不应回显 Allow-Origin（gin-contrib/cors 预检直接 403）
	w2 := doPreflight(r, "http://localhost:5173")
	if got := w2.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("非白名单源不应获得 Allow-Origin，得到 %q", got)
	}
	if w2.Code != http.StatusForbidden {
		t.Errorf("非白名单源预检应 403，得到 %d", w2.Code)
	}
}

func TestCORSEmptyFallbackNoPanic(t *testing.T) {
	// 空白名单不应 panic（gin-contrib/cors 原生会 panic），应回落默认值
	r := newCORSEngine(nil)
	w := doPreflight(r, "http://localhost:5173")
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("空白名单应回落默认，Allow-Origin=%q status=%d", got, w.Code)
	}
}

func TestDefaultCorsAllowOriginsIsolated(t *testing.T) {
	// 每次返回独立切片，防止调用方修改污染默认值
	a := config.DefaultCorsAllowOrigins()
	a[0] = "mutated"
	b := config.DefaultCorsAllowOrigins()
	if b[0] != "http://localhost:5173" {
		t.Errorf("默认值被污染: %v", b)
	}
}
