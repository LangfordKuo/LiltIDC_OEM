package service

// 余额域（契约文档第 14 章）：
//   - clients.credit 列（DECIMAL(12,2)，Go 字符串承载，运算走 money 整数分，ADR-027）；
//   - balance_transactions 流水：任何余额变更**必须**与流水在同一事务内写入（ADR-028）；
//   - 并发防透支：事务内 SELECT … FOR UPDATE 行级锁 + 校验后更新
//     （并发支付/调账串行化在同一行锁上，单测/集成测试覆盖）。

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/model"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/app"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/errcode"
	"github.com/LangfordKuo/LiltIDC_OEM/backend/internal/pkg/money"
)

// BalanceService 处理余额业务（会员查询 + 后台人工调账）。
type BalanceService struct {
	app *app.App
}

// NewBalance 构造余额服务。
func NewBalance(a *app.App) *BalanceService {
	return &BalanceService{app: a}
}

// maxAdminAdjustCents 是后台单次调账绝对值上限（100 万元——防误输入天量金额）。
const maxAdminAdjustCents int64 = 100_000_00

// AdminAdjustRequest 是后台人工充值/扣款请求。
type AdminAdjustRequest struct {
	Amount string `json:"amount"` // 带符号十进制文本："+50.00" 充值 / "-20" 扣款
	Remark string `json:"remark"` // 备注（≤255 字符）
}

// BalanceView 是余额视图（后台调账响应）。
type BalanceView struct {
	ClientID    uint                    `json:"client_id"`
	Credit      string                  `json:"credit"`
	Transaction *BalanceTransactionView `json:"transaction"`
}

// BalanceTransactionView 是流水对外视图。
type BalanceTransactionView struct {
	ID           uint   `json:"id"`
	Type         string `json:"type"`
	TypeText     string `json:"type_text"`
	Amount       string `json:"amount"`
	BalanceAfter string `json:"balance_after"`
	RelatedType  string `json:"related_type"`
	RelatedID    uint   `json:"related_id"`
	Remark       string `json:"remark"`
	AdminID      *uint  `json:"admin_id"`
	CreatedAt    string `json:"created_at"`
}

// AdminAdjust 后台人工充值/扣款：余额变更与流水同事务（行锁防并发）。
func (s *BalanceService) AdminAdjust(ctx context.Context, adminID, clientID uint, req *AdminAdjustRequest, lang string) (*BalanceView, error) {
	delta, err := money.ParseCents(req.Amount)
	if err != nil {
		return nil, errcode.InvalidParam("amount 必须是十进制金额文本（可带 +/- 号），如 \"+50.00\" 或 \"-20.00\": %v", err)
	}
	if delta == 0 {
		return nil, errcode.InvalidParam("amount 不能为 0")
	}
	if delta > maxAdminAdjustCents || delta < -maxAdminAdjustCents {
		return nil, errcode.InvalidParam("单次调账金额绝对值不能超过 %s 元", money.FormatCents(maxAdminAdjustCents))
	}
	remark := strings.TrimSpace(req.Remark)
	if len([]rune(remark)) > 255 {
		return nil, errcode.InvalidParam("remark 不超过 255 字符")
	}
	txType := model.BalanceTypeRechargeAdmin
	if delta < 0 {
		txType = model.BalanceTypeAdjust
	}

	db := s.app.GetDB()
	var afterCents int64
	var txRow model.BalanceTransaction
	aid := adminID
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		client, err := lockClientForUpdate(ctx, tx, clientID)
		if err != nil {
			return err
		}
		afterCents, err = creditBalanceTx(ctx, tx, client, delta, txType, "", 0, remark, &aid)
		if err != nil {
			return err
		}
		// 取回刚写入的流水行（同事务内最后一条）
		return tx.WithContext(ctx).
			Where("client_id = ? AND type = ?", clientID, txType).
			Order("id DESC").First(&txRow).Error
	})
	if err != nil {
		return nil, err
	}
	slog.Info("后台调整会员余额", "admin_id", adminID, "client_id", clientID,
		"amount", money.FormatCents(delta), "balance_after", money.FormatCents(afterCents))
	view := toBalanceTransactionView(&txRow, lang)
	return &BalanceView{ClientID: clientID, Credit: money.FormatCents(afterCents), Transaction: &view}, nil
}

// ClientBalanceView 是会员端余额视图（余额 + 流水分页）。
type ClientBalanceView struct {
	Credit   string                   `json:"credit"`
	List     []BalanceTransactionView `json:"list"`
	Total    int64                    `json:"total"`
	Page     int                      `json:"page"`
	PageSize int                      `json:"page_size"`
}

// ClientBalance 会员查询自己的余额与流水（分页，倒序）。
func (s *BalanceService) ClientBalance(ctx context.Context, clientID uint, page, pageSize int, lang string) (*ClientBalanceView, error) {
	db := s.app.GetDB()
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	client, err := mustFindClientByID(ctx, db, clientID)
	if err != nil {
		return nil, err
	}
	credit := client.Credit
	if credit == "" {
		credit = "0.00"
	}
	q := db.WithContext(ctx).Model(&model.BalanceTransaction{}).Where("client_id = ?", clientID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, errcode.Internal("统计余额流水失败").WithErr(err)
	}
	var rows []model.BalanceTransaction
	if err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, errcode.Internal("查询余额流水失败").WithErr(err)
	}
	list := make([]BalanceTransactionView, 0, len(rows))
	for i := range rows {
		list = append(list, toBalanceTransactionView(&rows[i], lang))
	}
	return &ClientBalanceView{Credit: credit, List: list, Total: total, Page: page, PageSize: pageSize}, nil
}

// ---- 事务内共享助手（订单支付/退款/人工调账复用，全部要求在 *gorm.DB 事务句柄内调用） ----

// lockClientForUpdate 事务内按行级锁加载会员（SELECT … FOR UPDATE）。
// 并发扣减/回充在同一行锁上串行化，锁内校验余额杜绝透支（ADR-028）。
func lockClientForUpdate(ctx context.Context, tx *gorm.DB, clientID uint) (*model.Client, error) {
	var client model.Client
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&client, clientID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.New(404, errcode.CodeClientNotFound, "会员不存在")
	}
	if err != nil {
		return nil, errcode.Internal("查询会员失败").WithErr(err)
	}
	if client.IsDeleted() {
		return nil, errcode.New(404, errcode.CodeClientNotFound, "会员不存在（已删除）")
	}
	return &client, nil
}

// clientCreditCents 读取会员当前余额（分）；credit 列异常值容错为 0 并告警。
func clientCreditCents(client *model.Client) int64 {
	if client.Credit == "" {
		return 0
	}
	cents, err := money.ParseCents(client.Credit)
	if err != nil {
		slog.Error("会员余额列值非法（按 0 处理，请人工检查）", "client_id", client.ID, "credit", client.Credit)
		return 0
	}
	return cents
}

// debitBalanceTx 扣减余额并写 consume 流水（事务内；余额不足返回 balance_insufficient）。
// 返回扣减后的余额（分）。调用方须先经 lockClientForUpdate 拿到行锁下的 client。
func debitBalanceTx(ctx context.Context, tx *gorm.DB, client *model.Client, amountCents int64,
	relatedType string, relatedID uint, remark string) (int64, error) {
	if amountCents <= 0 {
		return 0, errcode.Internal("扣减金额必须为正数（分）: %d", amountCents)
	}
	before := clientCreditCents(client)
	if before < amountCents {
		return 0, errcode.New(400, errcode.CodeBalanceInsufficient,
			fmt.Sprintf("余额不足：需 %s 元，当前余额 %s 元",
				money.FormatCents(amountCents), money.FormatCents(before)))
	}
	after := before - amountCents
	return applyBalanceChange(ctx, tx, client, after, model.BalanceTransaction{
		ClientID:     client.ID,
		Type:         model.BalanceTypeConsume,
		Amount:       money.FormatCents(-amountCents), // 扣减记负数
		BalanceAfter: money.FormatCents(after),
		RelatedType:  relatedType,
		RelatedID:    relatedID,
		Remark:       remark,
	})
}

// creditBalanceTx 回充余额并写流水（退款 refund / 人工 recharge_admin、adjust）。
// amountCents 带符号（正=入账）；返回变更后余额（分）。
func creditBalanceTx(ctx context.Context, tx *gorm.DB, client *model.Client, amountCents int64,
	txType, relatedType string, relatedID uint, remark string, adminID *uint) (int64, error) {
	if amountCents == 0 {
		return clientCreditCents(client), errcode.Internal("回充金额不能为 0")
	}
	before := clientCreditCents(client)
	after := before + amountCents
	if after < 0 {
		return 0, errcode.New(400, errcode.CodeBalanceInsufficient,
			fmt.Sprintf("余额不足：当前余额 %s 元，不足以扣减 %s 元",
				money.FormatCents(before), money.FormatCents(-amountCents)))
	}
	return applyBalanceChange(ctx, tx, client, after, model.BalanceTransaction{
		ClientID:     client.ID,
		Type:         txType,
		Amount:       money.FormatCents(amountCents),
		BalanceAfter: money.FormatCents(after),
		RelatedType:  relatedType,
		RelatedID:    relatedID,
		Remark:       remark,
		AdminID:      adminID,
	})
}

// applyBalanceChange 落库：更新 clients.credit + 插入流水（同一事务句柄，原子性由
// 调用方的 Transaction 保证——余额变更与流水永远同生共死，ADR-028）。
func applyBalanceChange(ctx context.Context, tx *gorm.DB, client *model.Client, afterCents int64, row model.BalanceTransaction) (int64, error) {
	after := money.FormatCents(afterCents)
	if err := tx.WithContext(ctx).Model(client).Update("credit", after).Error; err != nil {
		return 0, errcode.Internal("更新会员余额失败").WithErr(err)
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now().Local()
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, errcode.Internal("写入余额流水失败").WithErr(err)
	}
	client.Credit = after // 同步内存态，供同事务后续步骤读取
	return afterCents, nil
}

// mustFindClientByID 按 ID 查会员（非事务场景）。
func mustFindClientByID(ctx context.Context, db *gorm.DB, clientID uint) (*model.Client, error) {
	var client model.Client
	err := db.WithContext(ctx).First(&client, clientID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.New(404, errcode.CodeClientNotFound, "会员不存在")
	}
	if err != nil {
		return nil, errcode.Internal("查询会员失败").WithErr(err)
	}
	return &client, nil
}

// toBalanceTransactionView 流水视图转换（类型文案按请求语言输出）。
func toBalanceTransactionView(t *model.BalanceTransaction, lang string) BalanceTransactionView {
	return BalanceTransactionView{
		ID: t.ID, Type: t.Type, TypeText: balanceTypeText(t.Type, lang),
		Amount: t.Amount, BalanceAfter: t.BalanceAfter,
		RelatedType: t.RelatedType, RelatedID: t.RelatedID,
		Remark: t.Remark, AdminID: t.AdminID,
		CreatedAt: t.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}

// balanceTypeText 输出流水类型文案（范式同 clientStatusText）。
func balanceTypeText(txType, lang string) string {
	en := strings.HasPrefix(strings.ToLower(lang), "en")
	switch txType {
	case model.BalanceTypeRechargeAdmin:
		if en {
			return "Admin Recharge"
		}
		return "人工充值"
	case model.BalanceTypeConsume:
		if en {
			return "Consume"
		}
		return "消费"
	case model.BalanceTypeRefund:
		if en {
			return "Refund"
		}
		return "退款"
	case model.BalanceTypeAdjust:
		if en {
			return "Adjustment"
		}
		return "人工调整"
	}
	if en {
		return "Unknown"
	}
	return "未知"
}
