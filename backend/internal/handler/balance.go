package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/middleware"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/response"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/service"
)

// BalanceHandler 处理余额域接口（契约文档第 14 章）：
// 会员 /api/client/balance 与后台 /api/admin/clients/:id/balance。
type BalanceHandler struct {
	svc *service.BalanceService
}

// NewBalance 构造余额 handler。
func NewBalance(svc *service.BalanceService) *BalanceHandler {
	return &BalanceHandler{svc: svc}
}

// ClientBalance 处理 GET /api/client/balance：余额 + 流水分页。
func (h *BalanceHandler) ClientBalance(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	view, err := h.svc.ClientBalance(c.Request.Context(), middleware.CurrentClientID(c),
		page, pageSize, detectLang(c))
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, view)
}

// AdminAdjust 处理 POST /api/admin/clients/:id/balance：人工充值/扣款。
func (h *BalanceHandler) AdminAdjust(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req service.AdminAdjustRequest
	if !bindJSON(c, &req) {
		return
	}
	view, err := h.svc.AdminAdjust(c.Request.Context(), middleware.CurrentAdminID(c),
		id, &req, detectLang(c))
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, view)
}
