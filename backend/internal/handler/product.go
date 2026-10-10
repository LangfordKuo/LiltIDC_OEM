package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/errcode"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/response"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/service"
)

// ProductHandler 处理后台·本地商品管理接口（契约文档第 10 章）。
type ProductHandler struct {
	svc *service.ProductService
}

// NewProduct 构造商品 handler。
func NewProduct(svc *service.ProductService) *ProductHandler {
	return &ProductHandler{svc: svc}
}

// parseID 解析路径参数 :id（正整数）。
func parseID(c *gin.Context) (uint, bool) {
	id64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id64 == 0 {
		response.Fail(c, 400, errcode.CodeInvalidParam, "id 必须是正整数")
		return 0, false
	}
	return uint(id64), true
}

// List 处理 GET /api/admin/products?group_id=&page=&page_size=。
func (h *ProductHandler) List(c *gin.Context) {
	groupID, _ := strconv.ParseUint(c.Query("group_id"), 10, 64)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	view, err := h.svc.List(c.Request.Context(), uint(groupID), page, pageSize)
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, view)
}

// ListGroups 处理 GET /api/admin/product-groups。
func (h *ProductHandler) ListGroups(c *gin.Context) {
	list, err := h.svc.ListGroups(c.Request.Context())
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, gin.H{"list": list})
}

// Update 处理 PUT /api/admin/products/:id。
func (h *ProductHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req service.UpdateProductRequest
	if !bindJSON(c, &req) {
		return
	}
	view, err := h.svc.Update(c.Request.Context(), id, &req)
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, view)
}

// UpdateStatus 处理 PUT /api/admin/products/:id/status。
func (h *ProductHandler) UpdateStatus(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req service.UpdateStatusRequest
	if !bindJSON(c, &req) {
		return
	}
	view, err := h.svc.UpdateStatus(c.Request.Context(), id, &req)
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, view)
}

// SyncStock 处理 POST /api/admin/products/sync-stock：批量从上游 proinfo 同步库存。
func (h *ProductHandler) SyncStock(c *gin.Context) {
	result, err := h.svc.SyncStock(c.Request.Context())
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, result)
}
