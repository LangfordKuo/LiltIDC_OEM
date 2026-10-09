package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/middleware"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/response"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/service"
)

// AuthHandler 处理管理员认证接口（契约文档第 4 章）。
type AuthHandler struct {
	svc *service.AuthService
}

// NewAuth 构造认证 handler。
func NewAuth(svc *service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

// Login 处理 POST /api/auth/login。
func (h *AuthHandler) Login(c *gin.Context) {
	var req service.LoginRequest
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

// Me 处理 GET /api/auth/me（需鉴权）。
func (h *AuthHandler) Me(c *gin.Context) {
	info, err := h.svc.Me(c.Request.Context(), middleware.CurrentAdminID(c))
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, info)
}
