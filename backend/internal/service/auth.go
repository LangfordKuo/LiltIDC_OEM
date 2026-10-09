package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/model"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/app"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/errcode"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/jwtutil"
)

// AuthService 处理管理员认证业务。
type AuthService struct {
	app *app.App
}

// NewAuth 构造认证服务。
func NewAuth(a *app.App) *AuthService {
	return &AuthService{app: a}
}

// LoginRequest 是登录请求（契约文档 4.1）。
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// AdminInfo 是对外暴露的管理员信息（不含密码哈希）。
type AdminInfo struct {
	ID          uint       `json:"id"`
	Username    string     `json:"username"`
	RealName    string     `json:"real_name"`
	Email       string     `json:"email"`
	LastLoginAt *time.Time `json:"last_login_at"`
}

// LoginResult 是登录成功结果。
type LoginResult struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	Admin     AdminInfo `json:"admin"`
}

// Login 校验用户名密码并签发 JWT。
// 安全约定：用户名不存在与密码错误返回同一错误（防用户枚举）。
func (s *AuthService) Login(ctx context.Context, req *LoginRequest) (*LoginResult, error) {
	if req.Username == "" || req.Password == "" {
		return nil, errcode.InvalidParam("username 与 password 均不能为空")
	}
	db := s.app.GetDB()
	if db == nil {
		return nil, errcode.Internal("数据库未就绪")
	}

	var admin model.Admin
	err := db.WithContext(ctx).Where("username = ?", req.Username).First(&admin).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.InvalidCredentials()
	}
	if err != nil {
		return nil, errcode.Internal("查询管理员失败").WithErr(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.Password)); err != nil {
		return nil, errcode.InvalidCredentials()
	}
	if !admin.IsEnabled() {
		return nil, errcode.AccountDisabled()
	}

	// 更新最后登录时间（失败不阻断登录）
	now := time.Now().Local()
	if err := db.WithContext(ctx).Model(&admin).Update("last_login_at", &now).Error; err != nil {
		slog.Warn("更新最后登录时间失败", "admin_id", admin.ID, "err", err)
	}
	admin.LastLoginAt = &now

	expire := time.Duration(s.app.Cfg().JWT.ExpireHours) * time.Hour
	token, expiresAt, err := jwtutil.Sign(s.app.Cfg().JWT.Secret, expire, admin.ID, admin.Username)
	if err != nil {
		return nil, errcode.Internal("签发 token 失败").WithErr(err)
	}
	return &LoginResult{
		Token:     token,
		ExpiresAt: expiresAt,
		Admin:     toAdminInfo(&admin),
	}, nil
}

// Me 按 ID 返回当前登录管理员信息。
func (s *AuthService) Me(ctx context.Context, adminID uint) (*AdminInfo, error) {
	db := s.app.GetDB()
	if db == nil {
		return nil, errcode.Internal("数据库未就绪")
	}
	var admin model.Admin
	err := db.WithContext(ctx).First(&admin, adminID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.Unauthorized("账号不存在，请重新登录")
	}
	if err != nil {
		return nil, errcode.Internal("查询管理员失败").WithErr(err)
	}
	info := toAdminInfo(&admin)
	return &info, nil
}

func toAdminInfo(a *model.Admin) AdminInfo {
	return AdminInfo{
		ID:          a.ID,
		Username:    a.Username,
		RealName:    a.RealName,
		Email:       a.Email,
		LastLoginAt: a.LastLoginAt,
	}
}
