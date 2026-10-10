// 路由树（不含 Router 本体，便于测试用 MemoryRouter 包裹渲染）。
//
// 守卫结构（ADR-010，修复「未安装态访问 / 与 /login 未跳安装页」缺陷）：
// - /install 是唯一守卫外路由：未安装态的唯一入口
//   （其「已安装 → 跳回 /」自检在页面内部，方向保持不变）；
// - 其余全部路由（/、/login、/admin/*、* 兜底）包在 RequireInstalled
//   顶层布局路由内：未安装态访问任何路径都跳 /install；
//   后续批次新增路由默认受保护，杜绝逐条包守卫的遗漏（本缺陷根因）；
// - /admin 保持双重守卫：RequireInstalled（顶层）+ RequireAuth（本级），
//   未装 → /install，已装未登录 → /login；
// - 批 3-β 会员端：/store 系列公开可浏览（ClientLayout 壳），
//   /console、/balance、/orders 需会员登录（RequireClientAuth 布局路由，
//   token 键 liltidc_client_token，未登录 → /client-login）；
//   /register、/client-login 为独立整页（范式同 /login）。
import { lazy, Suspense } from "react"
import { Route, Routes } from "react-router-dom"
import { Loader2 } from "lucide-react"

import { RequireAuth, RequireClientAuth, RequireInstalled } from "@/routes/guards"
import Home from "@/pages/home"
import InstallPage from "@/pages/install"
import LoginPage from "@/pages/login"
import NotFoundPage from "@/pages/not-found"

// 后台路由树：按需加载（ADR-015 代码分割）
const AdminLayout = lazy(() => import("@/pages/admin/layout"))
const DashboardPage = lazy(() => import("@/pages/admin/dashboard"))
const PlaceholderPage = lazy(() => import("@/pages/admin/placeholder"))
const SettingsPage = lazy(() => import("@/pages/admin/settings"))
const ProductsPage = lazy(() => import("@/pages/admin/products"))
const UpstreamPage = lazy(() => import("@/pages/admin/upstream"))
const MembersPage = lazy(() => import("@/pages/admin/members"))
const AdminOrdersPage = lazy(() => import("@/pages/admin/orders"))

// 会员端路由树：同样按需加载（商城/控制台/余额/订单）
const ClientLayout = lazy(() => import("@/pages/client/layout"))
const StorePage = lazy(() => import("@/pages/client/store"))
const StoreProductPage = lazy(() => import("@/pages/client/product"))
const ClientRegisterPage = lazy(() => import("@/pages/client/register"))
const ClientLoginPage = lazy(() => import("@/pages/client/client-login"))
const ConsolePage = lazy(() => import("@/pages/client/console"))
const BalancePage = lazy(() => import("@/pages/client/balance"))
const ClientOrdersPage = lazy(() => import("@/pages/client/orders"))
const ClientOrderDetailPage = lazy(() => import("@/pages/client/order-detail"))

/** 懒加载路由的过渡占位。 */
function RouteFallback() {
  return (
    <div className="flex min-h-[50vh] items-center justify-center" role="status">
      <Loader2 className="size-6 animate-spin text-muted-foreground" />
    </div>
  )
}

export function AppRoutes() {
  return (
    <Routes>
      {/* 安装向导：守卫外的唯一路由 */}
      <Route path="/install" element={<InstallPage />} />

      {/* 其余全部路由：未安装态统一跳 /install（渲染期同步读缓存，无闪回） */}
      <Route element={<RequireInstalled />}>
        <Route path="/" element={<Home />} />
        <Route path="/login" element={<LoginPage />} />

        {/* 会员注册/登录：独立整页（免鉴权） */}
        <Route
          path="/register"
          element={
            <Suspense fallback={<RouteFallback />}>
              <ClientRegisterPage />
            </Suspense>
          }
        />
        <Route
          path="/client-login"
          element={
            <Suspense fallback={<RouteFallback />}>
              <ClientLoginPage />
            </Suspense>
          }
        />

        {/* 会员端布局壳（顶部导航）：商城公开可浏览 */}
        <Route
          element={
            <Suspense fallback={<RouteFallback />}>
              <ClientLayout />
            </Suspense>
          }
        >
          <Route path="/store" element={<StorePage />} />
          <Route path="/store/:productId" element={<StoreProductPage />} />

          {/* 需会员登录的页面（守卫在布局层，未登录统一跳 /client-login） */}
          <Route element={<RequireClientAuth />}>
            <Route path="/console" element={<ConsolePage />} />
            <Route path="/balance" element={<BalancePage />} />
            <Route path="/orders" element={<ClientOrdersPage />} />
            <Route path="/orders/:id" element={<ClientOrderDetailPage />} />
          </Route>
        </Route>

        <Route
          path="/admin"
          element={
            <RequireAuth>
              <Suspense fallback={<RouteFallback />}>
                <AdminLayout />
              </Suspense>
            </RequireAuth>
          }
        >
          <Route index element={<DashboardPage />} />
          <Route path="products" element={<ProductsPage />} />
          <Route path="orders" element={<AdminOrdersPage />} />
          <Route path="members" element={<MembersPage />} />
          <Route path="tickets" element={<PlaceholderPage titleKey="nav.tickets" />} />
          <Route path="finance" element={<PlaceholderPage titleKey="nav.finance" />} />
          <Route path="upstream" element={<UpstreamPage />} />
          <Route path="settings" element={<SettingsPage />} />
        </Route>
        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  )
}
