package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/response"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/service"
)

// BrandHandler 处理公共品牌接口（契约文档第 2 章）。
type BrandHandler struct {
	svc *service.BrandService
}

// NewBrand 构造品牌 handler。
func NewBrand(svc *service.BrandService) *BrandHandler {
	return &BrandHandler{svc: svc}
}

// Get 处理 GET /api/public/brand：未安装态也可访问（安装页需要样式）。
func (h *BrandHandler) Get(c *gin.Context) {
	response.OK(c, h.svc.Get(c.Request.Context()))
}
