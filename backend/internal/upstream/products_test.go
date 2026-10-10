package upstream

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"testing"
)

// cartAllFixture 是 /cart/all 的假上游响应，采用 lyew.com 真实形状：
// payload 嵌于 data 键、商品对象**不含任何价格字段**、pay_type 只有付费模式
// 与周期时长定义（pay_hour_cycle 等是时长不是价格！）、description 含 HTML
// 实体转义、is_aff 噪声字段。成本价真源见 prodetail_test.go。
const cartAllFixture = `{
  "status": 200,
  "msg": "请求成功",
  "data": {
    "products": [
      {
        "id": 1,
        "name": "云服务器",
        "products": [
          {
            "id": 101,
            "type": "dcimcloud",
            "name": "入门型云主机",
            "description": "&lt;li&gt;1核1G&lt;/li&gt;&lt;li&gt;50G SSD&lt;/li&gt;",
            "pay_type": "{\"pay_type\":\"recurring\",\"pay_hour_cycle\":\"720\",\"pay_day_cycle\":\"30\",\"pay_ontrial_status\":0,\"pay_ontrial_cycle\":\"12\",\"pay_ontrial_num\":\"1\",\"pay_ontrial_condition\":[\"email\"],\"pay_ontrial_cycle_type\":\"hour\",\"pay_ontrial_num_rule\":\"0\",\"clientscount_rule\":\"0\"}"
          },
          {
            "id": 102,
            "type": "host",
            "name": "无 pay_type 商品",
            "description": "",
            "pay_type": ""
          }
        ]
      }
    ],
    "currency": {"code": "CNY", "prefix": "¥", "suffix": "元"}
  },
  "is_aff": "1"
}`

func TestGetProductsParsing(t *testing.T) {
	f := newFakeUpstream()
	defer f.close()
	f.bizHandler = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cart/all" {
			t.Errorf("意外路径: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(cartAllFixture))
	}
	c := f.client()
	groups, err := c.GetProducts(context.Background())
	if err != nil {
		t.Fatalf("GetProducts 失败: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("分组数应为 1，得到 %d", len(groups))
	}
	g := groups[0]
	if g.ID != 1 || g.Name != "云服务器" {
		t.Errorf("分组解析错误: %+v", g)
	}
	if len(g.Products) != 2 {
		t.Fatalf("商品数应为 2，得到 %d", len(g.Products))
	}
	p := g.Products[0]
	if p.ID != 101 || p.Type != "dcimcloud" || p.Name != "入门型云主机" {
		t.Errorf("商品字段解析错误: %+v", p)
	}
	// HTML 实体必须已解码
	if p.Description != "<li>1核1G</li><li>50G SSD</li>" {
		t.Errorf("description 实体解码错误: %q", p.Description)
	}
	// 付费模式从 pay_type 标记解析
	if p.PayMode != "recurring" {
		t.Errorf("PayMode 解析错误: %q", p.PayMode)
	}
	// cart/all 无价格：Prices 在 client 层必须恒空（成本价真源是 prodetail）
	if len(p.Prices) != 0 {
		t.Errorf("cart/all 不含价格，Prices 必须为空（真源 prodetail）: %v", p.Prices)
	}
	// 空 pay_type → PayMode 空、不报错
	if g.Products[1].PayMode != "" || len(g.Products[1].Prices) != 0 {
		t.Errorf("空 pay_type 容错错误: mode=%q prices=%v", g.Products[1].PayMode, g.Products[1].Prices)
	}
}

// realPayTypeFixture 是 lyew.com 真实 /cart/all 中 pid=1 的 pay_type 原文：
// 只含付费模式与周期时长定义（pay_hour_cycle=720 小时数等），**没有任何价格**。
// 批 2 第二个 P0：修复前 ParsePayType 会把 pay_hour_cycle:"720" 等当价格解析出垃圾。
const realPayTypeFixture = `{"pay_type":"recurring","pay_hour_cycle":"720","pay_day_cycle":"30","pay_ontrial_status":0,"pay_ontrial_cycle":"12","pay_ontrial_num":"1","pay_ontrial_condition":["email"],"pay_ontrial_cycle_type":"hour","pay_ontrial_num_rule":"0","clientscount_rule":"0"}`

func TestGetProductsNoPriceInPayType(t *testing.T) {
	// 行为级回归：真实 cart/all（pay_type 无价格）→ 商品 Prices 必须为空，
	// 绝不能出现 pay_hour_cycle/pay_day_cycle 等周期时长字段被当成价格。
	f := newFakeUpstream()
	defer f.close()
	f.bizHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":200,"msg":"请求成功","data":{"products":[
			{"id":1,"name":"二区 CN2标准性能","products":[
				{"id":1,"type":"dcimcloud","name":"香港二区 CN2 A型","description":"&lt;li&gt;CPU:2核心&lt;/li&gt;","pay_type":` + strconv.Quote(realPayTypeFixture) + `}
			]}
		],"currency":{"code":"CNY","prefix":"¥","suffix":"元"}},"is_aff":"1"}`))
	}
	c := f.client()
	groups, err := c.GetProducts(context.Background())
	if err != nil {
		t.Fatalf("GetProducts 失败: %v", err)
	}
	if len(groups) != 1 || len(groups[0].Products) != 1 {
		t.Fatalf("data 信封解包应正常: %+v", groups)
	}
	p := groups[0].Products[0]
	if len(p.Prices) != 0 {
		t.Errorf("cart/all 的 pay_type 不含价格，Prices 必须为空，得到垃圾解析: %v", p.Prices)
	}
	for bad := range p.Prices {
		t.Errorf("垃圾周期键混入成本价: %q", bad)
	}
}

func TestParsePayMode(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "真实 recurring（lyew.com pid=1 原文）",
			in:   realPayTypeFixture,
			want: "recurring",
		},
		{
			name: "onetime 型",
			in:   `{"pay_type":"onetime","clientscount_rule":"0"}`,
			want: "onetime",
		},
		{name: "空串", in: "", want: ""},
		{name: "非法 JSON 容错", in: "{bad", want: ""},
		{name: "无 pay_type 字段", in: `{"monthly":{"price":"10.00"}}`, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParsePayMode(tc.in); got != tc.want {
				t.Errorf("ParsePayMode = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGetProductInfoQueryAndParsing(t *testing.T) {
	f := newFakeUpstream()
	defer f.close()
	var gotQuery url.Values
	f.bizHandler = func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		// 真实信封形状（data 内含 info；currency 为字符串噪声字段）
		_, _ = w.Write([]byte(`{"status":200,"msg":"请求成功","data":{"info":[
			{"id":101,"name":"A","stock_control":1,"qty":"5","server_type":""},
			{"id":102,"name":"B","stock_control":false,"qty":0}
		],"currency":"CNY"}}`))
	}
	c := f.client()
	list, err := c.GetProductInfo(context.Background(), []int64{101, 102})
	if err != nil {
		t.Fatalf("GetProductInfo 失败: %v", err)
	}
	// 数组参数必须是 pids[] 重复键形式
	pids := gotQuery["pids[]"]
	if len(pids) != 2 || pids[0] != "101" || pids[1] != "102" {
		t.Errorf("pids[] 参数错误: %v（raw=%s）", pids, gotQuery)
	}
	if len(list) != 2 {
		t.Fatalf("结果数应为 2，得到 %d", len(list))
	}
	// 宽松类型转换：1→true、"5"→5
	if !list[0].StockControl || list[0].Qty != 5 {
		t.Errorf("宽松转换错误: %+v", list[0])
	}
	if list[1].StockControl || list[1].Qty != 0 {
		t.Errorf("宽松转换错误: %+v", list[1])
	}
}

func TestGetProductInfoEmptyPids(t *testing.T) {
	c := NewClient(Credentials{Hostname: "http://127.0.0.1:1"})
	list, err := c.GetProductInfo(context.Background(), nil)
	if err != nil || list != nil {
		t.Errorf("空 pids 应直接返回 nil,nil，得到 %v,%v", list, err)
	}
}

func TestGetProductConfig(t *testing.T) {
	f := newFakeUpstream()
	defer f.close()
	f.bizHandler = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pid") != "101" {
			t.Errorf("pid 参数错误: %s", r.URL.RawQuery)
		}
		// 真实信封形状：透传的应是解包后的 data 内容
		_, _ = w.Write([]byte(`{"status":200,"msg":"请求成功","data":{"config":{"os":[{"id":1,"name":"CentOS 7"}]}}}`))
	}
	c := f.client()
	raw, err := c.GetProductConfig(context.Background(), 101)
	if err != nil {
		t.Fatalf("GetProductConfig 失败: %v", err)
	}
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatalf("透传 JSON 非法: %v", err)
	}
	if _, ok := probe["config"]; !ok {
		t.Errorf("透传内容缺 config 字段: %s", raw)
	}
}

func TestGetCredit(t *testing.T) {
	f := newFakeUpstream()
	defer f.close()
	f.bizHandler = func(w http.ResponseWriter, r *http.Request) {
		// 真实信封形状（lyew.com 实测原文同构，含 is_aff 噪声）
		_, _ = w.Write([]byte(`{"status":200,"msg":"请求成功","data":{"credit":"57664.50","currency":{"id":1,"code":"CNY","prefix":"¥","suffix":"元"}},"is_aff":"1"}`))
	}
	c := f.client()
	credit, err := c.GetCredit(context.Background())
	if err != nil {
		t.Fatalf("GetCredit 失败: %v", err)
	}
	if credit.Credit != "57664.50" {
		t.Errorf("余额解析错误: %s", credit.Credit)
	}
	if credit.Currency.Code != "CNY" || credit.Currency.Prefix != "¥" || credit.Currency.Suffix != "元" {
		t.Errorf("货币解析错误: %+v", credit.Currency)
	}
}

func TestGetHosts(t *testing.T) {
	f := newFakeUpstream()
	defer f.close()
	var gotQuery url.Values
	f.bizHandler = func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		// 真实信封形状（data 内含全部分页字段；group:null 噪声；color 为十六进制）
		_, _ = w.Write([]byte(`{"group":null,"status":200,"data":{"page":1,"limit":20,"sum":1,"max_page":1,"orderby":"id","sort":"DESC","list":[
			{"id":9001,"domain":"demo.example.com","domainstatus":"Active","dedicatedip":"1.2.3.4","nextduedate":"2026-12-01"}
		],"domainstatus":{"Pending":{"name":"待激活","color":"#fca426"},"Active":{"name":"已激活","color":"#3fbf70"}}},"is_aff":"1"}`))
	}
	c := f.client()
	list, err := c.GetHosts(context.Background(), 1, 20)
	if err != nil {
		t.Fatalf("GetHosts 失败: %v", err)
	}
	if gotQuery.Get("page") != "1" || gotQuery.Get("limit") != "20" {
		t.Errorf("分页参数错误: %v", gotQuery)
	}
	if list.Sum != 1 || len(list.List) != 1 {
		t.Fatalf("列表解析错误: %+v", list)
	}
	if list.List[0]["domain"] != "demo.example.com" {
		t.Errorf("主机字段透传错误: %v", list.List[0])
	}
	if list.DomainStatus["Active"].Name != "已激活" {
		t.Errorf("状态字典解析错误: %v", list.DomainStatus)
	}
}

func TestGetHostsParamClamp(t *testing.T) {
	f := newFakeUpstream()
	defer f.close()
	var gotQuery url.Values
	f.bizHandler = func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		_, _ = w.Write([]byte(`{"status":200,"data":{"page":1,"limit":20,"sum":0,"list":[]}}`))
	}
	c := f.client()
	if _, err := c.GetHosts(context.Background(), 0, 0); err != nil {
		t.Fatalf("GetHosts 失败: %v", err)
	}
	if gotQuery.Get("page") != "1" || gotQuery.Get("limit") != "20" {
		t.Errorf("非法分页参数应回落默认: %v", gotQuery)
	}
}

func TestAnyConversions(t *testing.T) {
	if !anyToBool(1) || !anyToBool(true) || !anyToBool("1") || !anyToBool("true") || anyToBool(0) || anyToBool("0") || anyToBool(nil) {
		t.Error("anyToBool 转换错误")
	}
	if anyToInt("5") != 5 || anyToInt(5.0) != 5 || anyToInt(json.Number("7")) != 7 || anyToInt(nil) != 0 || anyToInt("bad") != 0 {
		t.Error("anyToInt 转换错误")
	}
}
