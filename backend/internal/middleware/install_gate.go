// Package middleware 提供 HTTP 中间件：未安装门禁、JWT 认证、CORS、请求日志、panic 恢复。
package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/app"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/errcode"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/response"
)

// installWhitelist 是未安装态仍可访问的 API 路径（精确匹配）。
// 见 docs/api-contract.md 第 0.5 节。
var installWhitelist = map[string]bool{
	"/api/install/status":   true,
	"/api/install/dbtest":   true,
	"/api/install/complete": true,
	"/api/public/brand":     true,
	"/api/healthz":          true,
}

// InstallGate 是未安装门禁：
//   - 已安装：全部放行；
//   - 未安装：白名单 API 与非 /api/ 前缀路径（静态资源）放行，
//     其余返回 409 {"status":409,"code":"not_installed"}，前端据此跳安装页。
func InstallGate(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		if a.IsInstalled() {
			c.Next()
			return
		}
		path := c.Request.URL.Path
		// 静态资源放行（生产由 Nginx 托管，开发由 Vite 托管；
		// 后端 [server] static_dir 托管时同样不经过门禁）
		if !strings.HasPrefix(path, "/api/") {
			c.Next()
			return
		}
		if installWhitelist[path] {
			c.Next()
			return
		}
		response.AbortFail(c, 409, errcode.CodeNotInstalled, "系统尚未安装")
	}
}
