package notify

import (
	"regexp"
	"strings"
)

// placeholderRe 匹配 {key} 形式的占位符（key 限字母数字下划线，见契约文档 8.3）。
var placeholderRe = regexp.MustCompile(`\{([A-Za-z0-9_]+)\}`)

// Render 渲染模板：单遍扫描替换 {key} 占位符。
// 未知占位符原样保留（便于排查模板配置错误）；替换值中的 {…} 不会被二次替换。
func Render(tpl string, vars map[string]string) string {
	if !strings.ContainsRune(tpl, '{') {
		return tpl
	}
	return placeholderRe.ReplaceAllStringFunc(tpl, func(m string) string {
		key := m[1 : len(m)-1]
		if v, ok := vars[key]; ok {
			return v
		}
		return m
	})
}
