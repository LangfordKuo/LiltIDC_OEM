// Package app 定义后端运行时容器（依赖根）。
//
// App 贯穿进程生命周期：
//   - Cfg 用 atomic.Pointer 持有，安装完成回写配置后热替换（避免读写竞争）；
//   - Installed 是安装状态运行时真源（持久化真源为 config.toml，见 ADR-004）；
//   - DB / Notify 在安装完成前为 nil，安装成功后原子热装载，无需重启进程（ADR-010）。
package app

import (
	"sync"
	"sync/atomic"

	"gorm.io/gorm"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/config"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/notify"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/upstream"
)

// App 是运行时容器。
type App struct {
	// Version 是后端版本号（/api/healthz 暴露）。
	Version string
	// ConfigPath 是当前生效的配置文件路径（安装器回写用）。
	ConfigPath string

	cfg       atomic.Pointer[config.Config]
	Installed atomic.Bool
	DB        atomic.Pointer[gorm.DB]
	Notify    atomic.Pointer[notify.Manager]
	Upstream  atomic.Pointer[upstream.SessionManager] // 批 2：上游会话管理（settings 真源 + jwt 缓存复用）

	installMu sync.Mutex // 安装互斥：防并发 complete 造成双写

	// 批 3-β：后台任务（开通 worker/超时清扫）启动器。路由组装时注册，
	// 启动（已安装态）与安装热切换后各调一次 StartBackground，sync.Once 保证只启动一次。
	bgOnce  sync.Once
	bgStart atomic.Pointer[func()]
}

// New 构造运行时容器，安装状态初值取自配置。
func New(cfg *config.Config, configPath, version string) *App {
	a := &App{Version: version, ConfigPath: configPath}
	a.cfg.Store(cfg)
	a.Installed.Store(cfg.Install.Installed)
	return a
}

// Cfg 返回当前配置快照（只读；安装器回写后用 SetCfg 热替换）。
func (a *App) Cfg() *config.Config {
	return a.cfg.Load()
}

// SetCfg 热替换配置（安装完成时调用）。
func (a *App) SetCfg(cfg *config.Config) {
	a.cfg.Store(cfg)
}

// IsInstalled 返回运行时安装状态。
func (a *App) IsInstalled() bool {
	return a.Installed.Load()
}

// GetDB 返回 GORM 句柄；未安装时为 nil（调用方须判空）。
func (a *App) GetDB() *gorm.DB {
	return a.DB.Load()
}

// GetNotify 返回通知管理器；未安装时为 nil（调用方须判空）。
func (a *App) GetNotify() *notify.Manager {
	return a.Notify.Load()
}

// GetUpstream 返回上游会话管理器；未安装时为 nil（调用方须判空）。
func (a *App) GetUpstream() *upstream.SessionManager {
	return a.Upstream.Load()
}

// InstallLock 获取安装互斥锁，返回解锁函数（防并发安装）。
func (a *App) InstallLock() func() {
	a.installMu.Lock()
	return a.installMu.Unlock
}

// SetBackgroundStarter 注册后台任务启动器（路由组装期调用；只注册不启动）。
func (a *App) SetBackgroundStarter(fn func()) {
	a.bgStart.Store(&fn)
}

// StartBackground 启动后台任务（开通 worker + 超时清扫）。
// sync.Once 保证进程内只启动一次：启动时（已安装态）与安装热切换后各调一次是安全的。
func (a *App) StartBackground() {
	p := a.bgStart.Load()
	if p == nil {
		return
	}
	a.bgOnce.Do(*p)
}
