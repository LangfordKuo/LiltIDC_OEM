package handler

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/errcode"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/response"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/service"
)

// UpstreamHandler 处理后台·上游对接接口（契约文档第 9 章）。
type UpstreamHandler struct {
	svc *service.UpstreamService
}

// NewUpstream 构造上游 handler。
func NewUpstream(svc *service.UpstreamService) *UpstreamHandler {
	return &UpstreamHandler{svc: svc}
}

// GetConfig 处理 GET /api/admin/upstream/config。
func (h *UpstreamHandler) GetConfig(c *gin.Context) {
	view, err := h.svc.GetConfig(c.Request.Context())
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, view)
}

// SaveConfig 处理 PUT /api/admin/upstream/config（api_key 留空 = 不修改）。
func (h *UpstreamHandler) SaveConfig(c *gin.Context) {
	var req service.SaveUpstreamConfigRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.SaveConfig(c.Request.Context(), &req); err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, gin.H{"saved": true})
}

// Test 处理 POST /api/admin/upstream/test：真实调上游登录 + 余额（只读）。
func (h *UpstreamHandler) Test(c *gin.Context) {
	result, err := h.svc.TestConnection(c.Request.Context())
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, result)
}

// GetCredit 处理 GET /api/admin/upstream/credit。
func (h *UpstreamHandler) GetCredit(c *gin.Context) {
	view, err := h.svc.GetCredit(c.Request.Context())
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, view)
}

// ResetDownstreamToken 处理 POST /api/admin/upstream/downstream-token/reset。
func (h *UpstreamHandler) ResetDownstreamToken(c *gin.Context) {
	token, err := h.svc.ResetDownstreamToken(c.Request.Context())
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, gin.H{"downstream_token": token})
}

// GetProducts 处理 GET /api/admin/upstream/products：上游商品树 + 本地导入状态。
func (h *UpstreamHandler) GetProducts(c *gin.Context) {
	groups, err := h.svc.GetProducts(c.Request.Context())
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, gin.H{"groups": groups})
}

// GetStock 处理 GET /api/admin/upstream/products/stock?pids=1,2。
func (h *UpstreamHandler) GetStock(c *gin.Context) {
	raw := c.Query("pids")
	if strings.TrimSpace(raw) == "" {
		response.Fail(c, 400, errcode.CodeInvalidParam, "pids 不能为空（逗号分隔的上游商品 ID）")
		return
	}
	parts := strings.Split(raw, ",")
	pids := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		pid, err := strconv.ParseInt(p, 10, 64)
		if err != nil || pid <= 0 {
			response.Fail(c, 400, errcode.CodeInvalidParam, "pids 含非法值: "+p)
			return
		}
		pids = append(pids, pid)
	}
	list, err := h.svc.GetStock(c.Request.Context(), pids)
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, gin.H{"list": list})
}

// ImportProducts 处理 POST /api/admin/upstream/products/import：批量导入（幂等）。
func (h *UpstreamHandler) ImportProducts(c *gin.Context) {
	var req service.ImportRequest
	if !bindJSON(c, &req) {
		return
	}
	result, err := h.svc.ImportProducts(c.Request.Context(), &req)
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, result)
}

// GetUpstreamHosts 处理 GET /api/admin/upstream/hosts?page=1&limit=20。
func (h *UpstreamHandler) GetUpstreamHosts(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	list, err := h.svc.GetUpstreamHosts(c.Request.Context(), page, limit)
	if err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, list)
}
