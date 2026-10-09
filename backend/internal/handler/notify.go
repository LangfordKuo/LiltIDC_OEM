package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/errcode"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/response"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/service"
)

// NotifyHandler 处理后台通知配置/模板/测试发送接口（契约文档第 5 章）。
type NotifyHandler struct {
	svc *service.NotifyService
}

// NewNotify 构造通知 handler。
func NewNotify(svc *service.NotifyService) *NotifyHandler {
	return &NotifyHandler{svc: svc}
}

// GetSettings 处理 GET /api/admin/notify/settings。
func (h *NotifyHandler) GetSettings(c *gin.Context) {
	settings, err := h.svc.GetSettings(c.Request.Context())
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, settings)
}

// SaveSettings 处理 PUT /api/admin/notify/settings。
func (h *NotifyHandler) SaveSettings(c *gin.Context) {
	var req service.SaveNotifySettingsRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.SaveSettings(c.Request.Context(), &req); err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, gin.H{"saved": true})
}

// TestSMS 处理 POST /api/admin/notify/test-sms。
func (h *NotifyHandler) TestSMS(c *gin.Context) {
	var req service.TestSMSRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.TestSMS(c.Request.Context(), &req); err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, gin.H{"sent": true})
}

// TestEmail 处理 POST /api/admin/notify/test-email。
func (h *NotifyHandler) TestEmail(c *gin.Context) {
	var req service.TestEmailRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.TestEmail(c.Request.Context(), &req); err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, gin.H{"sent": true})
}

// SMSBalance 处理 GET /api/admin/notify/sms-balance。
func (h *NotifyHandler) SMSBalance(c *gin.Context) {
	sent, remain, err := h.svc.SMSBalance(c.Request.Context())
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, gin.H{"sent": sent, "remain": remain})
}

// ListTemplates 处理 GET /api/admin/notify/templates（支持 scene/channel/lang 过滤）。
func (h *NotifyHandler) ListTemplates(c *gin.Context) {
	list, err := h.svc.ListTemplates(c.Request.Context(), c.Query("scene"), c.Query("channel"), c.Query("lang"))
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, gin.H{"list": list})
}

// UpdateTemplate 处理 PUT /api/admin/notify/templates/:id。
func (h *NotifyHandler) UpdateTemplate(c *gin.Context) {
	id64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id64 == 0 {
		response.Fail(c, 400, errcode.CodeInvalidParam, "模板 id 必须是正整数")
		return
	}
	var req service.UpdateTemplateRequest
	if !bindJSON(c, &req) {
		return
	}
	tpl, err := h.svc.UpdateTemplate(c.Request.Context(), uint(id64), &req)
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, tpl)
}
