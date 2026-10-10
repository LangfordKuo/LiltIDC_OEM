package upstream

// 上游写操作端点（批 3-β 解锁：加购/结算/余额支付/下游登记/主机详情）。
//
// 参数形状逐字对齐 ZJMF-CBAP 开源实现（mayio2008/zjmf）并已在 lyew.com 真实试探：
//   - 魔方互联调用（zjmfCurl/commonCurl）POST 一律 application/x-www-form-urlencoded
//     （http_build_query，嵌套数组用 cart_data[pid] 形态），本文件同形发送；
//   - 购买链路 = add_to_shop（预检入会话购物车）→ settle（内联 cart_data 结算，
//     会清空会话购物车，天然规避陈旧残留）→ apply_credit（余额支付账单）。
//     ⚠ settle 本身**不扣款**（total>0 时只建 Unpaid 账单并返回 invoiceid）；
//     余额支付走 /apply_credit（use_credit=1&enough=1），成功状态码是 **1001**
//     （"支付完成"，data.hostid 为主机 ID 列表）——源码位点：
//     app/common/logic/Host.php（上游互联购买流程原文）、
//     app/home/controller/CartController.php::settle、PayController.php::applyCredit。
//   - ⚠ settle/apply_credit **不要携带 downstream_* 参数**：settle 对
//     downstream_token 有 whereLike 幂等检查，携带全局共享 token 时第二次购买会
//     命中第一台的 stream_info 而**静默回滚整笔新订单**（只返回旧 hostid）。
//     下游登记改在开通完成后调 /host/setdownstream（见 ADR-026）。
//   - /cart/summary 对 API 密钥身份 403（Machine credentials cannot access
//     member security operations，lyew.com 实测），本系统不调用。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// BizError 是上游业务拒绝错误（信封 status 非成功码，携带上游原文 msg）。
//
// 与网络/超时/协议错误区分开：service 层对「明确业务拒绝」（余额不足/商品下架等）
// 走自动退款补偿，对「网络性未知错误」冻结资金转人工（补偿路径见 ADR-029）。
type BizError struct {
	Path   string // 请求路径（诊断用）
	Status int    // 上游信封 status（400/406/410/415…）
	Msg    string // 上游 msg 原文
}

// Error 实现 error 接口。
func (e *BizError) Error() string {
	return fmt.Sprintf("上游业务拒绝（%s status=%d）: %s", e.Path, e.Status, e.Msg)
}

// postForm 以 form-urlencoded 发送 POST 并解包 data 信封。
// 成功状态码集合由调用方指定（写端点存在 1001=成功语义，读端点不变）。
func (c *Client) postForm(ctx context.Context, path string, form url.Values, success []int, out any) error {
	body := form.Encode()
	return c.doBody(ctx, "POST", path, nil, body,
		"application/x-www-form-urlencoded", success, out)
}

// AddToShopResult 是加购结果（上游只回 status/msg，无 data）。
type AddToShopResult struct {
	OK bool
}

// AddToShopParams 是加购参数（可配置项/自定义字段缺省时上游自动选默认值，
// 源码 Shop::checkProductToArr 的 auto-fill 行为，lyew.com 实测省略可成功）。
type AddToShopParams struct {
	Pid          int64             // 上游商品 ID
	BillingCycle string            // 周期键（monthly 等）
	Qty          int               // 数量（>=1）
	CurrencyID   int64             // 货币 ID（/cart/credit 的 currency.id）
	Host         string            // 主机名（商品要求时传；规则见商品 host 配置）
	Password     string            // 主机密码（商品要求时传）
	ConfigOption map[string]string // 可配置项 config_id→sub_id/数量（可空）
	CustomField  map[string]string // 自定义字段（可空）
}

// AddToShop 加入购物车（POST /cart/add_to_shop，form 编码）。
// 用作下单前的预检（库存/周期/绑定手机校验），成功返回 {status:200,msg:"添加成功"}。
func (c *Client) AddToShop(ctx context.Context, p AddToShopParams) error {
	form := url.Values{}
	form.Set("pid", strconv.FormatInt(p.Pid, 10))
	form.Set("billingcycle", p.BillingCycle)
	form.Set("qty", strconv.Itoa(p.Qty))
	form.Set("serverid", "0")
	form.Set("currencyid", strconv.FormatInt(p.CurrencyID, 10))
	form.Set("checkout", "0")
	if p.Host != "" {
		form.Set("host", p.Host)
	}
	if p.Password != "" {
		form.Set("password", p.Password)
	}
	for k, v := range p.ConfigOption {
		form.Set("configoption["+k+"]", v)
	}
	for k, v := range p.CustomField {
		form.Set("customfield["+k+"]", v)
	}
	var ignored json.RawMessage
	return c.postForm(ctx, "/cart/add_to_shop", form, []int{200}, &ignored)
}

// SettleParams 是结算参数（内联 cart_data 单商品形态——ZJMF 自家下游购买原文形状，
// Host.php：settle 收到 cart_data 会**清空会话购物车**并只结算这一件，
// 天然规避 add_to_shop 残留导致的重复购买）。
type SettleParams struct {
	Pid           int64
	BillingCycle  string
	Qty           int
	CurrencyID    int64
	Host          string
	Password      string
	ConfigOptions map[string]string // cart_data[configoptions][k]=v
	Notes         string            // 订单备注（写本地订单号，便于上游对账）
}

// SettleResult 是结算结果。
type SettleResult struct {
	InvoiceID int64   // 上游账单 ID（total>0 时返回；免费单为 0）
	HostIDs   []int64 // 主机 ID（免费单 status=1001 时直接返回；付费单在 apply_credit 后取得）
	PaidFree  bool    // true = 0 元单已由 settle 直接完成（status 1001）
}

// Settle 结算下单（POST /cart/settle，form 编码）。
// 响应形状（源码 CartController::settle 尾部）：
//   - total>0：{status:200, data:{invoiceid:N}}（账单 Unpaid，需再调 ApplyCredit 支付）；
//   - total==0：{status:1001, msg:"购买成功", data:{hostid:[…], url}}（免支付直接开通）。
func (c *Client) Settle(ctx context.Context, p SettleParams) (*SettleResult, error) {
	form := url.Values{}
	form.Set("cart_data[pid]", strconv.FormatInt(p.Pid, 10))
	form.Set("cart_data[billingcycle]", p.BillingCycle)
	form.Set("cart_data[qty]", strconv.Itoa(p.Qty))
	form.Set("cart_data[currencyid]", strconv.FormatInt(p.CurrencyID, 10))
	if p.Host != "" {
		form.Set("cart_data[host]", p.Host)
	}
	if p.Password != "" {
		form.Set("cart_data[password]", p.Password)
	}
	for k, v := range p.ConfigOptions {
		form.Set("cart_data[configoptions]["+k+"]", v)
	}
	if p.Notes != "" {
		form.Set("notes", p.Notes)
	}
	var raw struct {
		InvoiceID FlexInt   `json:"invoiceid"`
		HostID    []FlexInt `json:"hostid"`
	}
	// 1001 也是成功（0 元单直接购买成功）
	if err := c.postForm(ctx, "/cart/settle", form, []int{200, 1001}, &raw); err != nil {
		return nil, err
	}
	res := &SettleResult{InvoiceID: int64(raw.InvoiceID)}
	for _, h := range raw.HostID {
		res.HostIDs = append(res.HostIDs, int64(h))
	}
	res.PaidFree = len(res.HostIDs) > 0 && res.InvoiceID == 0
	return res, nil
}

// ApplyCreditResult 是余额支付结果。
type ApplyCreditResult struct {
	HostIDs []int64 // 账单关联的主机 ID 列表（qty>1 时多台）
}

// ApplyCredit 用上游账户余额支付账单（POST /apply_credit，use_credit=1&enough=1）。
// 成功：{status:1001, msg:"支付完成", data:{hostid:[…], url}}；支付成功即触发上游
// 自动开通（auto_setup=payment）。余额不足等 → 业务拒绝 {status:400,msg:"余额不足…"}
// 转 *BizError（service 层据此走自动退款补偿，见 ADR-029）。
//
// ⚠ enough=1 语义：余额不足以全额支付时直接拒绝（不做部分支付）——本系统要求
// 全额余额支付，部分支付会把账单留在半付状态，补偿复杂度不可控。
func (c *Client) ApplyCredit(ctx context.Context, invoiceID int64) (*ApplyCreditResult, error) {
	form := url.Values{}
	form.Set("invoiceid", strconv.FormatInt(invoiceID, 10))
	form.Set("use_credit", "1")
	form.Set("enough", "1")
	var raw struct {
		HostID []FlexInt `json:"hostid"`
	}
	if err := c.postForm(ctx, "/apply_credit", form, []int{1001, 200}, &raw); err != nil {
		return nil, err
	}
	res := &ApplyCreditResult{}
	for _, h := range raw.HostID {
		res.HostIDs = append(res.HostIDs, int64(h))
	}
	return res, nil
}

// SetDownstream 把下游信息登记到上游主机（POST /host/setdownstream）。
// 参数形状（源码 HostController::postSetDownStream）：
//   - id = 上游主机 ID；pid = 上游商品 ID（必须与主机的商品一致，否则 406"不是同一商品"）；
//   - downstream_url 必须以 http:// 或 https:// 开头；
//   - downstream_token 必须**恰好 32 位**（上游 strlen==32 硬校验；本系统用
//     downstream_token 设置值的前 32 位派生，见 ADR-026/契约 9.9）；
//   - downstream_id 必须为数字（本地 hosts.id）。
//
// 成功 {status:200,msg:"关联成功"}（无 data 键）。本机部署上游推送不可达属预期，
// 调用方失败只记日志不阻断（任务约定）。
func (c *Client) SetDownstream(ctx context.Context, upstreamHostID, upstreamPid, downstreamID int64, downstreamURL, downstreamToken string) error {
	form := url.Values{}
	form.Set("id", strconv.FormatInt(upstreamHostID, 10))
	form.Set("pid", strconv.FormatInt(upstreamPid, 10))
	form.Set("downstream_url", downstreamURL)
	form.Set("downstream_token", downstreamToken)
	form.Set("downstream_id", strconv.FormatInt(downstreamID, 10))
	return c.postForm(ctx, "/host/setdownstream", form, []int{200}, nil)
}

// HostHeader 是 /host/header 的 data.host_data 关键字段（开通结果轮询用）。
//
// ⚠ NextDueDate 是 unix 秒时间戳（host 表 nextduedate 列，与推送的日期文本不同）；
// Password 上游已解密为明文；AssignedIPs 是数组形态。
type HostHeader struct {
	ID           FlexInt  `json:"id"`
	Domain       string   `json:"domain"`
	DomainStatus string   `json:"domainstatus"` // Pending/Active/Suspended/Cancelled/Fraud/Deleted
	DedicatedIP  string   `json:"dedicatedip"`
	AssignedIPs  []string `json:"assignedips"`
	Username     string   `json:"username"`
	Password     string   `json:"password"`
	Port         FlexInt  `json:"port"` // 上游列可能是数字或字符串，宽松接收
	OS           string   `json:"os"`
	NextDueDate  FlexInt  `json:"nextduedate"` // unix 秒
	BillingCycle string   `json:"billingcycle"`
	ProductName  string   `json:"productname"`
	OrderNum     string   `json:"ordernum"` // 上游订单号
	Status       string   `json:"status"`   // 账单态（Paid/Unpaid），非主机态
}

// GetHostHeader 拉取主机详情（GET /host/header?host_id=N）。
// 主机不存在/不属于本账号 → 406 业务错误（*BizError）。
func (c *Client) GetHostHeader(ctx context.Context, hostID int64) (*HostHeader, error) {
	q := url.Values{}
	q.Set("host_id", strconv.FormatInt(hostID, 10))
	var raw struct {
		HostData *HostHeader `json:"host_data"`
	}
	if err := c.doBody(ctx, "GET", "/host/header", q, "", "", []int{200}, &raw); err != nil {
		return nil, err
	}
	if raw.HostData == nil {
		return nil, fmt.Errorf("上游 /host/header 响应缺少 host_data（host_id=%d）", hostID)
	}
	return raw.HostData, nil
}

// DefaultConfigOptions 从 /cart/get_product_config 响应中提取每个可配置项的默认值
// （镜像上游 Shop::configfilter 的 auto-fill：数量型取 qty_minimum，选择型取第一个
// 非隐藏子项 id——API 响应的 sub 数组已按 sort_order/id 排序）。
// 解析失败/无配置组返回空 map（settle 端上游会再做一次 auto-fill 兜底）。
func DefaultConfigOptions(configJSON json.RawMessage) map[string]string {
	out := map[string]string{}
	var parsed struct {
		ConfigGroups []struct {
			Options []struct {
				ID         FlexInt `json:"id"`
				OptionType int     `json:"option_type"`
				QtyMinimum int     `json:"qty_minimum"`
				Sub        []struct {
					ID     FlexInt `json:"id"`
					Hidden int     `json:"hidden"`
				} `json:"sub"`
			} `json:"options"`
		} `json:"config_groups"`
	}
	if err := json.Unmarshal(configJSON, &parsed); err != nil {
		return out
	}
	for _, g := range parsed.ConfigGroups {
		for _, opt := range g.Options {
			if opt.OptionType == 20 { // 隐藏型：未显式提交则跳过（同上游行为）
				continue
			}
			key := strconv.FormatInt(int64(opt.ID), 10)
			if isQuantityOption(opt.OptionType) {
				out[key] = strconv.Itoa(opt.QtyMinimum)
				continue
			}
			for _, sub := range opt.Sub {
				if sub.Hidden == 0 {
					out[key] = strconv.FormatInt(int64(sub.ID), 10)
					break
				}
			}
		}
	}
	return out
}

// isQuantityOption 判断可配置项是否数量型（源码 judgeQuantity：4/7/9/11/14-19）。
func isQuantityOption(optionType int) bool {
	switch optionType {
	case 4, 7, 9, 11, 14, 15, 16, 17, 18, 19:
		return true
	}
	return false
}

// PushToken32 从完整 downstream_token 派生上游登记用的 32 位推送密钥。
//
// 上游 setdownstream/settle 对 downstream_token 有 strlen==32 硬校验，而本系统
// 生成的是 64 位 hex——派生规则：超过 32 位取前 32 位，否则原样返回。
// host/sync 验签端用同一规则派生（PushTokenVariants），两侧对同一 secret 收敛。
func PushToken32(fullToken string) string {
	t := strings.TrimSpace(fullToken)
	if len(t) > 32 {
		return t[:32]
	}
	return t
}

// PushTokenVariants 返回 host/sync 验签应尝试的 token 变体（派生 32 位优先，
// 完整 token 兜底——兼容管理员在上游手工登记全文的旧形态）。
func PushTokenVariants(fullToken string) []string {
	t := strings.TrimSpace(fullToken)
	if t == "" {
		return nil
	}
	p32 := PushToken32(t)
	if p32 == t {
		return []string{t}
	}
	return []string{p32, t}
}
