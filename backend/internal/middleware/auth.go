package middleware

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/app"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/errcode"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/jwtutil"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/response"
)

// gin.Context 中存放认证信息的键。
const (
	CtxKeyAdminID  = "ctx_admin_id"
	CtxKeyUsername = "ctx_username"
)

// AuthRequired 是 JWT 认证中间件：解析 Authorization: Bearer <token>，
// 校验签名/有效期/签发者，通过后把 admin_id 与 username 写入 gin 上下文。
//
// 批 1 不做 token 吊销（登出=前端清除）；管理员状态（禁用）变更在下次登录时生效。
func AuthRequired(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		tokenStr, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || strings.TrimSpace(tokenStr) == "" {
			response.AbortFail(c, 401, errcode.CodeUnauthorized, "缺少 Authorization: Bearer <token> 请求头")
			return
		}
		claims, err := jwtutil.Parse(a.Cfg().JWT.Secret, tokenStr)
		if err != nil {
			msg := "登录无效，请重新登录"
			if errors.Is(err, jwtutil.ErrExpired) {
				msg = "登录已过期，请重新登录"
			}
			response.AbortFail(c, 401, errcode.CodeUnauthorized, msg)
			return
		}
		c.Set(CtxKeyAdminID, claims.AdminID)
		c.Set(CtxKeyUsername, claims.Username)
		c.Next()
	}
}

// CurrentAdminID 从 gin 上下文取当前登录管理员 ID（须在 AuthRequired 之后调用）。
func CurrentAdminID(c *gin.Context) uint {
	v, exists := c.Get(CtxKeyAdminID)
	if !exists {
		return 0
	}
	id, _ := v.(uint)
	return id
}

// CurrentUsername 从 gin 上下文取当前登录管理员用户名。
func CurrentUsername(c *gin.Context) string {
	v, exists := c.Get(CtxKeyUsername)
	if !exists {
		return ""
	}
	s, _ := v.(string)
	return s
}
