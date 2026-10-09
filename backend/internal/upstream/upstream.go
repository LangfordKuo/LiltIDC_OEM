// Package upstream 预留上游魔方财务对接（批 2 实现调用逻辑）。
//
// LiltIDC_OEM 不对接 PVE 等虚拟化层：商品开通、暂停、恢复、重装等运维动作
// 均转发上游魔方财务执行。批 1 只声明客户端结构与配置绑定（[upstream] 分节），
// 不包含任何 HTTP 调用；接口签名与回调约定将在批 2 的契约章节中定义。
package upstream

import (
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/config"
)

// Client 是上游魔方财务客户端占位（批 2 实现具体调用）。
type Client struct {
	Hostname string // 上游站点地址，如 https://upstream.example.com
	Username string // 上游 API 用户名/ID
	APIKey   string // 上游 API 密钥
}

// NewClient 从配置构造上游客户端占位。
func NewClient(cfg config.UpstreamConfig) *Client {
	return &Client{
		Hostname: cfg.Hostname,
		Username: cfg.Username,
		APIKey:   cfg.APIKey,
	}
}

// Configured 判断上游配置是否完整（批 2 用于功能开关）。
func (c *Client) Configured() bool {
	return c.Hostname != "" && c.APIKey != ""
}
