// Package response 提供统一响应体输出助手。
// 成功：{"status":200,"msg":"ok","data":{...}}
// 失败：{"status":<HTTP状态码>,"code":"<错误码>","msg":"<可读信息>"}
// 见 docs/api-contract.md 第 0.2 节。
package response

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/errcode"
)

// Body 是成功响应体。
type Body struct {
	Status int    `json:"status"`
	Msg    string `json:"msg"`
	Data   any    `json:"data"`
}

// ErrorBody 是错误响应体。
type ErrorBody struct {
	Status int    `json:"status"`
	Code   string `json:"code"`
	Msg    string `json:"msg"`
}

// OK 输出成功响应。data 为 nil 时输出空对象。
func OK(c *gin.Context, data any) {
	if data == nil {
		data = gin.H{}
	}
	c.JSON(http.StatusOK, Body{Status: 200, Msg: "ok", Data: data})
}

// Fail 输出错误响应（HTTP 状态码与 body.status 一致）。
func Fail(c *gin.Context, httpStatus int, code, msg string) {
	c.JSON(httpStatus, ErrorBody{Status: httpStatus, Code: code, Msg: msg})
}

// FailBiz 输出 *errcode.BizError：底层错误仅记日志，不进响应体。
func FailBiz(c *gin.Context, biz *errcode.BizError) {
	if biz.Err != nil {
		slog.Error("请求处理失败",
			"path", c.Request.URL.Path,
			"code", biz.Code,
			"status", biz.HTTPStatus,
			"err", biz.Err)
	}
	Fail(c, biz.HTTPStatus, biz.Code, biz.Msg)
}

// FailErr 智能输出任意 error：是 *BizError 则按其输出，否则统一 500 internal_error。
// 非业务错误的原始信息只记日志，不泄露给客户端。
func FailErr(c *gin.Context, err error) {
	var biz *errcode.BizError
	if errors.As(err, &biz) {
		FailBiz(c, biz)
		return
	}
	slog.Error("未分类错误", "path", c.Request.URL.Path, "err", err)
	Fail(c, http.StatusInternalServerError, errcode.CodeInternalError, "服务器内部错误")
}

// AbortFail 输出错误响应并中止后续 handler（中间件用）。
func AbortFail(c *gin.Context, httpStatus int, code, msg string) {
	Fail(c, httpStatus, code, msg)
	c.Abort()
}
