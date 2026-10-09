package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/app"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/response"
)

// HealthHandler 处理探活接口（契约文档第 3 章）。
type HealthHandler struct {
	app *app.App
}

// NewHealth 构造探活 handler。
func NewHealth(a *app.App) *HealthHandler {
	return &HealthHandler{app: a}
}

// Health 处理 GET /api/healthz：供 Nginx/运维探活，未安装态可访问。
func (h *HealthHandler) Health(c *gin.Context) {
	response.OK(c, gin.H{
		"ok":        true,
		"installed": h.app.IsInstalled(),
		"version":   h.app.Version,
	})
}
