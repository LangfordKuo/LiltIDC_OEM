package middleware

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/config"
)

// CORS 返回跨域中间件；白名单来自配置 [server] cors_allow_origins。
//
// 生产环境前端与 API 同域（Nginx 托管静态产物 + 反代 /api），不依赖 CORS；
// 白名单主要服务开发期前后端分离（Vite dev server 跨端口访问）。
func CORS(allowOrigins []string) gin.HandlerFunc {
	if len(allowOrigins) == 0 {
		// 防御：手工构造的配置未经过 applyDefaults 时回落默认值
		// （gin-contrib/cors 对白名单为空会 panic）
		allowOrigins = config.DefaultCorsAllowOrigins()
	}
	return cors.New(cors.Config{
		AllowOrigins:     allowOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	})
}
