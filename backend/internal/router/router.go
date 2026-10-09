// Package router 负责路由组装与依赖注入（handler/service 在此构造）。
// 路由清单与契约见 docs/api-contract.md。
package router

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/handler"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/middleware"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/app"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/errcode"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/response"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/service"
)

// New 构造完整的 gin 引擎（中间件 + 全部路由）。
func New(a *app.App) *gin.Engine {
	cfg := a.Cfg()
	gin.SetMode(cfg.Server.Mode)
	r := gin.New()

	// 全局中间件顺序：请求日志 → panic 恢复 → CORS → 未安装门禁
	r.Use(middleware.RequestLog(), middleware.Recovery(),
		middleware.CORS(cfg.Server.CorsAllowOrigins), middleware.InstallGate(a))

	// ---- 依赖注入 ----
	installH := handler.NewInstall(service.NewInstall(a))
	authH := handler.NewAuth(service.NewAuth(a))
	brandH := handler.NewBrand(service.NewBrand(a))
	healthH := handler.NewHealth(a)
	notifyH := handler.NewNotify(service.NewNotify(a))

	// ---- 公共接口（未安装态白名单，见契约文档 0.5） ----
	r.GET("/api/healthz", healthH.Health)
	r.GET("/api/install/status", installH.Status)
	r.POST("/api/install/dbtest", installH.DBTest)
	r.POST("/api/install/complete", installH.Complete)
	r.GET("/api/public/brand", brandH.Get)

	// ---- 认证（受未安装门禁保护，无需 JWT） ----
	r.POST("/api/auth/login", authH.Login)

	// ---- 管理后台（需 JWT 认证） ----
	authed := r.Group("/api", middleware.AuthRequired(a))
	{
		authed.GET("/auth/me", authH.Me)

		// 通知配置与模板（批 1：仅基础设施与配置界面，不做业务触发）
		authed.GET("/admin/notify/settings", notifyH.GetSettings)
		authed.PUT("/admin/notify/settings", notifyH.SaveSettings)
		authed.POST("/admin/notify/test-sms", notifyH.TestSMS)
		authed.POST("/admin/notify/test-email", notifyH.TestEmail)
		authed.GET("/admin/notify/sms-balance", notifyH.SMSBalance)
		authed.GET("/admin/notify/templates", notifyH.ListTemplates)
		authed.PUT("/admin/notify/templates/:id", notifyH.UpdateTemplate)
	}

	// ---- 静态资源与 404 ----
	setupNoRoute(r, cfg.Server.StaticDir)
	return r
}

// setupNoRoute 配置未匹配路由的处理：
//   - [server] static_dir 有效时托管前端静态产物（SPA history 回退到 index.html）；
//   - 其余情况返回统一 404 JSON。
func setupNoRoute(r *gin.Engine, staticDir string) {
	base := ""
	if staticDir != "" {
		if abs, err := filepath.Abs(staticDir); err == nil {
			if fi, statErr := os.Stat(abs); statErr == nil && fi.IsDir() {
				base = abs
			}
		}
		if base == "" {
			slog.Warn("[server] static_dir 不存在或不是目录，跳过静态托管", "dir", staticDir)
		}
	}
	r.NoRoute(func(c *gin.Context) {
		if base != "" && c.Request.Method == http.MethodGet {
			serveSPA(c, base)
			return
		}
		response.Fail(c, 404, errcode.CodeNotFound, "接口不存在")
	})
}

// serveSPA 托管前端静态产物：命中文件直接返回，否则回退 index.html（前端路由接管）。
// 含路径穿越防护（Clean + 前缀校验双保险）。
func serveSPA(c *gin.Context, base string) {
	urlPath := c.Request.URL.Path
	if strings.HasPrefix(urlPath, "/api/") {
		response.Fail(c, 404, errcode.CodeNotFound, "接口不存在")
		return
	}
	target := filepath.Join(base, filepath.Clean("/"+urlPath))
	if target != base && !strings.HasPrefix(target, base+string(filepath.Separator)) {
		response.Fail(c, 404, errcode.CodeNotFound, "资源不存在")
		return
	}
	if fi, err := os.Stat(target); err == nil && !fi.IsDir() {
		c.File(target)
		return
	}
	index := filepath.Join(base, "index.html")
	if _, err := os.Stat(index); err == nil {
		c.File(index)
		return
	}
	response.Fail(c, 404, errcode.CodeNotFound, "资源不存在")
}
