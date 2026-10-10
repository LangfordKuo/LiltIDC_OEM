package upstream

import (
	"context"
	"net/http"
	"net/url"
	"testing"
)

// realProdetailFixture 取自 lyew.com 真实 GET /api/product/prodetail?pids[]=1&pids[]=14&pids[]=99999
// 响应（data 信封内，字段有精简但价格/setupfee/容错形态逐字保留）：
//   - pid=1：recurring 型，monthly=20.00 quarterly=60.00 semiannually=120.00 annually=200.00，
//     其余周期 -1.00（停售），17 个 setupfee 安装费键，code=CNY；
//   - pid=14：onetime 型，onetime=300.00，monthly=-1.00；
//   - pid=99999：product_pricings 为 null（必须容错）。
const realProdetailFixture = `{"status":200,"msg":"请求成功","data":{"detail":{
"1":{"id":1,"name":"香港二区 CN2 A型","pay_type":"{\"pay_type\":\"recurring\",\"pay_hour_cycle\":\"720\",\"pay_day_cycle\":\"30\"}","stock_control":1,"qty":72,
 "product_pricings":[{"id":1,"type":"product","currency":1,"relid":1,
  "osetupfee":"0.00","hsetupfee":"0.00","dsetupfee":"0.00","ontrialfee":"0.00","msetupfee":"0.00","qsetupfee":"0.00","ssetupfee":"0.00","asetupfee":"0.00","bsetupfee":"0.00","tsetupfee":"0.00","foursetupfee":"0.00","fivesetupfee":"0.00","sixsetupfee":"0.00","sevensetupfee":"0.00","eightsetupfee":"0.00","ninesetupfee":"0.00","tensetupfee":"0.00",
  "onetime":"-1.00","hour":"-1.00","day":"-1.00","ontrial":"-1.00",
  "monthly":"20.00","quarterly":"60.00","semiannually":"120.00","annually":"200.00",
  "biennially":"-1.00","triennially":"-1.00","fourly":"-1.00","fively":"-1.00","sixly":"-1.00","sevenly":"-1.00","eightly":"-1.00","ninely":"-1.00","tenly":"-1.00",
  "code":"CNY"}]},
"14":{"id":14,"name":"一次性测试商品","stock_control":0,"qty":0,
 "product_pricings":[{"onetime":"300.00","monthly":"-1.00","code":"CNY"}]},
"99999":{"id":99999,"name":"不存在的商品","product_pricings":null}
}},"is_aff":"1"}`

func TestGetProductDetailRealShape(t *testing.T) {
	f := newFakeUpstream()
	defer f.close()
	var gotQuery url.Values
	var gotRawQuery string
	f.bizHandler = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/product/prodetail" {
			t.Errorf("意外路径: %s", r.URL.Path)
		}
		gotQuery = r.URL.Query()
		gotRawQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(realProdetailFixture))
	}
	c := f.client()

	// 88888 不在 fixture detail 中 → 模拟实测的「键整个缺失」容错
	details, err := c.GetProductDetail(context.Background(), []int64{1, 14, 99999, 88888})
	if err != nil {
		t.Fatalf("GetProductDetail 失败: %v", err)
	}

	// 查询参数：pids[] 重复键、未编码原样（PHP 与简单实现双兼容）
	if pids := gotQuery["pids[]"]; len(pids) != 4 || pids[0] != "1" || pids[3] != "88888" {
		t.Errorf("pids[] 参数错误: %v (raw=%s)", pids, gotRawQuery)
	}

	// ---- pid=1：recurring 型 ----
	d1, ok := details[1]
	if !ok {
		t.Fatal("detail[1] 缺失")
	}
	if d1.Name != "香港二区 CN2 A型" || d1.PayMode != "recurring" {
		t.Errorf("detail[1] 基本字段错误: name=%q pay_mode=%q", d1.Name, d1.PayMode)
	}
	if d1.CurrencyCode != "CNY" || !d1.StockControl || d1.Qty != 72 {
		t.Errorf("detail[1] 货币/库存错误: %+v", d1)
	}
	// 成本价：四个在售周期，键名与上游一致（semiannually 无下划线）
	want := map[string]string{"monthly": "20.00", "quarterly": "60.00", "semiannually": "120.00", "annually": "200.00"}
	if len(d1.CostPrices) != len(want) {
		t.Fatalf("detail[1] 成本价周期数=%d（期望 %d）: %v", len(d1.CostPrices), len(want), d1.CostPrices)
	}
	for k, v := range want {
		if d1.CostPrices[k] != v {
			t.Errorf("detail[1] 成本价 %s: got=%q want=%q", k, d1.CostPrices[k], v)
		}
	}
	// -1.00 停售周期必须被过滤
	for _, stopped := range []string{"onetime", "hour", "day", "ontrial", "biennially", "triennially", "tenly"} {
		if _, bad := d1.CostPrices[stopped]; bad {
			t.Errorf("-1.00 停售周期 %s 未被过滤: %v", stopped, d1.CostPrices)
		}
	}
	// 非价格键不得混入
	for _, junk := range []string{"code", "relid", "id", "type", "currency", "pay_hour_cycle"} {
		if _, bad := d1.CostPrices[junk]; bad {
			t.Errorf("非价格键 %s 混入成本价: %v", junk, d1.CostPrices)
		}
	}
	// setupfee 分离解析（本批不导入，结构预留）
	if len(d1.SetupFees) != 17 || d1.SetupFees["msetupfee"] != "0.00" || d1.SetupFees["tensetupfee"] != "0.00" {
		t.Errorf("setupfee 解析错误: %d 项 %v", len(d1.SetupFees), d1.SetupFees)
	}

	// ---- pid=14：onetime 型 ----
	d14, ok := details[14]
	if !ok {
		t.Fatal("detail[14] 缺失")
	}
	if len(d14.CostPrices) != 1 || d14.CostPrices["onetime"] != "300.00" {
		t.Errorf("onetime 型成本价错误: %v", d14.CostPrices)
	}

	// ---- pid=99999：product_pricings=null 容错 ----
	d99, ok := details[99999]
	if !ok {
		t.Fatal("detail[99999]（null pricings）应保留条目而非崩溃")
	}
	if len(d99.CostPrices) != 0 {
		t.Errorf("null pricings 应得空成本价: %v", d99.CostPrices)
	}

	// ---- pid=88888：detail 键整个缺失容错 ----
	if _, exists := details[88888]; exists {
		t.Error("detail 中不存在的 pid 不应出现在结果中")
	}
}

func TestGetProductDetailEmptyPids(t *testing.T) {
	c := NewClient(Credentials{Hostname: "http://127.0.0.1:1"})
	details, err := c.GetProductDetail(context.Background(), nil)
	if err != nil || details == nil || len(details) != 0 {
		t.Errorf("空 pids 应返回空 map, nil error，得到 %v, %v", details, err)
	}
}

func TestNormalizePrice(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{"20.00", "20.00"},
		{"-1.00", ""},    // 停售过滤
		{"-1", ""},       // 停售过滤（无小数形态）
		{"0.00", "0.00"}, // 免费保留
		{"abc", ""},      // 非数字过滤
		{"", ""},
		{float64(20), "20"},
		{nil, ""},
		{true, ""}, // 非价格类型过滤
	}
	for _, tc := range cases {
		if got := normalizePrice(tc.in); got != tc.want {
			t.Errorf("normalizePrice(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
