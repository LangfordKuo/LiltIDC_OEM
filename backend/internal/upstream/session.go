package upstream

import (
	"context"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/model"
)

// SessionManager 管理上游 Client 的生命周期（ADR-019/020）：
//   - 凭据运行时真源是数据库 settings（后台改配置立即生效，无需重启）；
//   - 按凭据指纹缓存复用 Client —— jwt 缓存跨 HTTP 请求存活，避免每次调用都登录；
//   - 指纹变化（后台改了地址/账号/密钥）时重建 Client（旧 jwt 作废）。
type SessionManager struct {
	db      *gorm.DB
	timeout time.Duration

	mu          sync.Mutex
	cached      *Client
	fingerprint string
}

// NewSessionManager 构造会话管理器；timeout 来自 config [upstream] timeout_seconds。
func NewSessionManager(db *gorm.DB, timeout time.Duration) *SessionManager {
	return &SessionManager{db: db, timeout: timeout}
}

// Credentials 从 settings 读取上游凭据；关键项缺失返回 ErrNotConfigured。
func (m *SessionManager) Credentials(ctx context.Context) (*Credentials, error) {
	settings, err := model.GetSettingsMap(m.db.WithContext(ctx),
		model.SettingUpstreamHostname, model.SettingUpstreamUsername, model.SettingUpstreamAPIKey)
	if err != nil {
		return nil, err
	}
	cred := &Credentials{
		Hostname: settings[model.SettingUpstreamHostname],
		Username: settings[model.SettingUpstreamUsername],
		APIKey:   settings[model.SettingUpstreamAPIKey],
		Timeout:  m.timeout,
	}
	if cred.Hostname == "" || cred.Username == "" || cred.APIKey == "" {
		return nil, ErrNotConfigured
	}
	return cred, nil
}

// Client 返回可复用的上游客户端（凭据变化时自动重建）。
// 未配置时返回 ErrNotConfigured。
func (m *SessionManager) Client(ctx context.Context) (*Client, error) {
	cred, err := m.Credentials(ctx)
	if err != nil {
		return nil, err
	}
	fp := cred.Hostname + "\x00" + cred.Username + "\x00" + cred.APIKey
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cached != nil && m.fingerprint == fp {
		return m.cached, nil
	}
	m.cached = NewClient(*cred)
	m.fingerprint = fp
	return m.cached, nil
}

// Invalidate 使缓存的 Client 失效（后台保存配置后调用，强制下次重建）。
func (m *SessionManager) Invalidate() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cached = nil
	m.fingerprint = ""
}
