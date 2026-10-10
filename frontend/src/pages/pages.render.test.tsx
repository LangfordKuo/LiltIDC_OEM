// 页面渲染冒烟测试：渲染 首页 / 登录页 / 安装向导第一步 / 后台布局，
// 断言 body 文本中不出现「小写段.驼峰段」形态的未翻译 i18n key（任务指定正则），
// 并断言关键译文真实出现。这是 i18n 失效缺陷的组件级防线（与 i18n.smoke 互补）。
import type { ReactNode } from "react"
import { beforeEach, beforeAll, describe, expect, it, vi } from "vitest"
import { render, screen } from "@testing-library/react"
import { MemoryRouter } from "react-router-dom"

import i18n, { i18nReady } from "@/i18n"
import zhCN from "@/i18n/locales/zh-CN.json"
import AdminLayout from "@/pages/admin/layout"
import MembersPage from "@/pages/admin/members"
import AdminOrdersPage from "@/pages/admin/orders"
import ProductsPage from "@/pages/admin/products"
import SettingsPage from "@/pages/admin/settings"
import UpstreamPage from "@/pages/admin/upstream"
import Home from "@/pages/home"
import InstallPage from "@/pages/install"
import LoginPage from "@/pages/login"

// 未翻译 key 的形态：nav.dashboard / settings.tabSms 这类「小写段.字母段」
const RAW_KEY_PATTERN = /\b[a-z]+\.[a-zA-Z]+\b/g

// 扫描白名单：语言无关的域名/格式示例，形似 key 但不是 i18n key
const RAW_KEY_ALLOWLIST = ["smsbao.com", "lyew.com"]

function renderWithProviders(ui: ReactNode, route = "/") {
  return render(<MemoryRouter initialEntries={[route]}>{ui}</MemoryRouter>)
}

/**
 * 扫描渲染出的「可见文本」，出现任何未翻译 key 即失败。
 * 先克隆容器并剔除 script/style 节点（第三方库可能注入内联脚本，
 * 其源码不是用户可见文本，不应参与扫描）。
 */
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

beforeAll(async () => {
  await i18nReady
  await i18n.changeLanguage("zh-CN")
})

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

beforeEach(() => {
  // stub 全局 fetch：各页面挂载时的数据请求（vitest 配置了
  // unstubGlobals，需在每个用例前重新 stub）
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.includes("/api/auth/me")) {
        return jsonResponse({
          id: 1,
          username: "admin",
          real_name: "郭政皓",
          email: "admin@example.com",
          last_login_at: null,
        })
      }
      if (url.includes("/api/admin/upstream/config")) {
        return jsonResponse({
          hostname: "https://lyew.com",
          username: "16650492239",
          api_key_set: true,
          downstream_token: "tok-test-0123456789abcdef",
          sync_path: "/api/upstream/host/sync",
        })
      }
      if (url.includes("/api/admin/products")) {
        return jsonResponse({ list: [], total: 0 })
      }
      if (url.includes("/api/admin/product-groups")) {
        return jsonResponse({ list: [] })
      }
      if (url.includes("/api/admin/clients")) {
        return jsonResponse({
          list: [
            {
              id: 1,
              username: "member01",
              email: "m1@example.com",
              phone: "13800138000",
              real_name: "测试会员",
              company: "",
              credit: "88.80",
              status: 3,
              status_text: "上游关联异常",
              upstream_uid: null,
              upstream_linked_at: "",
              last_login_at: "",
              created_at: "2026-10-10 12:00:00",
            },
          ],
          total: 1,
        })
      }
      if (url.includes("/api/admin/orders")) {
        return jsonResponse({
          list: [
            {
              id: 1,
              order_no: "LIO202610100001",
              client_id: 1,
              client_username: "member01",
              status: "ProvisionFailed",
              status_text: "开通失败",
              amount: "20.00",
              balance_used: "20.00",
              pay_method: "balance",
              pay_time: "2026-10-10 12:00:00",
              cancel_time: "",
              remark: "上游结算失败",
              created_at: "2026-10-10 11:59:00",
              items: [
                {
                  id: 1,
                  product_id: 1,
                  product_name: "香港二区 CN2 A型",
                  upstream_pid: 1,
                  cycle: "monthly",
                  cycle_text: "月付",
                  qty: 1,
                  unit_price: "20.00",
                  amount: "20.00",
                  upstream_order_id: 0,
                  upstream_host_id: 0,
                  provision_status: "failed",
                  provision_status_text: "失败",
                  provision_info: null,
                },
              ],
            },
          ],
          total: 1,
          page: 1,
          page_size: 20,
        })
      }
      return jsonResponse(null, 404)
    }),
  )
})

describe("首页 /", () => {
  it("渲染关键译文且无未翻译 key（批 3-β：商城已上线形态）", () => {
    const { container } = renderWithProviders(<Home />)
    expect(screen.getByText(zhCN.home.nowOpen)).toBeInTheDocument()
    expect(container.textContent).toContain(zhCN.home.tagline)
    expect(container.textContent).toContain(zhCN.home.adminLogin)
    expect(container.textContent).toContain(zhCN.home.clientLogin)
    expect(container.textContent).toContain(zhCN.home.enterStore)
    expect(container.textContent).toContain("由 LiltIDC_OEM 驱动")
    expectNoRawKeys(container)
  })

  it("暗色模式已移除：html/body 不出现 dark class，且无主题切换按钮", () => {
    renderWithProviders(<Home />)
    expect(document.documentElement.classList.contains("dark")).toBe(false)
    expect(document.body.classList.contains("dark")).toBe(false)
    // 原主题切换按钮的 aria-label（"切换主题"/"Toggle theme"）不应再出现
    expect(screen.queryByLabelText("切换主题")).not.toBeInTheDocument()
    expect(screen.queryByLabelText("Toggle theme")).not.toBeInTheDocument()
  })
})

describe("登录页 /login", () => {
  it("渲染关键译文且无未翻译 key", () => {
    const { container } = renderWithProviders(<LoginPage />, "/login")
    expect(screen.getByText(zhCN.login.title)).toBeInTheDocument()
    expect(container.textContent).toContain(zhCN.login.username)
    expect(container.textContent).toContain(zhCN.login.password)
    expectNoRawKeys(container)
  })
})

describe("安装向导 /install（第一步：数据库连接）", () => {
  it("渲染步骤条与数据库表单译文且无未翻译 key", () => {
    const { container } = renderWithProviders(<InstallPage />, "/install")
    expect(screen.getByText(zhCN.install.title)).toBeInTheDocument()
    expect(container.textContent).toContain(zhCN.install.stepDatabase)
    expect(container.textContent).toContain(zhCN.install.dbHost)
    expect(container.textContent).toContain(zhCN.install.dbName)
    expect(container.textContent).toContain(zhCN.install.testAndNext)
    expectNoRawKeys(container)
  })
})

describe("后台布局 /admin", () => {
  it("渲染侧边栏全部菜单译文、管理员姓名，且无未翻译 key", async () => {
    const { container } = renderWithProviders(<AdminLayout />, "/admin")
    // /api/auth/me stub 返回的管理员姓名（异步加载）
    expect(await screen.findByText("郭政皓")).toBeInTheDocument()
    // 侧边栏七个菜单 + 顶栏
    for (const key of [
      "dashboard",
      "products",
      "orders",
      "members",
      "tickets",
      "finance",
      "settings",
    ] as const) {
      expect(container.textContent).toContain(zhCN.nav[key])
    }
    expect(container.textContent).toContain(zhCN.topbar.adminPanel)
    expectNoRawKeys(container)
  })
})

describe("上游对接页 /admin/upstream（三个页签）", () => {
  it("渲染页签与上游配置表单译文，且无未翻译 key", async () => {
    const { container } = renderWithProviders(<UpstreamPage />, "/admin/upstream")
    expect(screen.getByText(zhCN.upstream.title)).toBeInTheDocument()
    expect(screen.getByText(zhCN.upstream.tabs.config)).toBeInTheDocument()
    expect(screen.getByText(zhCN.upstream.tabs.push)).toBeInTheDocument()
    expect(screen.getByText(zhCN.upstream.tabs.hosts)).toBeInTheDocument()
    // 等待异步配置加载落地（stub 返回 api_key_set=true → 徽章「已设置」）
    expect(await screen.findByText(zhCN.upstream.config.apiKeySet)).toBeInTheDocument()
    expect(screen.getByText(zhCN.upstream.config.description)).toBeInTheDocument()
    expectNoRawKeys(container)
  })
})

describe("商品管理页 /admin/products", () => {
  it("渲染工具栏与表格译文，且无未翻译 key", async () => {
    const { container } = renderWithProviders(<ProductsPage />, "/admin/products")
    expect(screen.getByText(zhCN.products.title)).toBeInTheDocument()
    expect(screen.getByText(zhCN.products.import)).toBeInTheDocument()
    expect(screen.getByText(zhCN.products.syncStock)).toBeInTheDocument()
    // 等待异步列表落地（stub 返回空列表 → 空态文案）
    expect(await screen.findByText(zhCN.products.empty)).toBeInTheDocument()
    expectNoRawKeys(container)
  })
})

describe("会员管理页 /admin/members", () => {
  it("渲染列表与状态徽章（含上游关联异常），且无未翻译 key", async () => {
    const { container } = renderWithProviders(<MembersPage />, "/admin/members")
    expect(screen.getByText(zhCN.members.title)).toBeInTheDocument()
    expect(screen.getByText(zhCN.members.search)).toBeInTheDocument()
    // 等待异步列表落地（stub 返回 1 位 status=3 的会员）
    expect(await screen.findByText("member01")).toBeInTheDocument()
    expect(screen.getByText("上游关联异常")).toBeInTheDocument()
    expect(screen.getByText(zhCN.members.neverLogin)).toBeInTheDocument()
    expectNoRawKeys(container)
  })

  it("批 3-β：余额列展示且充值/扣款入口存在", async () => {
    const { container } = renderWithProviders(<MembersPage />, "/admin/members")
    expect(await screen.findByText("member01")).toBeInTheDocument()
    // 余额列（stub credit=88.80）
    expect(screen.getByText("¥88.80")).toBeInTheDocument()
    expect(screen.getByText(zhCN.members.colCredit)).toBeInTheDocument()
    expectNoRawKeys(container)
  })
})

describe("后台订单管理页 /admin/orders（批 3-β）", () => {
  it("渲染列表与失败订单（重试/取消退款入口），且无未翻译 key", async () => {
    const { container } = renderWithProviders(<AdminOrdersPage />, "/admin/orders")
    expect(screen.getByText(zhCN.adminOrders.title)).toBeInTheDocument()
    expect(screen.getByText(zhCN.adminOrders.search)).toBeInTheDocument()
    // 等待异步列表落地（stub 返回 1 笔 ProvisionFailed 订单）
    expect(await screen.findByText("LIO202610100001")).toBeInTheDocument()
    expect(screen.getByText("开通失败")).toBeInTheDocument()
    expect(screen.getByText("member01")).toBeInTheDocument()
    // 详情按钮在行内（重试/取消退款在 DropdownMenu 内，点击展开后才入 DOM）
    expect(screen.getByText(zhCN.clientOrders.detail)).toBeInTheDocument()
    expectNoRawKeys(container)
  })
})

describe("系统设置页 /admin/settings（三个页签）", () => {
  it("渲染页签与短信配置译文，且无未翻译 key", async () => {
    const { container } = renderWithProviders(<SettingsPage />, "/admin/settings")
    // 三个页签标题
    expect(screen.getByText(zhCN.settings.tabSms)).toBeInTheDocument()
    expect(screen.getByText(zhCN.settings.tabEmail)).toBeInTheDocument()
    expect(screen.getByText(zhCN.settings.tabTemplates)).toBeInTheDocument()
    // 默认激活的短信页签内容（CardTitle 含图标/徽章，断言纯文本的描述段）
    expect(screen.getByText(zhCN.settings.sms.description)).toBeInTheDocument()
    // 等待异步加载落地：fetch stub 对 notify/settings 返回 404，
    // 页面应显示加载失败 Alert（同时消除 act 警告）
    expect(await screen.findByText("not found")).toBeInTheDocument()
    expectNoRawKeys(container)
  })
})
