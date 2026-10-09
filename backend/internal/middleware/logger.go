package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestLog 是结构化请求日志中间件（slog 输出：方法/路径/状态/耗时/来源）。
func RequestLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery
		c.Next()

		attrs := []any{
			"method", c.Request.Method,
			"path", path,
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
			"ip", c.ClientIP(),
		}
		if query != "" {
			attrs = append(attrs, "query", query)
		}
		if len(c.Errors) > 0 {
			attrs = append(attrs, "errors", c.Errors.String())
			slog.Error("请求处理有错误", attrs...)
			return
		}
		status := c.Writer.Status()
		if status >= 500 {
			slog.Error("请求", attrs...)
		} else if status >= 400 {
			slog.Warn("请求", attrs...)
		} else {
			slog.Info("请求", attrs...)
		}
	}
}
