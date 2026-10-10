// 会员端布局壳（批 3-β）：顶部导航（商城/我的主机/订单/余额/工单占位）+
// 语言切换 + 登录态菜单（未登录显示 登录/注册 按钮）。
// 商城页面公开可浏览；/console、/balance、/orders 由路由层 RequireClientAuth 守卫。
import { useEffect, useState } from "react"
import { Link, NavLink, Outlet, useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Headphones, LogOut, Server, ShoppingBag, User, Wallet } from "lucide-react"

import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { LanguageSwitcher } from "@/components/language-switcher"
import { SiteLogo } from "@/components/site-logo"
import { api, clearClientToken, getClientToken, type ClientInfo } from "@/lib/api"
import { cn } from "@/lib/utils"

/** 导航项定义（to 为空 = 占位不可点，工单留批 4）。 */
const NAV_ITEMS = [
  { to: "/store", i18nKey: "clientNav.store", icon: ShoppingBag },
  { to: "/console", i18nKey: "clientNav.console", icon: Server },
  { to: "/orders", i18nKey: "clientNav.orders", icon: Wallet },
  { to: "/balance", i18nKey: "clientNav.balance", icon: Wallet },
  { to: "", i18nKey: "clientNav.tickets", icon: Headphones },
] as const

export default function ClientLayout() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const loggedIn = Boolean(getClientToken())
  const [client, setClient] = useState<ClientInfo | null>(null)

  // 登录态下拉取会员信息（401 时 api 层已清 token 跳会员登录页）
  useEffect(() => {
    if (!loggedIn) return
    let alive = true
    api.client
      .profile()
      .then((p) => {
        if (alive)
          setClient({
            id: p.id,
            username: p.username,
            email: p.email,
            phone: p.phone,
            real_name: p.real_name,
          })
      })
      .catch(() => {
        /* 401 已由 api 层全局处理 */
      })
    return () => {
      alive = false
    }
  }, [loggedIn])

  function handleLogout() {
    clearClientToken()
    toast.success(t("clientNav.logoutDone"))
    navigate("/store", { replace: true })
  }

  return (
    <div className="flex min-h-screen flex-col">
      <header className="sticky top-0 z-20 border-b bg-background/95 backdrop-blur">
        <div className="mx-auto flex h-14 max-w-6xl items-center gap-2 px-4">
          <Link to="/store" className="mr-2 shrink-0">
            <SiteLogo />
          </Link>
          <nav className="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto">
            {NAV_ITEMS.map((item) =>
              item.to ? (
                <NavLink
                  key={item.to}
                  to={item.to}
                  className={({ isActive }) =>
                    cn(
                      "flex shrink-0 items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors",
                      isActive
                        ? "bg-primary text-primary-foreground"
                        : "text-muted-foreground hover:bg-muted hover:text-foreground",
                    )
                  }
                >
                  <item.icon className="size-4" />
                  {t(item.i18nKey)}
                </NavLink>
              ) : (
                // 工单占位（批 4）：不可点击的禁用态
                <span
                  key={item.i18nKey}
                  className="flex shrink-0 cursor-not-allowed items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium text-muted-foreground/50"
                  title={t("clientNav.ticketsSoon")}
                >
                  <item.icon className="size-4" />
                  {t(item.i18nKey)}
                </span>
              ),
            )}
          </nav>
          <div className="flex shrink-0 items-center gap-1">
            <LanguageSwitcher />
            {loggedIn ? (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" className="gap-2 px-2">
                    <Avatar className="size-7">
                      <AvatarFallback className="text-xs">
                        {(client?.username || "U").slice(0, 1).toUpperCase()}
                      </AvatarFallback>
                    </Avatar>
                    <span className="hidden max-w-24 truncate text-sm sm:inline">
                      {client?.username || "…"}
                    </span>
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-44">
                  <DropdownMenuItem onClick={() => navigate("/console")}>
                    <User />
                    {t("clientNav.console")}
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem onClick={handleLogout} className="text-destructive">
                    <LogOut />
                    {t("clientNav.logout")}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            ) : (
              <div className="flex items-center gap-1">
                <Button asChild variant="ghost" size="sm">
                  <Link to="/client-login">{t("clientNav.login")}</Link>
                </Button>
                <Button asChild size="sm">
                  <Link to="/register">{t("clientNav.register")}</Link>
                </Button>
              </div>
            )}
          </div>
        </div>
      </header>

      <main className="mx-auto w-full max-w-6xl flex-1 p-4 md:p-6">
        <Outlet />
      </main>

      <footer className="border-t py-4 text-center text-xs text-muted-foreground">
        © {new Date().getFullYear()} · Powered by LiltIDC_OEM
      </footer>
    </div>
  )
}
