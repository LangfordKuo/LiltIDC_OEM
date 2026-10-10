// 路由守卫集成测试（修复「未安装态访问 / 与 /login 未跳安装页」缺陷的回归防线）。
//
// 用 MemoryRouter + AppRoutes 渲染完整路由树，覆盖三种安装状态：
// - 未安装（false）：除 /install 外任何路由都必须落到安装向导；
// - 已安装（true）：/ 停留首页、/install 跳回首页（原正确方向不许弄坏）、
//   /admin 未登录跳登录页（双重守卫次序保持）；
// - 未知（null）：放行，由 API 层 409 兜底（守卫不抢先，ADR-010）。
import { beforeAll, beforeEach, describe, expect, it } from "vitest"
import { render, screen } from "@testing-library/react"
import { MemoryRouter } from "react-router-dom"

import i18n, { i18nReady } from "@/i18n"
import zhCN from "@/i18n/locales/zh-CN.json"
import { setInstallState } from "@/lib/install-state"
import { AppRoutes } from "@/routes/app-routes"

/** 在指定路径渲染完整路由树。 */
function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <AppRoutes />
    </MemoryRouter>,
  )
}

beforeAll(async () => {
  await i18nReady
  // jsdom 的 navigator.language 是 en-US，显式固定中文再断言译文
  await i18n.changeLanguage("zh-CN")
})

beforeEach(() => {
  // 每个用例前重置为未知态，各 describe 再显式设定
  setInstallState(null)
  localStorage.removeItem("liltidc_token")
})

describe("未安装态：除 /install 外全部路由跳安装向导", () => {
  beforeEach(() => {
    setInstallState(false)
  })

  it.each(["/", "/login", "/admin", "/admin/settings", "/any/unknown/path"])(
    "访问 %s 落到安装向导，不停留原页面",
    (path) => {
      renderAt(path)
      expect(screen.getByText(zhCN.install.title)).toBeInTheDocument()
      expect(screen.queryByText(zhCN.home.nowOpen)).not.toBeInTheDocument()
      expect(screen.queryByText(zhCN.login.title)).not.toBeInTheDocument()
    },
  )

  it("访问 /install 正常显示安装向导（守卫外路由，不受影响）", () => {
    renderAt("/install")
    expect(screen.getByText(zhCN.install.title)).toBeInTheDocument()
  })
})

describe("已安装态：原有正确方向保持不变", () => {
  beforeEach(() => {
    setInstallState(true)
  })

  it("访问 / 停留首页，不跳安装页", () => {
    renderAt("/")
    expect(screen.getByText(zhCN.home.nowOpen)).toBeInTheDocument()
    expect(screen.queryByText(zhCN.install.title)).not.toBeInTheDocument()
  })

  it("访问 /install 跳回首页（页面内部自检）", () => {
    renderAt("/install")
    expect(screen.getByText(zhCN.home.nowOpen)).toBeInTheDocument()
    expect(screen.queryByText(zhCN.install.title)).not.toBeInTheDocument()
  })

  it("访问 /admin 未登录跳登录页（RequireInstalled + RequireAuth 双重守卫次序）", () => {
    renderAt("/admin")
    expect(screen.getByText(zhCN.login.title)).toBeInTheDocument()
    expect(screen.queryByText(zhCN.install.title)).not.toBeInTheDocument()
  })
})

describe("未知态（install-state=null）：守卫放行，由 API 层 409 兜底", () => {
  it("访问 / 渲染首页，不抢先跳 /install", () => {
    setInstallState(null)
    renderAt("/")
    expect(screen.getByText(zhCN.home.nowOpen)).toBeInTheDocument()
    expect(screen.queryByText(zhCN.install.title)).not.toBeInTheDocument()
  })
})

// ---- 批 3-β：会员守卫 RequireClientAuth（token 键 liltidc_client_token） ----
describe("会员端守卫（批 3-β）", () => {
  beforeEach(() => {
    setInstallState(true)
    localStorage.removeItem("liltidc_client_token")
  })

  it("未登录访问 /console 跳会员登录页（不是管理员登录页）", async () => {
    renderAt("/console")
    expect(await screen.findByText(zhCN.clientLogin.title)).toBeInTheDocument()
    expect(screen.queryByText(zhCN.login.title)).not.toBeInTheDocument()
  })

  it("未登录访问 /balance、/orders 同样跳会员登录页", async () => {
    renderAt("/balance")
    expect(await screen.findByText(zhCN.clientLogin.title)).toBeInTheDocument()
  })

  it("携带会员 token 访问 /console 渲染我的主机（守卫放行）", async () => {
    localStorage.setItem("liltidc_client_token", "test-client-token")
    renderAt("/console")
    // 导航与页面标题同文案，用 findAllByText
    expect((await screen.findAllByText(zhCN.console.title)).length).toBeGreaterThan(0)
    expect(screen.queryByText(zhCN.clientLogin.title)).not.toBeInTheDocument()
  })

  it("/store 公开可浏览（未登录不跳转）", async () => {
    renderAt("/store")
    expect((await screen.findAllByText(zhCN.store.title)).length).toBeGreaterThan(0)
    expect(screen.queryByText(zhCN.clientLogin.title)).not.toBeInTheDocument()
  })

  it("未安装态访问 /console 仍优先跳安装向导（RequireInstalled 在外层）", () => {
    setInstallState(false)
    localStorage.setItem("liltidc_client_token", "test-client-token")
    renderAt("/console")
    expect(screen.getByText(zhCN.install.title)).toBeInTheDocument()
  })
})
