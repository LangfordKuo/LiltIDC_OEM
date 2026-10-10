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
	upstreamH := handler.NewUpstream(service.NewUpstream(a))
	productH := handler.NewProduct(service.NewProduct(a))
	hostSyncH := handler.NewHostSync(service.NewHostSync(a))
	verifySvc := service.NewVerify(a)
	clientH := handler.NewClient(service.NewClient(a, verifySvc), verifySvc)
	// 批 3-β：商城/订单/余额 + 开通转发 worker（后台任务经 app.StartBackground 幂等启动）
	provisionSvc := service.NewProvision(a)
	a.SetBackgroundStarter(provisionSvc.Start)
	orderH := handler.NewOrder(service.NewOrder(a, provisionSvc), service.NewStore(a))
	balanceH := handler.NewBalance(service.NewBalance(a))

	// ---- 公共接口（未安装态白名单，见契约文档 0.5） ----
	r.GET("/api/healthz", healthH.Health)
	r.GET("/api/install/status", installH.Status)
	r.POST("/api/install/dbtest", installH.DBTest)
	r.POST("/api/install/complete", installH.Complete)
	r.GET("/api/public/brand", brandH.Get)

	// ---- 认证（受未安装门禁保护，无需 JWT） ----
	r.POST("/api/auth/login", authH.Login)

	// ---- 会员前台·免鉴权（批 3-α，契约第 12 章；受未安装门禁保护） ----
	r.GET("/api/client/captcha", clientH.Captcha)
	r.POST("/api/client/auth/send-code", clientH.SendCode)
	r.POST("/api/client/auth/register", clientH.Register)
	r.POST("/api/client/auth/login", clientH.Login)

	// ---- 商城前台·公开只读（批 3-β，契约 13.1；免鉴权可匿名浏览，受未安装门禁保护） ----
	r.GET("/api/store/products", orderH.StoreProducts)
	r.GET("/api/store/products/:id", orderH.StoreProductDetail)

	// ---- 上游 → 下游主机推送（批 2，契约 9.6） ----
	// 免 JWT（上游服务器调用），以魔方标准 MD5 签名验证代替登录态；
	// 注意：不在未安装门禁白名单内——未安装态直接 409（安全模型见契约文档）。
	r.POST("/api/upstream/host/sync", hostSyncH.Sync)

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

		// 上游对接（批 2，契约第 9 章；只读端点 + 商品导入，不开订单写操作）
		authed.GET("/admin/upstream/config", upstreamH.GetConfig)
		authed.PUT("/admin/upstream/config", upstreamH.SaveConfig)
		authed.POST("/admin/upstream/test", upstreamH.Test)
		authed.GET("/admin/upstream/credit", upstreamH.GetCredit)
		authed.POST("/admin/upstream/downstream-token/reset", upstreamH.ResetDownstreamToken)
		authed.GET("/admin/upstream/products", upstreamH.GetProducts)
		authed.GET("/admin/upstream/products/stock", upstreamH.GetStock)
		authed.POST("/admin/upstream/products/import", upstreamH.ImportProducts)
		authed.GET("/admin/upstream/hosts", upstreamH.GetUpstreamHosts)

		// 本地商品管理（批 2，契约第 10 章）
		authed.GET("/admin/products", productH.List)
		authed.GET("/admin/product-groups", productH.ListGroups)
		authed.PUT("/admin/products/:id", productH.Update)
		authed.PUT("/admin/products/:id/status", productH.UpdateStatus)
		authed.POST("/admin/products/sync-stock", productH.SyncStock)

		// 会员管理（批 3-α，契约 12.6）
		authed.GET("/admin/clients", clientH.AdminList)
		authed.PUT("/admin/clients/:id/status", clientH.AdminSetStatus)
		authed.POST("/admin/clients/:id/reset-password", clientH.AdminResetPassword)
		authed.DELETE("/admin/clients/:id", clientH.AdminDelete)

		// 会员余额人工调账（批 3-β，契约 14.2）
		authed.POST("/admin/clients/:id/balance", balanceH.AdminAdjust)

		// 订单管理（批 3-β，契约 13.5）
		authed.GET("/admin/orders", orderH.AdminListOrders)
		authed.GET("/admin/orders/:id", orderH.AdminOrderDetail)
		authed.POST("/admin/orders/:id/retry-provision", orderH.AdminRetryProvision)
		authed.POST("/admin/orders/:id/cancel-refund", orderH.AdminCancelRefund)
	}

	// ---- 会员前台·需会员 JWT（issuer 隔离，管理员 token 无效） ----
	clientAuthed := r.Group("/api/client", middleware.ClientAuthRequired(a))
	{
		clientAuthed.GET("/profile", clientH.Profile)
		clientAuthed.PUT("/profile", clientH.UpdateProfile)
		clientAuthed.POST("/change-password", clientH.ChangePassword)

		// 批 3-β：订单/余额/主机（契约 13.2-13.4、13.7、14.3）
		clientAuthed.POST("/orders", orderH.CreateOrder)
		clientAuthed.GET("/orders", orderH.ListOrders)
		clientAuthed.GET("/orders/:id", orderH.OrderDetail)
		clientAuthed.POST("/orders/:id/pay", orderH.PayOrder)
		clientAuthed.POST("/orders/:id/cancel", orderH.CancelOrder)
		clientAuthed.GET("/hosts", orderH.ClientHosts)
		clientAuthed.GET("/balance", balanceH.ClientBalance)
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
