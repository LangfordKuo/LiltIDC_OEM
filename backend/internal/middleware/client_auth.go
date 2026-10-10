package middleware

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/model"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/app"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/errcode"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/jwtutil"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/response"
)

// 会员 JWT 上下文键（与管理员键隔离）。
const (
	CtxKeyClientID       = "ctx_client_id"
	CtxKeyClientUsername = "ctx_client_username"
)

// ClientAuthRequired 是会员 JWT 认证中间件（批 3-α，契约 12.3）：
//   - 仅接受 issuer=liltidc-oem-client 的 token（管理员 token 必然 401，双签发方隔离）；
//   - 每请求查一次 clients 复核状态：禁用 → 403 account_disabled、
//     已删除/不存在 → 401（会员端请求频率低，状态即时生效优先于查询开销，见 ADR-025）。
func ClientAuthRequired(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		tokenStr, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || strings.TrimSpace(tokenStr) == "" {
			response.AbortFail(c, 401, errcode.CodeUnauthorized, "缺少 Authorization: Bearer <token> 请求头")
			return
		}
		claims, err := jwtutil.ParseClient(a.Cfg().JWT.Secret, tokenStr)
		if err != nil {
			msg := "登录无效，请重新登录"
			if errors.Is(err, jwtutil.ErrExpired) {
				msg = "登录已过期，请重新登录"
			}
			response.AbortFail(c, 401, errcode.CodeUnauthorized, msg)
			return
		}
		db := a.GetDB()
		if db == nil {
			response.AbortFail(c, 500, errcode.CodeInternalError, "数据库未就绪")
			return
		}
		var client model.Client
		if err := db.WithContext(c.Request.Context()).First(&client, claims.ClientID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				response.AbortFail(c, 401, errcode.CodeUnauthorized, "账号不存在，请重新登录")
				return
			}
			response.AbortFail(c, 500, errcode.CodeInternalError, "查询会员失败")
			return
		}
		if client.IsDeleted() {
			response.AbortFail(c, 401, errcode.CodeUnauthorized, "账号不存在，请重新登录")
			return
		}
		if client.Status == model.ClientStatusDisabled {
			response.AbortFail(c, 403, errcode.CodeAccountDisabled, "账号已被禁用，请联系客服")
			return
		}
		c.Set(CtxKeyClientID, client.ID)
		c.Set(CtxKeyClientUsername, client.Username)
		c.Next()
	}
}

// CurrentClientID 从上下文取当前会员 ID（须在 ClientAuthRequired 之后调用）。
func CurrentClientID(c *gin.Context) uint {
	v, exists := c.Get(CtxKeyClientID)
	if !exists {
		return 0
	}
	id, _ := v.(uint)
	return id
}
