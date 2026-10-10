package upstream

import (
	"context"
	"net/http"
	"testing"
)

// 上游分页/ID 字段 int/string 类型漂移回归测试（批 2 小修）。
//
// 真上游 lyew.com 实测：GET /host/list 带 ?page=1&limit=10 时
// data.page="1"、data.limit="10"（字符串，PHP query 参数原样回显），
// 不带参数时 data.page=1（int）；sum/max_page 恒为 int。
// 修复前 HostList.Page/Limit 声明为 int → string 形态必然解析失败（502）。
// 防御性同类字段：cart/all 的分组与商品 id、proinfo/prodetail 的商品 id。

// ---- hosts：三种形态断言结果一致 ----

func TestGetHostsPaginationTypeDrift(t *testing.T) {
	cases := []struct {
		name                string
		fixture             string
		wantPage, wantLimit int
		wantRows            int
	}{
		{
			name:     "int 形态（不带参数调用的真实回显）",
			fixture:  `{"status":200,"data":{"page":1,"limit":10,"sum":2,"max_page":1,"list":[{"id":9001,"domain":"a.example.com","domainstatus":"Active"}],"domainstatus":{"Active":{"name":"已激活","color":"#3fbf70"}}}}`,
			wantPage: 1, wantLimit: 10, wantRows: 1,
		},
		{
			name:     "string 形态（带 query 参数的真实回显，实测原文同形）",
			fixture:  `{"status":200,"data":{"page":"1","limit":"10","sum":2,"max_page":1,"list":[{"id":9001,"domain":"a.example.com","domainstatus":"Active"}],"domainstatus":{"Active":{"name":"已激活","color":"#3fbf70"}}}}`,
			wantPage: 1, wantLimit: 10, wantRows: 1,
		},
		{
			name:     "混合形态（page/limit 字符串 + sum 数字 + 双 list 行）",
			fixture:  `{"status":200,"data":{"page":"2","limit":"20","sum":2,"list":[{"id":9001,"domain":"a.example.com"},{"id":9002,"domain":"b.example.com"}],"domainstatus":{}}}`,
			wantPage: 2, wantLimit: 20, wantRows: 2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeUpstream()
			defer f.close()
			fixture := tc.fixture
			f.bizHandler = func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(fixture))
			}
			c := f.client()
			list, err := c.GetHosts(context.Background(), 1, 10)
			if err != nil {
				t.Fatalf("GetHosts 解析失败（类型漂移未容错）: %v", err)
			}
			wantPage, wantLimit, wantRows := tc.wantPage, tc.wantLimit, tc.wantRows
			if int(list.Page) != wantPage || int(list.Limit) != wantLimit || int(list.Sum) != 2 || len(list.List) != wantRows {
				t.Errorf("解析错误: page=%v limit=%v sum=%v rows=%d（期望 %d/%d/%d 行）",
					list.Page, list.Limit, list.Sum, len(list.List), wantPage, wantLimit, wantRows)
			}
			if len(list.DomainStatus) > 0 && list.DomainStatus["Active"].Name != "已激活" {
				t.Errorf("domainstatus 解析错误: %v", list.DomainStatus)
			}
		})
	}
}

// ---- cart/all：分组与商品 id 的 string 形态（防御性同类宽松化） ----

func TestGetProductsStringIDs(t *testing.T) {
	f := newFakeUpstream()
	defer f.close()
	f.bizHandler = func(w http.ResponseWriter, r *http.Request) {
		// id 全部为字符串形态（PHP stringify 极端场景）
		_, _ = w.Write([]byte(`{"status":200,"data":{"products":[
			{"id":"1","name":"分组A","products":[
				{"id":"101","type":"dcimcloud","name":"商品A","description":"","pay_type":"{\"pay_type\":\"recurring\"}"}
			]}
		]}}`))
	}
	c := f.client()
	groups, err := c.GetProducts(context.Background())
	if err != nil {
		t.Fatalf("GetProducts 应容忍 string id: %v", err)
	}
	if len(groups) != 1 || groups[0].ID != 1 {
		t.Fatalf("分组 id 解析错误: %+v", groups)
	}
	if len(groups[0].Products) != 1 || groups[0].Products[0].ID != 101 {
		t.Fatalf("商品 id 解析错误: %+v", groups[0].Products)
	}
}

// ---- proinfo：商品 id string 形态（qty/stock_control 此前已 any 宽松化） ----

func TestGetProductInfoStringID(t *testing.T) {
	f := newFakeUpstream()
	defer f.close()
	f.bizHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":200,"data":{"info":[{"id":"101","name":"A","stock_control":"1","qty":"7"}]}}`))
	}
	c := f.client()
	infos, err := c.GetProductInfo(context.Background(), []int64{101})
	if err != nil {
		t.Fatalf("GetProductInfo 应容忍 string id: %v", err)
	}
	if len(infos) != 1 || infos[0].ID != 101 || infos[0].Qty != 7 || !infos[0].StockControl {
		t.Fatalf("解析错误: %+v", infos)
	}
}

// ---- prodetail：detail 项内 id string 形态 ----

func TestGetProductDetailStringID(t *testing.T) {
	f := newFakeUpstream()
	defer f.close()
	f.bizHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":200,"data":{"detail":{
			"101":{"id":"101","name":"A","stock_control":"1","qty":"7","product_pricings":[{"monthly":"20.00","code":"CNY"}]}
		}}}`))
	}
	c := f.client()
	details, err := c.GetProductDetail(context.Background(), []int64{101})
	if err != nil {
		t.Fatalf("GetProductDetail 应容忍 string id: %v", err)
	}
	d, ok := details[101]
	if !ok {
		t.Fatal("detail[101] 缺失")
	}
	if d.ID != 101 || d.Qty != 7 || !d.StockControl || d.CostPrices["monthly"] != "20.00" {
		t.Fatalf("解析错误: %+v", d)
	}
}
