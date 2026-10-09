package middleware

import (
	"log/slog"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/errcode"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/response"
)

// Recovery 是统一错误处理中间件：捕获 panic，记录堆栈日志，
// 对外输出 500 internal_error（不泄露内部细节）。
func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		slog.Error("panic 恢复",
			"path", c.Request.URL.Path,
			"method", c.Request.Method,
			"panic", recovered,
			"stack", string(debug.Stack()))
		response.AbortFail(c, 500, errcode.CodeInternalError, "服务器内部错误")
	})
}
