// Package errcode 集中定义业务错误码字符串常量与业务错误类型。
// 错误码与 HTTP 状态的对应关系见 docs/api-contract.md 第 0.4 节。
package errcode

import "fmt"

// 错误码常量（与 docs/api-contract.md 保持一致，新增错误码必须先更新契约文档）。
const (
	CodeOK                    = "ok"
	CodeNotInstalled          = "not_installed"
	CodeAlreadyInstalled      = "already_installed"
	CodeInvalidParam          = "invalid_param"
	CodeDBConnectFailed       = "db_connect_failed"
	CodeDBVersionUnsupported  = "db_version_unsupported"
	CodeMigrationFailed       = "migration_failed"
	CodeConfigWriteFailed     = "config_write_failed"
	CodeUnauthorized          = "unauthorized"
	CodeInvalidCredentials    = "invalid_credentials"
	CodeAccountDisabled       = "account_disabled"
	CodeNotFound              = "not_found"
	CodeTemplateNotFound      = "template_not_found"
	CodeTemplateDisabled      = "template_disabled"
	CodeNotifyNotConfigured   = "notify_not_configured"
	CodeSMSSendFailed         = "sms_send_failed"
	CodeSMSBalanceQueryFailed = "sms_balance_query_failed"
	CodeEmailSendFailed       = "email_send_failed"
	CodeInternalError         = "internal_error"
)

// BizError 是携带 HTTP 状态码与字符串错误码的业务错误。
// handler 层通过 response.FailBiz 统一输出。
type BizError struct {
	HTTPStatus int    // HTTP 状态码
	Code       string // 字符串错误码
	Msg        string // 人类可读信息（可直接展示）
	Err        error  // 可选：底层原始错误（仅记日志，不出响应体）
}

// Error 实现 error 接口。
func (e *BizError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s(%d %s): %v", e.Msg, e.HTTPStatus, e.Code, e.Err)
	}
	return fmt.Sprintf("%s(%d %s)", e.Msg, e.HTTPStatus, e.Code)
}

// Unwrap 暴露底层错误以支持 errors.Is/As。
func (e *BizError) Unwrap() error { return e.Err }

// WithErr 附加底层错误（用于服务端日志排查），返回自身便于链式调用。
func (e *BizError) WithErr(err error) *BizError {
	e.Err = err
	return e
}

// New 构造业务错误。
func New(httpStatus int, code, msg string) *BizError {
	return &BizError{HTTPStatus: httpStatus, Code: code, Msg: msg}
}

// 以下为常用错误构造助手，msg 支持 fmt 格式化参数。

// InvalidParam 400 参数错误。
func InvalidParam(format string, args ...any) *BizError {
	return New(400, CodeInvalidParam, fmt.Sprintf(format, args...))
}

// NotInstalled 409 未安装。
func NotInstalled() *BizError {
	return New(409, CodeNotInstalled, "系统尚未安装")
}

// AlreadyInstalled 409 已安装（防重装）。
func AlreadyInstalled() *BizError {
	return New(409, CodeAlreadyInstalled, "系统已安装，禁止重复执行安装")
}

// DBConnectFailed 数据库连接失败（msg 携带底层可读原因）。
func DBConnectFailed(reason string) *BizError {
	return New(400, CodeDBConnectFailed, fmt.Sprintf("数据库连接失败: %s", reason))
}

// DBVersionUnsupported MySQL 版本过低。
func DBVersionUnsupported(version string) *BizError {
	return New(400, CodeDBVersionUnsupported, fmt.Sprintf("MySQL 版本 %s 不受支持，要求 >= 5.7", version))
}

// Unauthorized 401 未认证。
func Unauthorized(msg string) *BizError {
	if msg == "" {
		msg = "未登录或登录已过期"
	}
	return New(401, CodeUnauthorized, msg)
}

// InvalidCredentials 401 用户名或密码错误（不区分，防用户枚举）。
func InvalidCredentials() *BizError {
	return New(401, CodeInvalidCredentials, "用户名或密码错误")
}

// AccountDisabled 403 账号被禁用。
func AccountDisabled() *BizError {
	return New(403, CodeAccountDisabled, "账号已被禁用，请联系上级管理员")
}

// NotFound 404 资源不存在。
func NotFound(msg string) *BizError {
	if msg == "" {
		msg = "资源不存在"
	}
	return New(404, CodeNotFound, msg)
}

// Internal 500 内部错误（对外隐藏细节，细节走日志）。
func Internal(format string, args ...any) *BizError {
	return New(500, CodeInternalError, fmt.Sprintf(format, args...))
}
