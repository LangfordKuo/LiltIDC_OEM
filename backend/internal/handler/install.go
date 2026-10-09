package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/response"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/service"
)

// InstallHandler 处理安装向导接口（契约文档第 1 章）。
type InstallHandler struct {
	svc *service.InstallService
}

// NewInstall 构造安装 handler。
func NewInstall(svc *service.InstallService) *InstallHandler {
	return &InstallHandler{svc: svc}
}

// Status 处理 GET /api/install/status：返回安装状态（未鉴权可访问）。
func (h *InstallHandler) Status(c *gin.Context) {
	response.OK(c, gin.H{"installed": h.svc.Status()})
}

// DBTest 处理 POST /api/install/dbtest：测试数据库连通性与版本。
func (h *InstallHandler) DBTest(c *gin.Context) {
	var req service.DBTestRequest
	if !bindJSON(c, &req) {
		return
	}
	result, err := h.svc.DBTest(c.Request.Context(), &req)
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, result)
}

// Complete 处理 POST /api/install/complete：一次性完成安装。
// 已安装态返回 409 already_installed（防重装攻击）。
func (h *InstallHandler) Complete(c *gin.Context) {
	var req service.InstallRequest
	if !bindJSON(c, &req) {
		return
	}
	result, err := h.svc.Complete(c.Request.Context(), &req)
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, result)
}
