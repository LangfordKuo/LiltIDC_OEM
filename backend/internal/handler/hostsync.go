package handler

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/errcode"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/response"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/service"
)

// maxHostSyncBodyBytes 是推送请求体大小上限（1MB，防异常大报文）。
const maxHostSyncBodyBytes = 1 << 20

// HostSyncHandler 处理上游 → 下游主机推送（契约文档 9.6）。
// 免 JWT（上游服务器调用），以魔方标准 MD5 签名验证代替登录态。
type HostSyncHandler struct {
	svc *service.HostSyncService
}

// NewHostSync 构造主机同步 handler。
func NewHostSync(svc *service.HostSyncService) *HostSyncHandler {
	return &HostSyncHandler{svc: svc}
}

// Sync 处理 POST /api/upstream/host/sync。
// 成功：HTTP 200 + 统一响应体（上游只关心 200）；验签失败：403 signature_invalid。
func (h *HostSyncHandler) Sync(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxHostSyncBodyBytes+1))
	if err != nil {
		response.Fail(c, http.StatusBadRequest, errcode.CodeInvalidParam, "读取请求体失败")
		return
	}
	if len(body) > maxHostSyncBodyBytes {
		response.Fail(c, http.StatusRequestEntityTooLarge, errcode.CodeInvalidParam, "请求体超过 1MB 上限")
		return
	}
	if len(body) == 0 {
		response.Fail(c, http.StatusBadRequest, errcode.CodeInvalidParam, "请求体为空")
		return
	}
	if err := h.svc.HandleSync(c.Request.Context(), body); err != nil {
		response.FailErr(c, err)
		return
	}
	response.OK(c, gin.H{"received": true})
}
