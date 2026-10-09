package handler

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
)

func TestToSnake(t *testing.T) {
	cases := map[string]string{
		"SiteName":     "site_name",
		"PrimaryColor": "primary_color",
		"DBName":       "d_b_name", // 连续大写按字符逐个处理，符合实现的朴素行为
		"Host":         "host",
		"A":            "a",
		"":             "",
	}
	for in, want := range cases {
		if got := toSnake(in); got != want {
			t.Errorf("toSnake(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBindingErrorMessageJSONSyntax(t *testing.T) {
	var syntaxErr *json.SyntaxError
	// 构造一个真实的 JSON 语法错误
	err := json.Unmarshal([]byte("{bad json"), &struct{}{})
	if err == nil {
		t.Fatal("预期构造出 JSON 语法错误")
	}
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("预期为 *json.SyntaxError，得到 %T", err)
	}
	msg := bindingErrorMessage(err)
	if msg != "请求体不是合法 JSON" {
		t.Errorf("JSON 语法错误消息不符: %q", msg)
	}
}

func TestBindingErrorMessageEOF(t *testing.T) {
	msg := bindingErrorMessage(errors.New("unexpected EOF"))
	if msg != "请求体不能为空" {
		t.Errorf("EOF 错误消息不符: %q", msg)
	}
}

func TestBindingErrorMessageValidation(t *testing.T) {
	// 用真实 validator 触发一个 required 校验错误
	type payload struct {
		Host string `validate:"required"`
	}
	v := validator.New()
	err := v.Struct(&payload{Host: ""})
	if err == nil {
		t.Fatal("预期触发校验错误")
	}
	var ve validator.ValidationErrors
	if !errors.As(err, &ve) {
		t.Fatalf("预期为 ValidationErrors，得到 %T", err)
	}
	msg := bindingErrorMessage(err)
	if !strings.Contains(msg, "为必填项") {
		t.Errorf("required 校验消息不符: %q", msg)
	}
}

func TestBindingErrorMessageFallback(t *testing.T) {
	msg := bindingErrorMessage(errors.New("some other error"))
	if !strings.Contains(msg, "参数解析失败") {
		t.Errorf("兜底消息不符: %q", msg)
	}
}
