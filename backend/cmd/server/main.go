// LiltIDC_OEM 后端入口。
//
// 启动流程：
//  1. 解析配置路径（环境变量 LILTIDC_CONFIG，默认 ./config.toml）并加载；
//  2. 已安装态：连接数据库（失败则清晰报错退出）+ 幂等自动迁移；
//     未安装态：跳过数据库，仅安装向导与公共接口可用（门禁中间件兜底）；
//  3. 组装 gin 路由并启动 HTTP 服务；
//  4. 捕获 SIGINT/SIGTERM 优雅退出（等待在途请求，最长 10 秒）。
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/config"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/migration"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/notify"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/app"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/router"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/upstream"
)

// Version 是后端版本号（/api/healthz 暴露）。
const Version = "0.1.0"

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))
	if err := run(); err != nil {
		slog.Error("启动失败", "err", err)
		os.Exit(1)
	}
}

func run() error {
	// ---- 1. 加载配置 ----
	configPath := config.ResolvePath()
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	a := app.New(cfg, configPath, Version)
	slog.Info("配置已加载", "path", configPath, "installed", cfg.Install.Installed)

	// ---- 2. 已安装态：连库 + 自动迁移 ----
	if cfg.Install.Installed {
		db, err := migration.Open(cfg.Database)
		if err != nil {
			slog.Error("数据库连接失败：请检查 config.toml [database] 配置与 MySQL 服务状态；"+
				"如需重新走安装向导，把 [install] installed 改为 false 后重启",
				"host", cfg.Database.Host, "port", cfg.Database.Port,
				"user", cfg.Database.User, "dbname", cfg.Database.DBName, "err", err)
			return fmt.Errorf("数据库连接失败: %w", err)
		}
		if err := migration.AutoMigrate(db); err != nil {
			slog.Error("数据库自动迁移失败", "err", err)
			return fmt.Errorf("数据库自动迁移失败: %w", err)
		}
		// 批 2：config.toml [upstream] 种子进 settings（仅当键不存在，不覆盖后台修改）
		if err := migration.SeedUpstreamFromConfig(db, cfg.Upstream); err != nil {
			slog.Warn("上游配置种子写入失败（不影响启动，可后台手工配置）", "err", err)
		}
		a.DB.Store(db)
		a.Notify.Store(notify.NewManager(db))
		a.Upstream.Store(upstream.NewSessionManager(db,
			time.Duration(cfg.Upstream.TimeoutSeconds)*time.Second))
		slog.Info("数据库已连接并完成迁移检查",
			"host", cfg.Database.Host, "dbname", cfg.Database.DBName)
	} else {
		slog.Warn("系统未安装：仅安装向导与公共接口可用，请通过前端 /install 页面完成安装")
	}

	// ---- 3. 启动 HTTP 服务 ----
	engine := router.New(a)
	// 批 3-β：后台任务（开通转发 worker + 未支付超时清扫）在路由组装（注册启动器）
	// 之后启动；已安装态才启动，未安装态由安装热切换路径触发（app.StartBackground 幂等）
	if cfg.Install.Installed {
		a.StartBackground()
	}
	srv := &http.Server{
		Addr:              cfg.Server.ListenAddr(),
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		slog.Info("HTTP 服务监听中", "addr", srv.Addr, "mode", cfg.Server.Mode, "installed", a.IsInstalled())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// ---- 4. 优雅退出 ----
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-errCh:
		return fmt.Errorf("HTTP 服务异常退出: %w", err)
	case sig := <-quit:
		slog.Info("收到退出信号，开始优雅关闭", "signal", sig.String())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("HTTP 优雅关闭失败（可能有在途请求超时）", "err", err)
	}
	if db := a.GetDB(); db != nil {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
	slog.Info("服务已退出")
	return nil
}
