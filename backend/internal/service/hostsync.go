package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"gorm.io/gorm"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/model"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/app"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/errcode"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/upstream"
)

// HostSyncService 处理上游 → 下游的主机推送（POST /api/upstream/host/sync）。
//
// 安全模型（契约文档 9.6）：该端点免 JWT（上游服务器调用），以魔方标准
// MD5 签名验证代替登录态；不在未安装门禁白名单内——未安装态直接 409。
type HostSyncService struct {
	app *app.App
}

// NewHostSync 构造主机同步服务。
func NewHostSync(a *app.App) *HostSyncService {
	return &HostSyncService{app: a}
}

// HandleSync 验签 → upsert hosts（按 upstream_host_id）→ 由 handler 返回 200。
// rawBody 必须是请求体原文（签名与 raw 备份都依赖原始字节）。
func (s *HostSyncService) HandleSync(ctx context.Context, rawBody []byte) error {
	db := s.app.GetDB()
	if db == nil {
		return errcode.Internal("数据库未就绪")
	}

	// 1. 验签（token 未配置 / 签名不匹配 → 403 signature_invalid）
	token, ok, err := model.GetSetting(db.WithContext(ctx), model.SettingDownstreamToken)
	if err != nil {
		return errcode.Internal("读取下游密钥失败").WithErr(err)
	}
	if !ok || token == "" {
		return errcode.New(403, errcode.CodeSignatureInvalid, "下游密钥未配置，拒绝推送（请先在后台·上游对接生成密钥并登记到上游）")
	}
	if err := upstream.VerifySignature(rawBody, token); err != nil {
		slog.Warn("host/sync 验签失败", "err", err, "body_size", len(rawBody))
		return errcode.New(403, errcode.CodeSignatureInvalid, "签名验证失败")
	}

	// 2. 宽松解析推送字段（json.Number 保留数字原文；缺失字段容错为空）
	dec := json.NewDecoder(bytes.NewReader(rawBody))
	dec.UseNumber()
	var body map[string]any
	if err := dec.Decode(&body); err != nil {
		return errcode.InvalidParam("推送请求体不是合法 JSON")
	}
	hostID, err := parseUpstreamHostID(body["host_id"])
	if err != nil {
		return errcode.InvalidParam("%v（host_id 为上游主机 ID，必填）", err)
	}

	// 3. upsert（按 upstream_host_id）
	now := time.Now().Local()
	row := model.Host{
		UpstreamHostID: hostID,
		Domain:         jsonFieldToString(body["domain"]),
		Username:       jsonFieldToString(body["username"]),
		Password:       jsonFieldToString(body["password"]), // 上游推送即明文，原样保存
		DedicatedIP:    jsonFieldToString(body["dedicatedip"]),
		AssignedIPs:    jsonFieldToString(body["assignedips"]),
		Port:           jsonFieldToString(body["port"]),
		OS:             jsonFieldToString(body["os"]),
		OSUrl:          jsonFieldToString(body["os_url"]),
		NextDueDate:    jsonFieldToString(body["nextduedate"]),
		DomainStatus:   jsonFieldToString(body["domainstatus"]),
		Raw:            string(rawBody), // 原文备份（截断保护见下）
		SyncedAt:       now,
	}
	if len(row.Raw) > 60000 { // text 列安全上限，超长推送截断备份（业务字段已提取）
		row.Raw = row.Raw[:60000]
	}

	var existing model.Host
	err = db.WithContext(ctx).Where("upstream_host_id = ?", hostID).First(&existing).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		if e := db.WithContext(ctx).Create(&row).Error; e != nil {
			return errcode.Internal("写入主机记录失败: %v", e)
		}
		slog.Info("host/sync 新主机落库", "upstream_host_id", hostID, "domain", row.Domain, "status", row.DomainStatus)
	case err != nil:
		return errcode.Internal("查询主机记录失败").WithErr(err)
	default:
		row.ID = existing.ID
		row.ProductID = existing.ProductID // 保留批 3 回填的关联
		row.ClientID = existing.ClientID
		row.CreatedAt = existing.CreatedAt
		// 注意：GORM 的 map 形式 Updates 直接把键当数据库列名（不走命名策略），
		// 必须用蛇形列名（dedicated_ip 等）而非 JSON 字段名（dedicatedip）。
		if e := db.WithContext(ctx).Model(&existing).Updates(map[string]any{
			"domain": row.Domain, "username": row.Username, "password": row.Password,
			"dedicated_ip": row.DedicatedIP, "assigned_ips": row.AssignedIPs,
			"port": row.Port, "os": row.OS, "os_url": row.OSUrl,
			"next_due_date": row.NextDueDate, "domain_status": row.DomainStatus,
			"raw": row.Raw, "synced_at": row.SyncedAt,
		}).Error; e != nil {
			return errcode.Internal("更新主机记录失败: %v", e)
		}
		slog.Info("host/sync 主机更新落库", "upstream_host_id", hostID, "domain", row.Domain, "status", row.DomainStatus)
	}
	return nil
}

// parseUpstreamHostID 解析 host_id（数字/字符串数字均可；缺失或非法报错）。
func parseUpstreamHostID(v any) (int64, error) {
	switch t := v.(type) {
	case json.Number:
		id, err := t.Int64()
		if err != nil || id <= 0 {
			return 0, errors.New("host_id 必须是正整数")
		}
		return id, nil
	case string:
		id, err := strconv.ParseInt(t, 10, 64)
		if err != nil || id <= 0 {
			return 0, errors.New("host_id 必须是正整数")
		}
		return id, nil
	case float64:
		id := int64(t)
		if id <= 0 {
			return 0, errors.New("host_id 必须是正整数")
		}
		return id, nil
	default:
		return 0, errors.New("缺少 host_id 字段")
	}
}

// jsonFieldToString 宽松转换推送字段为字符串：
// 字符串原样；数字保留原文；数组/对象转紧凑 JSON（如 assignedips 可能为数组）。
func jsonFieldToString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case nil:
		return ""
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		return string(b)
	}
}
