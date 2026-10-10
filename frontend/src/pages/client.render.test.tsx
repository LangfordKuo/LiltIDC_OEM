// 会员端页面渲染冒烟测试（批 3-β）：
// 商城列表 / 商品详情（重点：description HTML 的 XSS sanitize 与金额乘法）/
// 注册 / 会员登录 / 我的主机 / 余额 / 订单列表 / 订单详情。
// 未翻译 key 扫描范式与 pages.render.test.tsx 一致（ADR-014 硬门禁）。
import type { ReactNode } from "react"
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest"
import { render, screen } from "@testing-library/react"
import { MemoryRouter, Route, Routes } from "react-router-dom"

import i18n, { i18nReady } from "@/i18n"
import zhCN from "@/i18n/locales/zh-CN.json"
import BalancePage from "@/pages/client/balance"
import ClientLoginPage from "@/pages/client/client-login"
import ConsolePage from "@/pages/client/console"
import OrderDetailPage from "@/pages/client/order-detail"
import OrdersPage from "@/pages/client/orders"
import StoreProductPage from "@/pages/client/product"
import RegisterPage from "@/pages/client/register"
import StorePage from "@/pages/client/store"

// 未翻译 key 的形态：nav.dashboard / settings.tabSms 这类「小写段.字母段」
const RAW_KEY_PATTERN = /\b[a-z]+\.[a-zA-Z]+\b/g
const RAW_KEY_ALLOWLIST = ["smsbao.com", "lyew.com"]

function expectNoRawKeys(container: HTMLElement) {
  const clone = container.cloneNode(true) as HTMLElement
  for (const el of clone.querySelectorAll("script,style")) {
    el.remove()
  }
  const text = clone.textContent ?? ""
  const matches = text.match(RAW_KEY_PATTERN) ?? []
  const suspicious = matches.filter((m) => !RAW_KEY_ALLOWLIST.includes(m))
  expect(suspicious, `页面文本出现未翻译的 i18n key: ${suspicious.join(", ")}`).toEqual([])
}

/** 构造统一响应体 JSON Response。 */
function jsonResponse(data: unknown, status = 200) {
  const body =
    status === 200
      ? { status: 200, msg: "ok", data }
      : { status, code: "not_found", msg: "not found" }
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

// ---- fixture 数据（形状与后端视图逐字段对齐） ----

const storeFixture = {
  groups: [
    {
      id: 1,
      name: "二区 CN2标准性能",
      products: [
        {
          id: 1,
          name: "香港二区 CN2 A型",
          description: "<li>CPU:2核心</li><li>内存:1G</li>",
          type: "dcimcloud",
          group_id: 1,
          group_name: "二区 CN2标准性能",
          sale_prices: { monthly: "20.00", quarterly: "60.00", annually: "200.00" },
          price_from: "20.00",
          stock_qty: 72,
          stock_control: true,
          in_stock: true,
        },
      ],
    },
  ],
}

const detailFixture = {
  id: 1,
  name: "香港二区 CN2 A型",
  // XSS 攻击样本：script / 事件属性 / javascript: URL 必须全部被 sanitize 掉
  description:
    '<p>好机器</p><script>alert("xss")</script><img src="x" onerror="alert(1)"><a href="javascript:alert(2)">点我</a><ul><li>2核</li></ul>',
  type: "dcimcloud",
  group_id: 1,
  group_name: "二区 CN2标准性能",
  sale_prices: { monthly: "19.99", annually: "199.00" },
  price_from: "19.99",
  stock_qty: 72,
  stock_control: true,
  in_stock: true,
  cycles: ["monthly", "annually"],
}

const orderFixture = {
  id: 1,
  order_no: "LIO202610100001",
  client_id: 1,
  status: "Paid",
  status_text: "开通中",
  amount: "39.98",
  balance_used: "39.98",
  pay_method: "balance",
  pay_time: "2026-10-10 12:00:00",
  cancel_time: "",
  remark: "",
  created_at: "2026-10-10 11:59:00",
  items: [
    {
      id: 1,
      product_id: 1,
      product_name: "香港二区 CN2 A型",
      upstream_pid: 1,
      cycle: "monthly",
      cycle_text: "月付",
      qty: 2,
      unit_price: "19.99",
      amount: "39.98",
      upstream_order_id: 321,
      upstream_host_id: 456,
      provision_status: "active",
      provision_status_text: "已开通",
      provision_info: {
        hostname: "HK-ABCD1234",
        ip: "203.0.113.7",
        username: "root",
        password: "Abc12345def",
        next_due_date: "2026-11-10 12:00:00",
        upstream_order_num: "UP2026101001",
      },
    },
  ],
}

beforeAll(async () => {
  await i18nReady
  await i18n.changeLanguage("zh-CN")
})

beforeEach(() => {
  localStorage.setItem("liltidc_client_token", "test-client-token")
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.includes("/api/store/products/1")) return jsonResponse(detailFixture)
      if (url.includes("/api/store/products")) return jsonResponse(storeFixture)
      if (url.includes("/api/client/hosts"))
        return jsonResponse({
          list: [
            {
              id: 1,
              upstream_host_id: 456,
              product_id: 1,
              product_name: "香港二区 CN2 A型",
              domain: "HK-ABCD1234",
              domain_status: "Active",
              dedicated_ip: "203.0.113.7",
              assigned_ips: "203.0.113.7",
              username: "root",
              next_due_date: "2026-11-10 12:00:00",
              synced_at: "2026-10-10 12:03:00",
            },
          ],
          total: 1,
        })
      if (url.includes("/api/client/balance"))
        return jsonResponse({
          credit: "100.00",
          list: [
            {
              id: 1,
              type: "recharge_admin",
              type_text: "人工充值",
              amount: "120.00",
              balance_after: "120.00",
              related_type: "",
              related_id: 0,
              remark: "银行转账",
              admin_id: 1,
              created_at: "2026-10-10 10:00:00",
            },
            {
              id: 2,
              type: "consume",
              type_text: "消费",
              amount: "-20.00",
              balance_after: "100.00",
              related_type: "order",
              related_id: 1,
              remark: "订单支付 LIO202610100001",
              admin_id: null,
              created_at: "2026-10-10 12:00:00",
            },
          ],
          total: 2,
          page: 1,
          page_size: 20,
        })
      if (url.includes("/api/client/orders/1")) return jsonResponse(orderFixture)
      if (url.includes("/api/client/orders"))
        return jsonResponse({ list: [orderFixture], total: 1, page: 1, page_size: 20 })
      if (url.includes("/api/client/profile"))
        return jsonResponse({
          id: 1,
          username: "member01",
          email: "",
          phone: "",
          real_name: "",
          company: "",
          address: "",
          created_at: "2026-10-10 09:00:00",
        })
      return jsonResponse(null, 404)
    }),
  )
})

function renderPage(ui: ReactNode, route = "/") {
  return render(<MemoryRouter initialEntries={[route]}>{ui}</MemoryRouter>)
}

describe("商城列表 /store", () => {
  it("渲染分组与商品卡片（起价/周期价/描述摘要），且无未翻译 key", async () => {
    const { container } = renderPage(<StorePage />, "/store")
    expect((await screen.findAllByText(zhCN.store.title)).length).toBeGreaterThan(0)
    expect(await screen.findByText("二区 CN2标准性能")).toBeInTheDocument()
    expect(screen.getByText("香港二区 CN2 A型")).toBeInTheDocument()
    // 描述摘要：HTML 已转纯文本（出现 "CPU:2核心" 文本而非 <li> 元素）
    expect(await screen.findByText(/CPU:2核心/)).toBeInTheDocument()
    expect(screen.getByText(zhCN.store.priceFrom.replace("{{price}}", "20.00"))).toBeInTheDocument()
    expectNoRawKeys(container)
  })
})

describe("商品详情 /store/:productId", () => {
  function renderDetail() {
    return render(
      <MemoryRouter initialEntries={["/store/1"]}>
        <Routes>
          <Route path="/store/:productId" element={<StoreProductPage />} />
        </Routes>
      </MemoryRouter>,
    )
  }

  it("描述 HTML 经 sanitize：script/事件属性/javascript: URL 全部剥离", async () => {
    const { container } = renderDetail()
    expect(await screen.findByText("好机器")).toBeInTheDocument()
    // script 标签绝不允许入 DOM
    expect(container.querySelector("script")).toBeNull()
    // 事件属性（onerror）与 javascript: href 必须被剥掉
    expect(container.querySelector("[onerror]")).toBeNull()
    const links = Array.from(container.querySelectorAll("a"))
    for (const a of links) {
      expect(a.getAttribute("href") ?? "").not.toContain("javascript:")
    }
    // img 不在白名单：整个节点被移除
    expect(container.querySelector("img")).toBeNull()
    // 正常排版标签保留
    expect(container.querySelector("ul li")?.textContent).toBe("2核")
  })

  it("周期选择与总价（字符串金额乘法，无浮点漂移），且无未翻译 key", async () => {
    const { container } = renderDetail()
    // 19.99 × 2 = 39.98（浮点乘法会得 39.980000000000004）
    const qtyInput = await screen.findByLabelText(zhCN.product.qty)
    expect(qtyInput).toBeInTheDocument()
    const monthlyBtn = screen.getByText("月付")
    expect(monthlyBtn).toBeInTheDocument()
    // 改数量为 2
    const setter = Object.getOwnPropertyDescriptor(
      window.HTMLInputElement.prototype,
      "value",
    )?.set
    setter?.call(qtyInput, "2")
    qtyInput.dispatchEvent(new Event("input", { bubbles: true }))
    expect(await screen.findByText("¥39.98")).toBeInTheDocument()
    expect(screen.queryByText("¥39.980000000000004")).not.toBeInTheDocument()
    expect(screen.getByText(zhCN.product.buyNow)).toBeInTheDocument()
    expectNoRawKeys(container)
  })
})

describe("会员注册 /register", () => {
  it("渲染表单与双通道切换，且无未翻译 key", () => {
    const { container } = renderPage(<RegisterPage />, "/register")
    expect(screen.getByText(zhCN.register.title)).toBeInTheDocument()
    expect(container.textContent).toContain(zhCN.register.usernameHelp)
    expect(container.textContent).toContain(zhCN.register.channelEmail)
    expect(container.textContent).toContain(zhCN.register.channelSms)
    expect(container.textContent).toContain(zhCN.register.sendCode)
    expectNoRawKeys(container)
  })
})

describe("会员登录 /client-login", () => {
  it("渲染表单，且无未翻译 key", () => {
    const { container } = renderPage(<ClientLoginPage />, "/client-login")
    expect(screen.getByText(zhCN.clientLogin.title)).toBeInTheDocument()
    expect(container.textContent).toContain(zhCN.clientLogin.usernameHelp)
    expect(container.textContent).toContain(zhCN.clientLogin.toRegister)
    expectNoRawKeys(container)
  })
})

describe("我的主机 /console", () => {
  it("渲染主机行（状态文案/IP/到期/商品），且无未翻译 key", async () => {
    const { container } = renderPage(<ConsolePage />, "/console")
    expect(await screen.findByText("203.0.113.7")).toBeInTheDocument()
    expect(screen.getByText("HK-ABCD1234")).toBeInTheDocument()
    expect(screen.getByText(zhCN.console.status.Active)).toBeInTheDocument()
    expect(screen.getByText("2026-11-10 12:00:00")).toBeInTheDocument()
    expect(screen.getAllByText("香港二区 CN2 A型").length).toBeGreaterThan(0)
    expectNoRawKeys(container)
  })
})

describe("余额 /balance", () => {
  it("渲染余额与流水（带符号金额着色），且无未翻译 key", async () => {
    const { container } = renderPage(<BalancePage />, "/balance")
    expect(await screen.findByText("¥100.00")).toBeInTheDocument()
    expect(screen.getByText("人工充值")).toBeInTheDocument()
    expect(screen.getByText("+120.00")).toBeInTheDocument()
    expect(screen.getByText("-20.00")).toBeInTheDocument()
    expect(screen.getByText(zhCN.balance.rechargeHint)).toBeInTheDocument()
    expectNoRawKeys(container)
  })
})

describe("我的订单 /orders 与订单详情 /orders/:id", () => {
  it("列表渲染订单号与状态徽章，且无未翻译 key", async () => {
    const { container } = renderPage(<OrdersPage />, "/orders")
    expect(await screen.findByText("LIO202610100001")).toBeInTheDocument()
    expect(screen.getByText("开通中")).toBeInTheDocument()
    expect(screen.getByText("¥39.98")).toBeInTheDocument()
    expectNoRawKeys(container)
  })

  it("详情渲染条目与开通信息快照（IP/主机名/到期），且无未翻译 key", async () => {
    const { container } = render(
      <MemoryRouter initialEntries={["/orders/1"]}>
        <Routes>
          <Route path="/orders/:id" element={<OrderDetailPage />} />
        </Routes>
      </MemoryRouter>,
    )
    expect(await screen.findByText("LIO202610100001")).toBeInTheDocument()
    expect(screen.getByText(zhCN.orderDetail.infoIp)).toBeInTheDocument()
    expect(screen.getByText("203.0.113.7")).toBeInTheDocument()
    expect(screen.getByText("HK-ABCD1234")).toBeInTheDocument()
    expect(screen.getByText("UP2026101001")).toBeInTheDocument()
    expectNoRawKeys(container)
  })
})
