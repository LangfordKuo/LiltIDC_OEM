package handler

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/middleware"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/errcode"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/response"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/service"
)

// ClientHandler 处理会员域接口（契约文档第 12 章）：
// 前台 /api/client/*（会员 JWT）与后台 /api/admin/clients（管理员 JWT）。
type ClientHandler struct {
	svc    *service.ClientService
	verify *service.VerifyService
}

// NewClient 构造会员 handler。
func NewClient(svc *service.ClientService, v *service.VerifyService) *ClientHandler {
	return &ClientHandler{svc: svc, verify: v}
}

// detectLang 从 Accept-Language 提取模板语言（zh 前缀→zh-CN，en→en-US，其它→zh-CN）。
func detectLang(c *gin.Context) string {
	if strings.HasPrefix(strings.ToLower(c.GetHeader("Accept-Language")), "en") {
		return "en-US"
	}
	return "zh-CN"
}

// Captcha 处理 GET /api/client/captcha：生成图形验证码。
func (h *ClientHandler) Captcha(c *gin.Context) {
	view, err := h.verify.NewCaptcha(c.Request.Context())
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, view)
}

// SendCode 处理 POST /api/client/auth/send-code。
func (h *ClientHandler) SendCode(c *gin.Context) {
	var req service.SendCodeRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.SendCode(c.Request.Context(), &req, c.ClientIP(), detectLang(c)); err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, gin.H{"sent": true, "expires_in": 600})
}

// Register 处理 POST /api/client/auth/register。
func (h *ClientHandler) Register(c *gin.Context) {
	var req service.RegisterRequest
	if !bindJSON(c, &req) {
		return
	}
	result, err := h.svc.Register(c.Request.Context(), &req)
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, result)
}

// Login 处理 POST /api/client/auth/login。
func (h *ClientHandler) Login(c *gin.Context) {
	var req service.ClientLoginRequest
	if !bindJSON(c, &req) {
		return
	}
	result, err := h.svc.Login(c.Request.Context(), &req)
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, result)
}

// Profile 处理 GET /api/client/profile（会员 JWT）。
func (h *ClientHandler) Profile(c *gin.Context) {
	view, err := h.svc.Profile(c.Request.Context(), middleware.CurrentClientID(c))
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, view)
}

// UpdateProfile 处理 PUT /api/client/profile。
func (h *ClientHandler) UpdateProfile(c *gin.Context) {
	var req service.UpdateProfileRequest
	if !bindJSON(c, &req) {
		return
	}
	view, err := h.svc.UpdateProfile(c.Request.Context(), middleware.CurrentClientID(c), &req)
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, view)
}

// ChangePassword 处理 POST /api/client/change-password。
func (h *ClientHandler) ChangePassword(c *gin.Context) {
	var req service.ChangePasswordRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.ChangePassword(c.Request.Context(), middleware.CurrentClientID(c), &req); err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, gin.H{"changed": true})
}

// ---- 后台管理（管理员 JWT） ----

// AdminList 处理 GET /api/admin/clients。
func (h *ClientHandler) AdminList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	view, err := h.svc.AdminList(c.Request.Context(), c.Query("keyword"), c.Query("status"),
		page, pageSize, detectLang(c))
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, view)
}

// AdminSetStatus 处理 PUT /api/admin/clients/:id/status。
func (h *ClientHandler) AdminSetStatus(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req struct {
		Status *int8 `json:"status"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.Status == nil {
		response.Fail(c, 400, errcode.CodeInvalidParam, "status 必填")
		return
	}
	view, err := h.svc.AdminSetStatus(c.Request.Context(), id, *req.Status, detectLang(c))
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, view)
}

// AdminResetPassword 处理 POST /api/admin/clients/:id/reset-password。
func (h *ClientHandler) AdminResetPassword(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req struct {
		NewPassword string `json:"new_password"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.AdminResetPassword(c.Request.Context(), middleware.CurrentAdminID(c), id, req.NewPassword); err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, gin.H{"reset": true})
}

// AdminDelete 处理 DELETE /api/admin/clients/:id（软删除，幂等）。
func (h *ClientHandler) AdminDelete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.svc.AdminDelete(c.Request.Context(), id); err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}
