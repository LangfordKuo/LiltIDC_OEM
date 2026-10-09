package service

import (
	"context"
	"log/slog"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/model"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/app"
)

// BrandService 提供品牌信息（契约文档 2.1）。
// 未安装态读 config.toml [brand]；已安装态优先读 settings 表，缺失键回退配置。
type BrandService struct {
	app *app.App
}

// NewBrand 构造品牌服务。
func NewBrand(a *app.App) *BrandService {
	return &BrandService{app: a}
}

// BrandInfo 是品牌信息响应体。
type BrandInfo struct {
	SiteName     string `json:"site_name"`
	Logo         string `json:"logo"`
	PrimaryColor string `json:"primary_color"`
}

// Get 返回当前品牌信息。本接口在未安装态也必须可用（安装页需要样式），
// 因此任何数据库错误都只记日志并回退到配置值，绝不报错。
func (s *BrandService) Get(ctx context.Context) *BrandInfo {
	cfg := s.app.Cfg()
	info := &BrandInfo{
		SiteName:     cfg.Brand.SiteName,
		Logo:         cfg.Brand.Logo,
		PrimaryColor: cfg.Brand.PrimaryColor,
	}
	db := s.app.GetDB()
	if db == nil || !s.app.IsInstalled() {
		return info
	}
	settings, err := model.GetSettingsMap(db.WithContext(ctx),
		model.SettingSiteName, model.SettingLogo, model.SettingPrimaryColor)
	if err != nil {
		slog.Warn("读取品牌设置失败，回退配置文件", "err", err)
		return info
	}
	if v, ok := settings[model.SettingSiteName]; ok && v != "" {
		info.SiteName = v
	}
	if v, ok := settings[model.SettingLogo]; ok {
		info.Logo = v
	}
	if v, ok := settings[model.SettingPrimaryColor]; ok && v != "" {
		info.PrimaryColor = v
	}
	return info
}
