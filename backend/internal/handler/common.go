// Package handler 是 HTTP 适配层：参数绑定 → 调 service → 统一响应输出。
// 业务逻辑一律放 service 层，handler 保持薄。
package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/errcode"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/response"
)

// bindJSON 绑定 JSON 请求体；失败时输出可读的 invalid_param 并返回 false。
func bindJSON(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		response.Fail(c, 400, errcode.CodeInvalidParam, bindingErrorMessage(err))
		return false
	}
	return true
}

// bindingErrorMessage 把绑定/校验错误转换为中文可读信息。
func bindingErrorMessage(err error) string {
	// gin validator 字段校验错误
	var ve validator.ValidationErrors
	if errors.As(err, &ve) && len(ve) > 0 {
		fe := ve[0]
		field := toSnake(fe.Field())
		switch fe.Tag() {
		case "required":
			return fmt.Sprintf("参数校验失败: %s 为必填项", field)
		case "min":
			return fmt.Sprintf("参数校验失败: %s 不能小于 %s", field, fe.Param())
		case "max":
			return fmt.Sprintf("参数校验失败: %s 不能大于 %s", field, fe.Param())
		case "oneof":
			return fmt.Sprintf("参数校验失败: %s 只能是 %s 之一", field, fe.Param())
		default:
			return fmt.Sprintf("参数校验失败: %s 不满足约束 %s=%s", field, fe.Tag(), fe.Param())
		}
	}
	// JSON 语法错误
	var se *json.SyntaxError
	if errors.As(err, &se) {
		return "请求体不是合法 JSON"
	}
	if strings.Contains(err.Error(), "EOF") {
		return "请求体不能为空"
	}
	return "参数解析失败: " + err.Error()
}

// toSnake 把 Go 字段名（如 SiteName）转为 snake_case（site_name），贴近 JSON 字段。
func toSnake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r + ('a' - 'A'))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
