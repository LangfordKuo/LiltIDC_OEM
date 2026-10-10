package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/middleware"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/response"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/service"
)

// OrderHandler 处理商城前台商品与订单接口（契约文档第 13 章）：
// 公开 /api/store/*、会员 /api/client/orders|hosts、后台 /api/admin/orders。
type OrderHandler struct {
	orders *service.OrderService
	store  *service.StoreService
}

// NewOrder 构造订单 handler。
func NewOrder(orders *service.OrderService, store *service.StoreService) *OrderHandler {
	return &OrderHandler{orders: orders, store: store}
}

// ---- 商城前台（公开，免鉴权） ----

// StoreProducts 处理 GET /api/store/products：上架商品按分组列表。
func (h *OrderHandler) StoreProducts(c *gin.Context) {
	groups, err := h.store.StoreList(c.Request.Context())
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, gin.H{"groups": groups})
}

// StoreProductDetail 处理 GET /api/store/products/:id：商品详情（含 HTML 描述原文）。
func (h *OrderHandler) StoreProductDetail(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	view, err := h.store.StoreDetail(c.Request.Context(), id)
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, view)
}

// ---- 会员端（会员 JWT） ----

// CreateOrder 处理 POST /api/client/orders：单商品立即购买下单。
func (h *OrderHandler) CreateOrder(c *gin.Context) {
	var req service.CreateOrderRequest
	if !bindJSON(c, &req) {
		return
	}
	view, err := h.orders.Create(c.Request.Context(), middleware.CurrentClientID(c), &req, detectLang(c))
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, view)
}

// PayOrder 处理 POST /api/client/orders/:id/pay：余额支付。
func (h *OrderHandler) PayOrder(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req service.PayOrderRequest
	if !bindJSON(c, &req) {
		return
	}
	view, err := h.orders.Pay(c.Request.Context(), middleware.CurrentClientID(c), id, &req, detectLang(c))
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, view)
}

// CancelOrder 处理 POST /api/client/orders/:id/cancel：取消待支付订单。
func (h *OrderHandler) CancelOrder(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	view, err := h.orders.Cancel(c.Request.Context(), middleware.CurrentClientID(c), id, detectLang(c))
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, view)
}

// ListOrders 处理 GET /api/client/orders：会员自己的订单列表。
func (h *OrderHandler) ListOrders(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	view, err := h.orders.ClientList(c.Request.Context(), middleware.CurrentClientID(c),
		c.Query("status"), page, pageSize, detectLang(c))
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, view)
}

// OrderDetail 处理 GET /api/client/orders/:id：会员订单详情。
func (h *OrderHandler) OrderDetail(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	view, err := h.orders.ClientDetail(c.Request.Context(), middleware.CurrentClientID(c), id, detectLang(c))
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, view)
}

// ClientHosts 处理 GET /api/client/hosts：我的主机列表。
func (h *OrderHandler) ClientHosts(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	view, err := h.orders.ClientHosts(c.Request.Context(), middleware.CurrentClientID(c), page, pageSize)
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, view)
}

// ---- 后台（管理员 JWT） ----

// AdminListOrders 处理 GET /api/admin/orders：后台订单列表（状态筛选/关键词搜索）。
func (h *OrderHandler) AdminListOrders(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	view, err := h.orders.AdminList(c.Request.Context(), c.Query("status"), c.Query("keyword"),
		page, pageSize, detectLang(c))
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, view)
}

// AdminOrderDetail 处理 GET /api/admin/orders/:id：后台订单详情（含关联流水）。
func (h *OrderHandler) AdminOrderDetail(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	view, err := h.orders.AdminDetail(c.Request.Context(), id, detectLang(c))
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, view)
}

// AdminRetryProvision 处理 POST /api/admin/orders/:id/retry-provision：重试开通。
func (h *OrderHandler) AdminRetryProvision(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	view, err := h.orders.AdminRetryProvision(c.Request.Context(), id, detectLang(c))
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, view)
}

// AdminCancelRefund 处理 POST /api/admin/orders/:id/cancel-refund：取消并退款。
func (h *OrderHandler) AdminCancelRefund(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req struct {
		Remark string `json:"remark"`
	}
	// body 可选（无备注时允许空 body）
	_ = c.ShouldBindJSON(&req)
	view, err := h.orders.AdminCancelRefund(c.Request.Context(), middleware.CurrentAdminID(c),
		id, req.Remark, detectLang(c))
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, view)
}
