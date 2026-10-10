package upstream

import (
	"context"
	"net/http"
	"testing"
)

// 真实上游响应形状回归测试（批 2 P0 缺陷：data 信封未解包）。
//
// fixture 逐字取自 Langford mock 上游（127.0.0.1:18099，形状与真实 lyew.com
// 实测一致）的响应原文，含 is_aff / group 等噪声字段。
// ⚠ 修复 doOnce 的 data 信封解包前，本测试必须失败（红）；修复后必须通过（绿）。
//
// 真实 pay_type 形状（批 2 第二个 P0 修正）：**不含任何价格**，只有付费模式
// 标记与周期时长定义（pay_hour_cycle:"720" 是小时数！）：
//
//	{"pay_type":"recurring","pay_hour_cycle":"720","pay_day_cycle":"30",...}
//
// 成本价真源是 /api/product/prodetail 的 product_pricings（见 prodetail_test.go）。

const realCreditEnvelope = `{"status":200,"msg":"请求成功","data":{"credit":"57664.50","currency":{"id":1,"code":"CNY","prefix":"¥","suffix":"元"}},"is_aff":"1"}`

const realCartAllEnvelope = `{"status":200,"msg":"请求成功","data":{"products":[{"id":1,"name":"一区 标准云","products":[{"id":101,"type":"dcimcloud","name":"入门型 1核1G","description":"&lt;li&gt;CPU:1核心&lt;/li&gt;\n&lt;li&gt;内存:1G&lt;/li&gt;","pay_type":"{\"pay_type\":\"recurring\",\"pay_hour_cycle\":\"720\",\"pay_day_cycle\":\"30\",\"pay_ontrial_status\":0}"},{"id":102,"type":"dcimcloud","name":"进阶型 2核2G","description":"&lt;li&gt;CPU:2核心&lt;/li&gt;","pay_type":"{\"pay_type\":\"recurring\",\"pay_hour_cycle\":\"720\"}"}]},{"id":2,"name":"域名注册","products":[{"id":201,"type":"domain","name":".com 域名","description":"域名注册","pay_type":"{\"pay_type\":\"onetime\"}"}]}],"currency":{"code":"CNY","prefix":"¥","suffix":"元"}},"is_aff":"1"}`

const realHostListEnvelope = `{"group":null,"status":200,"data":{"page":1,"limit":10,"sum":2,"max_page":1,"orderby":"id","sort":"DESC","list":[{"id":9001,"domain":"host-a.example.com","domainstatus":"Active","dedicatedip":"1.2.3.4","nextduedate":"2026-12-01"},{"id":9002,"domain":"host-b.example.com","domainstatus":"Pending","dedicatedip":"1.2.3.5","nextduedate":"2026-11-01"}],"domainstatus":{"Pending":{"name":"待开通","color":"#fca426"},"Active":{"name":"已激活","color":"#3fbf70"},"Suspended":{"name":"已暂停","color":"#e31519"}}},"is_aff":"1"}`

const realProinfoEnvelope = `{"status":200,"msg":"请求成功","data":{"info":[{"id":1,"name":"商品1","stock_control":1,"qty":72,"server_type":""}],"currency":"CNY"}}`

// newRealEnvelopeClient 构造按路径返回真实信封 fixture 的假上游客户端。
func newRealEnvelopeClient(t *testing.T) (*Client, func()) {
	t.Helper()
	f := newFakeUpstream()
	f.bizHandler = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		switch r.URL.Path {
		case "/cart/credit":
			_, _ = w.Write([]byte(realCreditEnvelope))
		case "/cart/all":
			_, _ = w.Write([]byte(realCartAllEnvelope))
		case "/host/list":
			_, _ = w.Write([]byte(realHostListEnvelope))
		case "/api/product/proinfo":
			_, _ = w.Write([]byte(realProinfoEnvelope))
		default:
			_, _ = w.Write([]byte(`{"status":404,"msg":"接口不存在"}`))
		}
	}
	return f.client(), f.close
}

func TestRealEnvelopeGetCredit(t *testing.T) {
	c, closeFn := newRealEnvelopeClient(t)
	defer closeFn()
	credit, err := c.GetCredit(context.Background())
	if err != nil {
		t.Fatalf("GetCredit 失败: %v", err)
	}
	if credit.Credit != "57664.50" {
		t.Errorf("data 信封未解包或解析错误: credit=%q（期望 57664.50）", credit.Credit)
	}
	if credit.Currency.Code != "CNY" || credit.Currency.Prefix != "¥" || credit.Currency.Suffix != "元" {
		t.Errorf("currency 解析错误: %+v", credit.Currency)
	}
}

func TestRealEnvelopeGetProducts(t *testing.T) {
	c, closeFn := newRealEnvelopeClient(t)
	defer closeFn()
	groups, err := c.GetProducts(context.Background())
	if err != nil {
		t.Fatalf("GetProducts 失败: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("data 信封未解包或解析错误: 分组数=%d（期望 2）", len(groups))
	}
	if groups[0].Name != "一区 标准云" || len(groups[0].Products) != 2 {
		t.Fatalf("分组解析错误: %+v", groups[0])
	}
	p := groups[0].Products[0]
	if p.ID != 101 || p.Name != "入门型 1核1G" || p.Type != "dcimcloud" {
		t.Errorf("商品字段解析错误: %+v", p)
	}
	// description：HTML 实体解码 + 换行保留
	if p.Description != "<li>CPU:1核心</li>\n<li>内存:1G</li>" {
		t.Errorf("description 实体解码错误: %q", p.Description)
	}
	// 付费模式从 pay_type 标记解析；周期时长字段绝不能被当价格
	if p.PayMode != "recurring" {
		t.Errorf("PayMode 解析错误: %q", p.PayMode)
	}
	if len(p.Prices) != 0 {
		t.Errorf("cart/all 不含价格，client 层 Prices 必须恒空（真源 prodetail）: %v", p.Prices)
	}
	// onetime 商品的付费模式
	dom := groups[1].Products[0]
	if dom.PayMode != "onetime" {
		t.Errorf("onetime 付费模式解析错误: %q", dom.PayMode)
	}
}

func TestRealEnvelopeGetHosts(t *testing.T) {
	c, closeFn := newRealEnvelopeClient(t)
	defer closeFn()
	list, err := c.GetHosts(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("GetHosts 失败: %v", err)
	}
	if list.Sum != 2 || list.Page != 1 || list.Limit != 10 {
		t.Errorf("分页字段未从 data 解出: page=%d limit=%d sum=%d", list.Page, list.Limit, list.Sum)
	}
	if len(list.List) != 2 {
		t.Fatalf("主机行数错误: %d（期望 2）", len(list.List))
	}
	if list.List[0]["domain"] != "host-a.example.com" || list.List[1]["domainstatus"] != "Pending" {
		t.Errorf("主机字段透传错误: %v", list.List)
	}
	if list.DomainStatus["Active"].Name != "已激活" || list.DomainStatus["Pending"].Color != "#fca426" {
		t.Errorf("domainstatus 字典解析错误: %v", list.DomainStatus)
	}
}

func TestRealEnvelopeGetProductInfo(t *testing.T) {
	c, closeFn := newRealEnvelopeClient(t)
	defer closeFn()
	infos, err := c.GetProductInfo(context.Background(), []int64{1})
	if err != nil {
		t.Fatalf("GetProductInfo 失败: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("data 信封未解包: info 行数=%d（期望 1）", len(infos))
	}
	if infos[0].Qty != 72 || !infos[0].StockControl || infos[0].Name != "商品1" {
		t.Errorf("库存解析错误: %+v", infos[0])
	}
}

// 兼容路径：payload 在顶层（无 data 键）的端点/旧形状仍须解析正确。
func TestTopLevelShapeCompatibility(t *testing.T) {
	f := newFakeUpstream()
	defer f.close()
	f.bizHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":200,"credit":"1.23","currency":{"code":"CNY","prefix":"","suffix":""}}`))
	}
	c := f.client()
	credit, err := c.GetCredit(context.Background())
	if err != nil {
		t.Fatalf("顶层 payload 应兼容解析: %v", err)
	}
	if credit.Credit != "1.23" {
		t.Errorf("顶层 payload 解析失败: %+v", credit)
	}
}

// data:null 容错：不得解包失败（回落整包解析，字段为零值）。
func TestDataNullTolerance(t *testing.T) {
	f := newFakeUpstream()
	defer f.close()
	f.bizHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":200,"msg":"ok","data":null}`))
	}
	c := f.client()
	credit, err := c.GetCredit(context.Background())
	if err != nil {
		t.Fatalf("data:null 应容错: %v", err)
	}
	if credit.Credit != "" {
		t.Errorf("data:null 应得零值: %+v", credit)
	}
}

// 注：pay_type 的价格解析测试已随批 2 第二个 P0 修复移除——真实 pay_type 不含
// 价格（只有付费模式与周期时长），价格解析的回归测试见 prodetail_test.go
// （TestGetProductDetailRealShape / TestNormalizePrice）与 ParsePayMode 用例。
