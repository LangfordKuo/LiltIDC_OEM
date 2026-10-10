package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/model"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/app"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/errcode"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/jwtutil"
)

// ClientService 处理会员域业务（契约文档第 12 章）。
type ClientService struct {
	app    *app.App
	verify *VerifyService
}

// NewClient 构造会员服务。
func NewClient(a *app.App, v *VerifyService) *ClientService {
	return &ClientService{app: a, verify: v}
}

// 会员域校验正则（契约 12.2）。
var (
	clientUsernameRe = regexp.MustCompile(`^[A-Za-z0-9_]{3,32}$`)
	clientPhoneRe    = regexp.MustCompile(`^1[3-9]\d{9}$`)
	clientCodeRe     = regexp.MustCompile(`^\d{6}$`)
	hasLetterRe      = regexp.MustCompile(`[A-Za-z]`)
	hasDigitRe       = regexp.MustCompile(`[0-9]`)
	// send-code 的 sms 目标：宽松单号码（数字/+/-，5-20 位；注册落库时才严格校验手机号）
	smsTargetRe = regexp.MustCompile(`^[0-9+\-]{5,20}$`)
)

// validateClientPassword 校验密码强度：8-72 位且同时含字母与数字。
func validateClientPassword(pw string) error {
	if len(pw) < 8 || len(pw) > 72 {
		return errcode.InvalidParam("密码长度须为 8-72 位")
	}
	if !hasLetterRe.MatchString(pw) || !hasDigitRe.MatchString(pw) {
		return errcode.InvalidParam("密码须同时包含字母和数字")
	}
	return nil
}

// ---- 12.1 发送注册验证码 ----

// SendCodeRequest 是发送验证码请求。
type SendCodeRequest struct {
	Purpose       string `json:"purpose"`
	Channel       string `json:"channel"`
	To            string `json:"to"`
	Username      string `json:"username"`
	Email         string `json:"email"`
	CaptchaID     string `json:"captcha_id"`
	CaptchaAnswer string `json:"captcha_answer"`
}

// SendCode 校验参数（含 sms 场景用户名预占查重）后签发验证码。
func (s *ClientService) SendCode(ctx context.Context, req *SendCodeRequest, ip, lang string) error {
	if req.Purpose != "register" {
		return errcode.InvalidParam("purpose 当前仅支持 register")
	}
	to := strings.TrimSpace(req.To)
	switch req.Channel {
	case model.ChannelSMS:
		if !smsTargetRe.MatchString(to) || strings.Contains(to, ",") {
			return errcode.InvalidParam("手机号格式不正确")
		}
		// sms 场景注册前看不到表单错误 → 发送前预占查重用户名
		username := strings.TrimSpace(req.Username)
		if !clientUsernameRe.MatchString(username) {
			return errcode.InvalidParam("用户名须为 3-32 位字母、数字或下划线")
		}
		exists, err := s.usernameExists(ctx, username)
		if err != nil {
			return err
		}
		if exists {
			return errcode.New(400, errcode.CodeUsernameExists, fmt.Sprintf("用户名 %s 已被占用", username))
		}
	case model.ChannelEmail:
		if !emailRe.MatchString(to) {
			return errcode.InvalidParam("邮箱格式不正确")
		}
	default:
		return errcode.InvalidParam("channel 只能是 sms 或 email")
	}
	return s.verify.IssueCode(ctx, req.Purpose, req.Channel, to, lang, ip, req.CaptchaID, req.CaptchaAnswer)
}

// usernameExists 检查用户名是否被占用（软删除已释放后缀名，= 查询天然排除）。
func (s *ClientService) usernameExists(ctx context.Context, username string) (bool, error) {
	db := s.app.GetDB()
	var count int64
	if err := db.WithContext(ctx).Model(&model.Client{}).Where("username = ?", username).Count(&count).Error; err != nil {
		return false, errcode.Internal("查询用户名失败").WithErr(err)
	}
	return count > 0, nil
}

// ---- 12.2 注册 ----

// RegisterRequest 是会员注册请求。
type RegisterRequest struct {
	Username        string `json:"username"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirm_password"`
	Email           string `json:"email"`
	Phone           string `json:"phone"`
	VerifyChannel   string `json:"verify_channel"`
	VerifyCode      string `json:"verify_code"`
}

// RegisterResult 是注册成功结果（不自动登录）。
type RegisterResult struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
}

// Register 注册会员：验证码校验 → 唯一性终检 → bcrypt 落库 → 消耗验证码。
// 并发同名注册由数据库唯一索引兜底（冲突转 *_exists）。
func (s *ClientService) Register(ctx context.Context, req *RegisterRequest) (*RegisterResult, error) {
	db := s.app.GetDB()
	username := strings.TrimSpace(req.Username)
	email := strings.TrimSpace(req.Email)
	phone := strings.TrimSpace(req.Phone)

	// ---- 1. 字段校验 ----
	if !clientUsernameRe.MatchString(username) {
		return nil, errcode.InvalidParam("用户名须为 3-32 位字母、数字或下划线")
	}
	if err := validateClientPassword(req.Password); err != nil {
		return nil, err
	}
	if req.Password != req.ConfirmPassword {
		return nil, errcode.InvalidParam("两次输入的密码不一致")
	}
	if email != "" && !emailRe.MatchString(email) {
		return nil, errcode.InvalidParam("邮箱格式不正确")
	}
	if phone != "" && !clientPhoneRe.MatchString(phone) {
		return nil, errcode.InvalidParam("手机号格式不正确")
	}
	var verifyTo string
	switch req.VerifyChannel {
	case model.ChannelSMS:
		if phone == "" {
			return nil, errcode.InvalidParam("短信验证注册必须填写手机号")
		}
		verifyTo = phone
	case model.ChannelEmail:
		if email == "" {
			return nil, errcode.InvalidParam("邮件验证注册必须填写邮箱")
		}
		verifyTo = email
	default:
		return nil, errcode.InvalidParam("verify_channel 只能是 sms 或 email")
	}
	if !clientCodeRe.MatchString(strings.TrimSpace(req.VerifyCode)) {
		return nil, errcode.InvalidParam("验证码为 6 位数字")
	}

	// ---- 2. 验证码校验（失败不消耗；成功注册后 Consume） ----
	if err := s.verify.VerifyCode(ctx, "register", req.VerifyChannel, verifyTo, strings.TrimSpace(req.VerifyCode)); err != nil {
		return nil, err
	}

	// ---- 3. 唯一性终检（友好报错；并发由唯一索引兜底） ----
	if exists, err := s.usernameExists(ctx, username); err != nil {
		return nil, err
	} else if exists {
		return nil, errcode.New(400, errcode.CodeUsernameExists, fmt.Sprintf("用户名 %s 已被占用", username))
	}
	if email != "" {
		if exists, err := s.fieldExists(ctx, "email", email, 0); err != nil {
			return nil, err
		} else if exists {
			return nil, errcode.New(400, errcode.CodeEmailExists, "该邮箱已被注册")
		}
	}
	if phone != "" {
		if exists, err := s.fieldExists(ctx, "phone", phone, 0); err != nil {
			return nil, err
		} else if exists {
			return nil, errcode.New(400, errcode.CodePhoneExists, "该手机号已被注册")
		}
	}

	// ---- 4. 落库 ----
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errcode.Internal("密码哈希失败").WithErr(err)
	}
	client := model.Client{
		Username:     username,
		PasswordHash: string(hash),
		Email:        nilIfEmpty(email),
		Phone:        nilIfEmpty(phone),
		Status:       model.ClientStatusActive,
	}
	if err := db.WithContext(ctx).Create(&client).Error; err != nil {
		if dupErr := s.mapDuplicateErr(ctx, err, username, email, phone); dupErr != nil {
			return nil, dupErr
		}
		return nil, errcode.Internal("创建会员失败").WithErr(err)
	}

	// ---- 5. 消耗验证码（防重放；失败不影响注册结果） ----
	s.verify.ConsumeCode(ctx, "register", req.VerifyChannel, verifyTo)
	slog.Info("会员注册成功", "client_id", client.ID, "username", username, "verify_channel", req.VerifyChannel)
	return &RegisterResult{ID: client.ID, Username: client.Username}, nil
}

// fieldExists 检查 email/phone 是否被占用（excludeID>0 时排除自己，资料修改用）。
func (s *ClientService) fieldExists(ctx context.Context, field, value string, excludeID uint) (bool, error) {
	db := s.app.GetDB()
	q := db.WithContext(ctx).Model(&model.Client{}).Where(field+" = ?", value)
	if excludeID > 0 {
		q = q.Where("id != ?", excludeID)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return false, errcode.Internal("查询占用状态失败").WithErr(err)
	}
	return count > 0, nil
}

// mapDuplicateErr 把唯一索引冲突翻译为具体的 *_exists 错误（并发注册兜底）。
func (s *ClientService) mapDuplicateErr(ctx context.Context, err error, username, email, phone string) error {
	if !isDuplicateKeyErr(err) {
		return nil
	}
	// 逐字段回查定位冲突源
	if exists, e := s.usernameExists(ctx, username); e == nil && exists {
		return errcode.New(400, errcode.CodeUsernameExists, fmt.Sprintf("用户名 %s 已被占用", username))
	}
	if email != "" {
		if exists, e := s.fieldExists(ctx, "email", email, 0); e == nil && exists {
			return errcode.New(400, errcode.CodeEmailExists, "该邮箱已被注册")
		}
	}
	if phone != "" {
		if exists, e := s.fieldExists(ctx, "phone", phone, 0); e == nil && exists {
			return errcode.New(400, errcode.CodePhoneExists, "该手机号已被注册")
		}
	}
	return errcode.New(400, errcode.CodeUsernameExists, "唯一性冲突（用户名/邮箱/手机号已被占用）")
}

// isDuplicateKeyErr 判断是否 MySQL 唯一索引冲突（1062）。
func isDuplicateKeyErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	return strings.Contains(err.Error(), "Duplicate entry") || strings.Contains(err.Error(), "1062")
}

// ---- 12.3 登录 ----

// ClientLoginRequest 是会员登录请求（username 兼容 用户名/邮箱/手机号）。
type ClientLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// ClientInfo 是会员对外信息（不含敏感字段）。
type ClientInfo struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	RealName string `json:"real_name"`
}

// ClientLoginResult 是会员登录结果。
type ClientLoginResult struct {
	Token     string     `json:"token"`
	ExpiresAt time.Time  `json:"expires_at"`
	Client    ClientInfo `json:"client"`
}

// Login 会员登录：主体三合一匹配 → bcrypt → 状态检查 → 签发会员 JWT。
// 防枚举：主体不存在与密码错误统一 invalid_credentials；软删除视为不存在。
func (s *ClientService) Login(ctx context.Context, req *ClientLoginRequest) (*ClientLoginResult, error) {
	identity := strings.TrimSpace(req.Username)
	if identity == "" || req.Password == "" {
		return nil, errcode.InvalidParam("username 与 password 均不能为空")
	}
	db := s.app.GetDB()
	var client model.Client
	err := db.WithContext(ctx).
		Where("username = ? OR email = ? OR phone = ?", identity, identity, identity).
		First(&client).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.InvalidCredentials()
	}
	if err != nil {
		return nil, errcode.Internal("查询会员失败").WithErr(err)
	}
	if client.IsDeleted() {
		return nil, errcode.InvalidCredentials() // 软删除视为不存在（防枚举）
	}
	if err := bcrypt.CompareHashAndPassword([]byte(client.PasswordHash), []byte(req.Password)); err != nil {
		return nil, errcode.InvalidCredentials()
	}
	if client.Status == model.ClientStatusDisabled {
		return nil, errcode.AccountDisabled()
	}
	// status=3（上游关联异常）不阻断登录（契约 12.7）

	now := time.Now().Local()
	if err := db.WithContext(ctx).Model(&client).Update("last_login_at", &now).Error; err != nil {
		slog.Warn("更新会员登录时间失败", "client_id", client.ID, "err", err)
	}
	client.LastLoginAt = &now

	expire := time.Duration(s.app.Cfg().JWT.ClientExpireHours) * time.Hour
	token, expiresAt, err := jwtutil.SignClient(s.app.Cfg().JWT.Secret, expire, client.ID, client.Username)
	if err != nil {
		return nil, errcode.Internal("签发 token 失败").WithErr(err)
	}
	return &ClientLoginResult{
		Token:     token,
		ExpiresAt: expiresAt,
		Client: ClientInfo{
			ID: client.ID, Username: client.Username,
			Email: strVal(client.Email), Phone: strVal(client.Phone), RealName: client.RealName,
		},
	}, nil
}

// ---- 12.4 资料与改密 ----

// ClientProfile 是会员资料视图。
type ClientProfile struct {
	ID        uint   `json:"id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	RealName  string `json:"real_name"`
	Company   string `json:"company"`
	Address   string `json:"address"`
	CreatedAt string `json:"created_at"`
}

// Profile 读取会员资料。
func (s *ClientService) Profile(ctx context.Context, id uint) (*ClientProfile, error) {
	client, err := s.mustFindActive(ctx, id)
	if err != nil {
		return nil, err
	}
	return toClientProfile(client), nil
}

// UpdateProfileRequest 是资料修改请求（字段可选；email/phone 提交空串=解绑置 NULL）。
type UpdateProfileRequest struct {
	RealName *string `json:"real_name"`
	Company  *string `json:"company"`
	Address  *string `json:"address"`
	Email    *string `json:"email"`
	Phone    *string `json:"phone"`
}

// UpdateProfile 修改资料（username 不可改；email/phone 唯一性排除自己）。
func (s *ClientService) UpdateProfile(ctx context.Context, id uint, req *UpdateProfileRequest) (*ClientProfile, error) {
	db := s.app.GetDB()
	client, err := s.mustFindActive(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.RealName == nil && req.Company == nil && req.Address == nil && req.Email == nil && req.Phone == nil {
		return nil, errcode.InvalidParam("请求体为空：至少提交一个可修改字段")
	}
	updates := map[string]any{}
	if req.RealName != nil {
		v := strings.TrimSpace(*req.RealName)
		if len([]rune(v)) > 64 {
			return nil, errcode.InvalidParam("real_name 不超过 64 字符")
		}
		updates["real_name"] = v
	}
	if req.Company != nil {
		v := strings.TrimSpace(*req.Company)
		if len([]rune(v)) > 128 {
			return nil, errcode.InvalidParam("company 不超过 128 字符")
		}
		updates["company"] = v
	}
	if req.Address != nil {
		v := strings.TrimSpace(*req.Address)
		if len([]rune(v)) > 255 {
			return nil, errcode.InvalidParam("address 不超过 255 字符")
		}
		updates["address"] = v
	}
	if req.Email != nil {
		v := strings.TrimSpace(*req.Email)
		if v == "" {
			updates["email"] = nil // 解绑
		} else {
			if !emailRe.MatchString(v) {
				return nil, errcode.InvalidParam("邮箱格式不正确")
			}
			if exists, e := s.fieldExists(ctx, "email", v, id); e != nil {
				return nil, e
			} else if exists {
				return nil, errcode.New(400, errcode.CodeEmailExists, "该邮箱已被其他账号使用")
			}
			updates["email"] = v
		}
	}
	if req.Phone != nil {
		v := strings.TrimSpace(*req.Phone)
		if v == "" {
			updates["phone"] = nil // 解绑
		} else {
			if !clientPhoneRe.MatchString(v) {
				return nil, errcode.InvalidParam("手机号格式不正确")
			}
			if exists, e := s.fieldExists(ctx, "phone", v, id); e != nil {
				return nil, e
			} else if exists {
				return nil, errcode.New(400, errcode.CodePhoneExists, "该手机号已被其他账号使用")
			}
			updates["phone"] = v
		}
	}
	if err := db.WithContext(ctx).Model(client).Updates(updates).Error; err != nil {
		if isDuplicateKeyErr(err) {
			return nil, errcode.New(400, errcode.CodeEmailExists, "邮箱或手机号唯一性冲突")
		}
		return nil, errcode.Internal("更新资料失败").WithErr(err)
	}
	client, err = s.mustFindActive(ctx, id)
	if err != nil {
		return nil, err
	}
	return toClientProfile(client), nil
}

// ChangePasswordRequest 是修改密码请求。
type ChangePasswordRequest struct {
	OldPassword     string `json:"old_password"`
	NewPassword     string `json:"new_password"`
	ConfirmPassword string `json:"confirm_password"`
}

// ChangePassword 会员自改密码（需旧密码；新密码规则同注册）。
func (s *ClientService) ChangePassword(ctx context.Context, id uint, req *ChangePasswordRequest) error {
	db := s.app.GetDB()
	client, err := s.mustFindActive(ctx, id)
	if err != nil {
		return err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(client.PasswordHash), []byte(req.OldPassword)); err != nil {
		return errcode.InvalidCredentials() // 旧密码错误按 401 invalid_credentials（契约 12.4）
	}
	if err := validateClientPassword(req.NewPassword); err != nil {
		return err
	}
	if req.NewPassword != req.ConfirmPassword {
		return errcode.InvalidParam("两次输入的密码不一致")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return errcode.Internal("密码哈希失败").WithErr(err)
	}
	if err := db.WithContext(ctx).Model(client).Update("password_hash", string(hash)).Error; err != nil {
		return errcode.Internal("更新密码失败").WithErr(err)
	}
	slog.Info("会员修改密码成功", "client_id", id)
	return nil
}

// ---- 12.6 后台会员管理 ----

// ClientView 是后台会员列表视图。
type ClientView struct {
	ID               uint   `json:"id"`
	Username         string `json:"username"`
	Email            string `json:"email"`
	Phone            string `json:"phone"`
	RealName         string `json:"real_name"`
	Company          string `json:"company"`
	Status           int8   `json:"status"`
	StatusText       string `json:"status_text"`
	UpstreamUID      *int64 `json:"upstream_uid"`
	UpstreamLinkedAt string `json:"upstream_linked_at"`
	LastLoginAt      string `json:"last_login_at"`
	CreatedAt        string `json:"created_at"`
}

// ClientListView 是后台会员列表响应。
type ClientListView struct {
	List  []ClientView `json:"list"`
	Total int64        `json:"total"`
}

// AdminList 后台会员列表（keyword 模糊 username/email/phone；status 空=默认排除已删除）。
func (s *ClientService) AdminList(ctx context.Context, keyword, status string, page, pageSize int, lang string) (*ClientListView, error) {
	db := s.app.GetDB()
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	q := db.WithContext(ctx).Model(&model.Client{})
	if strings.TrimSpace(status) != "" {
		st, err := parseClientStatus(status)
		if err != nil {
			return nil, err
		}
		q = q.Where("status = ?", st)
	} else {
		q = q.Where("status != ?", model.ClientStatusDeleted) // 默认隐藏软删除
	}
	if kw := strings.TrimSpace(keyword); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("username LIKE ? OR email LIKE ? OR phone LIKE ?", like, like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, errcode.Internal("统计会员失败").WithErr(err)
	}
	var rows []model.Client
	if err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, errcode.Internal("查询会员失败").WithErr(err)
	}
	list := make([]ClientView, 0, len(rows))
	for i := range rows {
		list = append(list, toClientView(&rows[i], lang))
	}
	return &ClientListView{List: list, Total: total}, nil
}

// AdminSetStatus 后台启用/禁用/标记上游异常（status ∈ {0,1,3}；2 仅能经 DELETE）。
func (s *ClientService) AdminSetStatus(ctx context.Context, id uint, status int8, lang string) (*ClientView, error) {
	if status != model.ClientStatusDisabled && status != model.ClientStatusActive && status != model.ClientStatusUpstreamIssue {
		return nil, errcode.InvalidParam("status 只能是 0（禁用）/1（激活）/3（上游关联异常）；删除请用 DELETE 接口")
	}
	db := s.app.GetDB()
	client, err := s.mustFindNotDeleted(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := db.WithContext(ctx).Model(client).Update("status", status).Error; err != nil {
		return nil, errcode.Internal("更新状态失败").WithErr(err)
	}
	client.Status = status
	slog.Info("后台修改会员状态", "client_id", id, "status", status)
	view := toClientView(client, lang)
	return &view, nil
}

// AdminResetPassword 后台重置会员密码（不需要旧密码；记审计日志）。
func (s *ClientService) AdminResetPassword(ctx context.Context, adminID, id uint, newPassword string) error {
	if err := validateClientPassword(newPassword); err != nil {
		return err
	}
	db := s.app.GetDB()
	client, err := s.mustFindNotDeleted(ctx, id)
	if err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return errcode.Internal("密码哈希失败").WithErr(err)
	}
	if err := db.WithContext(ctx).Model(client).Update("password_hash", string(hash)).Error; err != nil {
		return errcode.Internal("重置密码失败").WithErr(err)
	}
	slog.Info("后台重置会员密码", "admin_id", adminID, "client_id", id) // 审计：谁重置了谁
	return nil
}

// AdminDelete 软删除会员：status=2 + deleted_at + 唯一字段加 "#del<id>" 后缀释放索引。
// 重复删除幂等返回成功。
func (s *ClientService) AdminDelete(ctx context.Context, id uint) error {
	db := s.app.GetDB()
	client, err := s.mustFindByID(ctx, id)
	if err != nil {
		return err
	}
	if client.IsDeleted() {
		return nil // 幂等
	}
	now := time.Now().Local()
	suffix := fmt.Sprintf("#del%d", client.ID)
	updates := map[string]any{
		"status":     model.ClientStatusDeleted,
		"deleted_at": &now,
		"username":   truncateTo(client.Username+suffix, 64),
	}
	if client.Email != nil && *client.Email != "" {
		updates["email"] = truncateTo(*client.Email+suffix, 128)
	}
	if client.Phone != nil && *client.Phone != "" {
		updates["phone"] = truncateTo(*client.Phone+suffix, 20)
	}
	if err := db.WithContext(ctx).Model(client).Updates(updates).Error; err != nil {
		if isDuplicateKeyErr(err) {
			return errcode.Internal("软删除唯一字段冲突（重复删除同名账号），请人工检查 client_id=%d", id)
		}
		return errcode.Internal("软删除失败").WithErr(err)
	}
	slog.Info("后台软删除会员", "client_id", id, "username", client.Username)
	return nil
}

// ---- 内部助手 ----

func (s *ClientService) mustFindByID(ctx context.Context, id uint) (*model.Client, error) {
	db := s.app.GetDB()
	var client model.Client
	err := db.WithContext(ctx).First(&client, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.New(404, errcode.CodeClientNotFound, "会员不存在")
	}
	if err != nil {
		return nil, errcode.Internal("查询会员失败").WithErr(err)
	}
	return &client, nil
}

// mustFindNotDeleted 查非删除态会员（后台操作用）。
func (s *ClientService) mustFindNotDeleted(ctx context.Context, id uint) (*model.Client, error) {
	client, err := s.mustFindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if client.IsDeleted() {
		return nil, errcode.New(404, errcode.CodeClientNotFound, "会员不存在（已删除）")
	}
	return client, nil
}

// mustFindActive 查可登录态会员（会员自助接口用；禁用也放行——资料可看，登录被拦）。
func (s *ClientService) mustFindActive(ctx context.Context, id uint) (*model.Client, error) {
	return s.mustFindNotDeleted(ctx, id)
}

func parseClientStatus(s string) (int8, error) {
	n := 0
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil || n < 0 || n > 3 {
		return 0, errcode.InvalidParam("status 只能是 0/1/2/3")
	}
	return int8(n), nil
}

func toClientProfile(c *model.Client) *ClientProfile {
	return &ClientProfile{
		ID: c.ID, Username: c.Username,
		Email: strVal(c.Email), Phone: strVal(c.Phone),
		RealName: c.RealName, Company: c.Company, Address: c.Address,
		CreatedAt: c.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}

func toClientView(c *model.Client, lang string) ClientView {
	v := ClientView{
		ID: c.ID, Username: c.Username,
		Email: strVal(c.Email), Phone: strVal(c.Phone),
		RealName: c.RealName, Company: c.Company,
		Status: c.Status, StatusText: clientStatusText(c.Status, lang),
		UpstreamUID: c.UpstreamUID,
		CreatedAt:   c.CreatedAt.Format("2006-01-02 15:04:05"),
	}
	if c.UpstreamLinkedAt != nil {
		v.UpstreamLinkedAt = c.UpstreamLinkedAt.Format("2006-01-02 15:04:05")
	}
	if c.LastLoginAt != nil {
		v.LastLoginAt = c.LastLoginAt.Format("2006-01-02 15:04:05")
	}
	return v
}

// clientStatusText 输出状态文案（按请求语言，前端直接展示）。
func clientStatusText(status int8, lang string) string {
	if strings.HasPrefix(strings.ToLower(lang), "en") {
		switch status {
		case model.ClientStatusDisabled:
			return "Disabled"
		case model.ClientStatusActive:
			return "Active"
		case model.ClientStatusDeleted:
			return "Deleted"
		case model.ClientStatusUpstreamIssue:
			return "Upstream Issue"
		}
		return "Unknown"
	}
	switch status {
	case model.ClientStatusDisabled:
		return "禁用"
	case model.ClientStatusActive:
		return "激活"
	case model.ClientStatusDeleted:
		return "已删除"
	case model.ClientStatusUpstreamIssue:
		return "上游关联异常"
	}
	return "未知"
}

// strVal 解引用可空字符串。
func strVal(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// nilIfEmpty 空串转 NULL 指针（可空唯一列）。
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// truncateTo 按字节安全截断（软删除加后缀防溢出列宽）。
func truncateTo(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
