package upstream

// 批 3-β 上游写端点单测：fixture 逐字对齐 ZJMF-CBAP 源码响应形状
//（CartController::settle / PayController::applyCredit / HostController::getHeader、
// postSetDownStream）与 lyew.com 真实试探结果（见交付报告试探记录）。

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newCartTestServer 构造带上游登录的 mock（业务 handler 处理其余路径）。
func newCartTestServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/zjmf_api_login" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"jwt":"test-jwt","status":200,"msg":"鉴权成功","is_aff":"1"}`)
			return
		}
		handler(w, r)
	}))
}

func newCartTestClient(srv *httptest.Server) *Client {
	return NewClient(Credentials{Hostname: srv.URL, Username: "u", APIKey: "k"})
}

func TestAddToShopFormEncoding(t *testing.T) {
	var gotContentType, gotBody string
	srv := newCartTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cart/add_to_shop" {
			t.Errorf("意外路径 %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("应为 POST，得到 %s", r.Method)
		}
		gotContentType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":200,"msg":"添加成功","is_aff":"1"}`)
	})
	defer srv.Close()
	c := newCartTestClient(srv)
	err := c.AddToShop(context.Background(), AddToShopParams{
		Pid: 1, BillingCycle: "monthly", Qty: 2, CurrencyID: 1,
		Host: "HK-TEST01", Password: "Lilt2026abc",
		ConfigOption: map[string]string{"3": "7"},
	})
	if err != nil {
		t.Fatalf("AddToShop 失败: %v", err)
	}
	if !strings.Contains(gotContentType, "application/x-www-form-urlencoded") {
		t.Errorf("Content-Type 应为 form-urlencoded，得到 %q", gotContentType)
	}
	// 魔方 commonCurl 用 http_build_query（form 编码），关键字段必须在体中
	for _, want := range []string{"pid=1", "billingcycle=monthly", "qty=2",
		"currencyid=1", "checkout=0", "serverid=0", "host=HK-TEST01"} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("请求体缺少 %q：%s", want, gotBody)
		}
	}
	// 嵌套 configoption[3]=7（url 编码后 %5B3%5D）
	if !strings.Contains(gotBody, "configoption%5B3%5D=7") {
		t.Errorf("configoption 嵌套键编码错误：%s", gotBody)
	}
}

func TestSettlePaidInvoice(t *testing.T) {
	var gotBody string
	srv := newCartTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		// 真实形状（CartController::settle total>0 分支）：invoiceid 可能是字符串（PHP stringify）
		_, _ = io.WriteString(w, `{"status":200,"msg":"结算成功","data":{"invoiceid":"321"},"is_aff":"1"}`)
	})
	defer srv.Close()
	c := newCartTestClient(srv)
	res, err := c.Settle(context.Background(), SettleParams{
		Pid: 1, BillingCycle: "monthly", Qty: 1, CurrencyID: 1,
		Host: "HK-TEST01", Password: "Lilt2026abc",
		ConfigOptions: map[string]string{"1": "1"},
		Notes:         "LiltIDC_OEM LIO202610100001",
	})
	if err != nil {
		t.Fatalf("Settle 失败: %v", err)
	}
	// FlexInt 复用：字符串 "321" 必须解析为数字 321（PHP stringify 漂移防御）
	if res.InvoiceID != 321 {
		t.Errorf("InvoiceID = %d，期望 321（字符串形态 FlexInt 解析）", res.InvoiceID)
	}
	if res.PaidFree || len(res.HostIDs) != 0 {
		t.Errorf("付费单不应带 hostid/PaidFree: %+v", res)
	}
	for _, want := range []string{"cart_data%5Bpid%5D=1", "cart_data%5Bbillingcycle%5D=monthly",
		"cart_data%5Bqty%5D=1", "cart_data%5Bconfigoptions%5D%5B1%5D=1", "notes=LiltIDC_OEM"} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("settle 请求体缺少 %q：%s", want, gotBody)
		}
	}
	// 硬性约束：settle 不得携带 downstream_* 参数（token 幂等回滚陷阱，ADR-026）
	if strings.Contains(gotBody, "downstream") {
		t.Errorf("settle 请求体不应携带 downstream_* 参数：%s", gotBody)
	}
}

func TestSettleFreeOrder(t *testing.T) {
	srv := newCartTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// 0 元单：status=1001 直接购买成功并返回主机 ID（字符串数组形态）
		_, _ = io.WriteString(w, `{"status":1001,"msg":"购买成功","data":{"hostid":["45","46"],"url":"servicedetail?id=45"},"is_aff":"1"}`)
	})
	defer srv.Close()
	c := newCartTestClient(srv)
	res, err := c.Settle(context.Background(), SettleParams{Pid: 2, BillingCycle: "onetime", Qty: 2, CurrencyID: 1})
	if err != nil {
		t.Fatalf("0 元单 settle 失败（1001 应视为成功）: %v", err)
	}
	if !res.PaidFree || res.InvoiceID != 0 || len(res.HostIDs) != 2 || res.HostIDs[0] != 45 {
		t.Errorf("0 元单结果错误: %+v", res)
	}
}

func TestSettleBizReject(t *testing.T) {
	srv := newCartTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":406,"msg":"美国一区 Cera 传家宝库存不足","is_aff":"1"}`)
	})
	defer srv.Close()
	c := newCartTestClient(srv)
	_, err := c.Settle(context.Background(), SettleParams{Pid: 21, BillingCycle: "monthly", Qty: 1, CurrencyID: 1})
	var biz *BizError
	if !errors.As(err, &biz) {
		t.Fatalf("业务拒绝应返回 *BizError，得到 %v", err)
	}
	if biz.Status != 406 || !strings.Contains(biz.Msg, "库存不足") || biz.Path != "/cart/settle" {
		t.Errorf("BizError 内容错误: %+v", biz)
	}
}

func TestApplyCreditSuccessAndReject(t *testing.T) {
	var gotBody string
	srv := newCartTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(gotBody, "invoiceid=999") {
			// 余额不足（真实 msg 形状：PayController::applyCredit）
			_, _ = io.WriteString(w, `{"status":400,"msg":"余额不足：账单需20.00，现余额3.00,上游账单#999完成支付后即可开通"}`)
			return
		}
		// 支付完成：status=1001 + data.hostid
		_, _ = io.WriteString(w, `{"status":1001,"msg":"支付完成","data":{"hostid":[7],"url":"servicedetail?id=7"},"is_aff":"1"}`)
	})
	defer srv.Close()
	c := newCartTestClient(srv)

	res, err := c.ApplyCredit(context.Background(), 123)
	if err != nil {
		t.Fatalf("ApplyCredit 失败（1001 应视为成功）: %v", err)
	}
	if len(res.HostIDs) != 1 || res.HostIDs[0] != 7 {
		t.Errorf("HostIDs 错误: %+v", res.HostIDs)
	}
	if !strings.Contains(gotBody, "use_credit=1") || !strings.Contains(gotBody, "enough=1") {
		t.Errorf("apply_credit 必须 use_credit=1&enough=1（全额余额支付）：%s", gotBody)
	}

	_, err = c.ApplyCredit(context.Background(), 999)
	var biz *BizError
	if !errors.As(err, &biz) || biz.Status != 400 || !strings.Contains(biz.Msg, "余额不足") {
		t.Fatalf("余额不足应返回 *BizError(400)，得到 %v", err)
	}
}

func TestSetDownstream(t *testing.T) {
	var gotBody string
	srv := newCartTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		// 成功响应无 data 键（postSetDownStream 只回 status/msg）
		_, _ = io.WriteString(w, `{"status":200,"msg":"关联成功"}`)
	})
	defer srv.Close()
	c := newCartTestClient(srv)
	err := c.SetDownstream(context.Background(), 7, 1, 42,
		"https://idc.example.com", "0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("SetDownstream 失败: %v", err)
	}
	for _, want := range []string{"id=7", "pid=1", "downstream_id=42",
		"downstream_token=0123456789abcdef0123456789abcdef"} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("请求体缺少 %q：%s", want, gotBody)
		}
	}
	if !strings.Contains(gotBody, "downstream_url=https%3A%2F%2Fidc.example.com") {
		t.Errorf("downstream_url 编码错误：%s", gotBody)
	}
}

func TestGetHostHeader(t *testing.T) {
	srv := newCartTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("host_id") != "7" {
			t.Errorf("host_id 参数错误: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		// 真实形状（getHeader）：数字字段可能字符串化（id/port/nextduedate）——FlexInt 防御位点
		_, _ = io.WriteString(w, `{"status":200,"msg":"请求成功","data":{"host_data":{"id":"7","orderid":"3","domain":"HK-TEST01","domainstatus":"Active","dedicatedip":"1.2.3.4","assignedips":["1.2.3.4"],"username":"root","password":"Lilt2026abc","port":"22","os":"Ubuntu 22.04","nextduedate":"1793500000","billingcycle":"monthly","productname":"美国一区 Cera 传家宝","ordernum":"UP2026101001","status":"Paid"},"module_button":{"control":[],"console":[]}},"is_aff":"1"}`)
	})
	defer srv.Close()
	c := newCartTestClient(srv)
	h, err := c.GetHostHeader(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetHostHeader 失败: %v", err)
	}
	if int64(h.ID) != 7 || h.DomainStatus != "Active" || h.DedicatedIP != "1.2.3.4" {
		t.Errorf("host_data 解析错误: %+v", h)
	}
	if int64(h.NextDueDate) != 1793500000 || int(h.Port) != 22 {
		t.Errorf("FlexInt 字段解析错误: nextduedate=%d port=%d", h.NextDueDate, h.Port)
	}
	if h.Username != "root" || h.Password != "Lilt2026abc" || h.OrderNum != "UP2026101001" {
		t.Errorf("开通信息字段错误: %+v", h)
	}
}

func TestGetHostHeaderNotFound(t *testing.T) {
	srv := newCartTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":406,"msg":"产品不存在","is_aff":"1"}`)
	})
	defer srv.Close()
	c := newCartTestClient(srv)
	_, err := c.GetHostHeader(context.Background(), 404)
	var biz *BizError
	if !errors.As(err, &biz) || biz.Status != 406 {
		t.Fatalf("406 应返回 *BizError，得到 %v", err)
	}
}

func TestWriteEndpointReauthOnce(t *testing.T) {
	// 写端点同样享有 405 重登重试一次的保护（do → doBody 重构后语义不变）
	logins, settles := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/zjmf_api_login":
			logins++
			_, _ = io.WriteString(w, `{"jwt":"jwt-`+string(rune('0'+logins))+`","status":200}`)
		case "/cart/settle":
			settles++
			if settles == 1 {
				_, _ = io.WriteString(w, `{"status":405,"msg":"请登陆后再试"}`)
				return
			}
			_, _ = io.WriteString(w, `{"status":200,"msg":"ok","data":{"invoiceid":5}}`)
		}
	}))
	defer srv.Close()
	c := newCartTestClient(srv)
	res, err := c.Settle(context.Background(), SettleParams{Pid: 1, BillingCycle: "monthly", Qty: 1, CurrencyID: 1})
	if err != nil || res.InvoiceID != 5 {
		t.Fatalf("405 后应重登重试成功: res=%+v err=%v", res, err)
	}
	if logins != 2 || settles != 2 {
		t.Errorf("应恰好 2 次登录 + 2 次 settle，得到 logins=%d settles=%d", logins, settles)
	}
}

func TestDefaultConfigOptions(t *testing.T) {
	// 形状取自 get_product_config 实测（config_groups[].options[]，sub 已按排序返回）
	cfg := json.RawMessage(`{
		"config_groups":[
			{"id":1,"options":[
				{"id":1,"option_type":12,"qty_minimum":0,"sub":[{"id":1,"hidden":0},{"id":2,"hidden":0}]},
				{"id":5,"option_type":4,"qty_minimum":2,"sub":[]},
				{"id":9,"option_type":20,"qty_minimum":0,"sub":[{"id":3,"hidden":0}]},
				{"id":11,"option_type":1,"qty_minimum":0,"sub":[{"id":7,"hidden":1},{"id":8,"hidden":0}]}
			]}
		]}`)
	got := DefaultConfigOptions(cfg)
	want := map[string]string{"1": "1", "5": "2", "11": "8"} // type20 跳过；隐藏 sub 跳过
	if len(got) != len(want) {
		t.Fatalf("默认配置项数量错误: %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("config %s = %q，期望 %q", k, got[k], v)
		}
	}
	// 非法 JSON / 空输入容错为空 map
	if len(DefaultConfigOptions(json.RawMessage(`not-json`))) != 0 {
		t.Error("非法 JSON 应容错为空 map")
	}
	if len(DefaultConfigOptions(nil)) != 0 {
		t.Error("nil 应容错为空 map")
	}
}

func TestPushTokenVariants(t *testing.T) {
	full := "0b8b187724fae820eb7f9938862807bde7894715a1b2c3d4e5f60718293a4b5c" // 64 hex
	p32 := PushToken32(full)
	if len(p32) != 32 || p32 != full[:32] {
		t.Errorf("派生 32 位错误: %q", p32)
	}
	short := "abc123"
	if PushToken32(short) != short {
		t.Error("≤32 位应原样返回")
	}
	variants := PushTokenVariants(full)
	if len(variants) != 2 || variants[0] != p32 || variants[1] != full {
		t.Errorf("验签变体顺序错误（派生 32 位优先，完整兜底）: %v", variants)
	}
	if v := PushTokenVariants(short); len(v) != 1 || v[0] != short {
		t.Errorf("短 token 只有一个变体: %v", v)
	}
	if v := PushTokenVariants(""); v != nil {
		t.Errorf("空 token 无变体: %v", v)
	}
}
