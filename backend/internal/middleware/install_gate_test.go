package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/config"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/app"
)

// setupTestEngine 构造带门禁的测试引擎，注册白名单内外的代表性路由。
func setupTestEngine(a *app.App) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(InstallGate(a))
	okHandler := func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) }

	r.GET("/api/install/status", okHandler)
	r.POST("/api/install/dbtest", okHandler)
	r.POST("/api/install/complete", okHandler)
	r.GET("/api/public/brand", okHandler)
	r.GET("/api/healthz", okHandler)
	r.GET("/api/auth/me", okHandler)     // 白名单外
	r.POST("/api/auth/login", okHandler) // 白名单外
	r.GET("/assets/logo.png", okHandler) // 非 /api/ 静态资源
	return r
}

func newTestApp(installed bool) *app.App {
	cfg := &config.Config{}
	cfg.Install.Installed = installed
	return app.New(cfg, "config.toml", "test")
}

func doReq(r *gin.Engine, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestInstallGateNotInstalled(t *testing.T) {
	r := setupTestEngine(newTestApp(false))

	// 白名单接口应放行
	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/install/status"},
		{"POST", "/api/install/dbtest"},
		{"POST", "/api/install/complete"},
		{"GET", "/api/public/brand"},
		{"GET", "/api/healthz"},
		{"GET", "/assets/logo.png"}, // 静态资源
	} {
		w := doReq(r, tc.method, tc.path)
		if w.Code != http.StatusOK {
			t.Errorf("未安装态 %s %s 应放行(200)，得到 %d", tc.method, tc.path, w.Code)
		}
	}

	// 白名单外接口应返回 409 not_installed
	for _, tc := range []struct{ method, path string }{
		{"POST", "/api/auth/login"},
		{"GET", "/api/auth/me"},
		{"GET", "/api/admin/notify/settings"},
	} {
		w := doReq(r, tc.method, tc.path)
		if w.Code != http.StatusConflict {
			t.Errorf("未安装态 %s %s 应返回 409，得到 %d", tc.method, tc.path, w.Code)
			continue
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("响应体不是 JSON: %s", w.Body.String())
		}
		if body["code"] != "not_installed" {
			t.Errorf("错误码应为 not_installed，得到 %v", body["code"])
		}
		if status, _ := body["status"].(float64); int(status) != 409 {
			t.Errorf("body.status 应为 409，得到 %v", body["status"])
		}
	}
}

func TestInstallGateInstalled(t *testing.T) {
	a := newTestApp(true)
	r := setupTestEngine(a)

	// 已安装态全部放行（认证由 AuthRequired 负责，不在门禁范围）
	for _, tc := range []struct{ method, path string }{
		{"POST", "/api/auth/login"},
		{"GET", "/api/auth/me"},
		{"GET", "/api/install/status"},
	} {
		w := doReq(r, tc.method, tc.path)
		if w.Code != http.StatusOK {
			t.Errorf("已安装态 %s %s 应放行(200)，得到 %d", tc.method, tc.path, w.Code)
		}
	}
}

func TestInstallGateHotSwitch(t *testing.T) {
	// 安装完成后不重启进程，门禁应立即放行（模拟安装器原子切换）
	a := newTestApp(false)
	r := setupTestEngine(a)

	if w := doReq(r, "POST", "/api/auth/login"); w.Code != http.StatusConflict {
		t.Fatalf("切换前应 409，得到 %d", w.Code)
	}
	a.Installed.Store(true)
	if w := doReq(r, "POST", "/api/auth/login"); w.Code != http.StatusOK {
		t.Fatalf("切换后应 200，得到 %d", w.Code)
	}
}
