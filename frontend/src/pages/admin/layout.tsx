// 后台布局壳：侧边栏（桌面固定 / 移动 Sheet 抽屉）+ 顶栏（语言/主题/用户菜单）+ 内容区。
// 子路由为 React.lazy 动态加载（ADR-015），Outlet 外包 Suspense：
// 切换菜单时侧边栏/顶栏保持挂载，仅内容区显示过渡占位。
import { Suspense, useEffect, useState } from "react"
import { Link, NavLink, Outlet, useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Home, Loader2, LogOut, Menu } from "lucide-react"

import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Separator } from "@/components/ui/separator"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { LanguageSwitcher } from "@/components/language-switcher"
import { SiteLogo } from "@/components/site-logo"
import { api, clearToken, type AdminInfo } from "@/lib/api"
import { useBrand } from "@/lib/brand"
import { cn } from "@/lib/utils"

import { navSections } from "./nav"

export default function AdminLayout() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const brand = useBrand()
  const [mobileOpen, setMobileOpen] = useState(false)
  const [admin, setAdmin] = useState<AdminInfo | null>(null)

  // 拉取当前管理员信息（401 时 api 层已自动清 token 跳登录）
  useEffect(() => {
    let alive = true
    api.auth
      .me()
      .then((info) => {
        if (alive) setAdmin(info)
      })
      .catch(() => {
        /* 401 已由 api 层全局处理 */
      })
    return () => {
      alive = false
    }
  }, [])

  function handleLogout() {
    clearToken()
    toast.success(t("topbar.logoutDone"))
    navigate("/login", { replace: true })
  }

  const displayName = admin ? admin.real_name || admin.username : "…"

  return (
    <div className="flex min-h-screen">
      {/* 桌面端侧边栏 */}
      <aside className="sticky top-0 hidden h-screen w-60 shrink-0 border-r bg-sidebar md:block">
        <SidebarContent />
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        {/* 顶栏 */}
        <header className="sticky top-0 z-20 flex h-14 items-center gap-2 border-b bg-background/95 px-4 backdrop-blur">
          <Button
            variant="ghost"
            size="icon"
            className="md:hidden"
            aria-label={t("topbar.adminPanel")}
            onClick={() => setMobileOpen(true)}
          >
            <Menu />
          </Button>
          <span className="text-sm font-medium">{t("topbar.adminPanel")}</span>
          <div className="ml-auto flex items-center gap-1">
            <LanguageSwitcher />
            <Separator orientation="vertical" className="mx-1 h-6" />
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" className="gap-2 px-2">
                  <Avatar className="size-7">
                    <AvatarFallback className="text-xs">
                      {displayName.slice(0, 1).toUpperCase()}
                    </AvatarFallback>
                  </Avatar>
                  <span className="hidden max-w-24 truncate text-sm sm:inline">{displayName}</span>
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="w-44">
                <DropdownMenuItem onClick={() => navigate("/")}>
                  <Home />
                  {t("topbar.frontHome")}
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem onClick={handleLogout} className="text-destructive">
                  <LogOut />
                  {t("topbar.logout")}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </header>

        {/* 内容区（子路由懒加载过渡） */}
        <main className="flex-1 p-4 md:p-6">
          <Suspense
            fallback={
              <div className="flex min-h-[40vh] items-center justify-center" role="status">
                <Loader2 className="size-6 animate-spin text-muted-foreground" />
              </div>
            }
          >
            <Outlet />
          </Suspense>
        </main>
      </div>

      {/* 移动端抽屉侧边栏 */}
      <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
        <SheetContent side="left" className="w-64 p-0">
          <SheetHeader className="sr-only">
            <SheetTitle>{brand?.site_name || "LiltIDC"}</SheetTitle>
            <SheetDescription>{t("topbar.adminPanel")}</SheetDescription>
          </SheetHeader>
          <SidebarContent onNavigate={() => setMobileOpen(false)} />
        </SheetContent>
      </Sheet>
    </div>
  )
}

/** 侧边栏内容（桌面与移动抽屉共用）。 */
function SidebarContent({ onNavigate }: { onNavigate?: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="flex h-full flex-col">
      <div className="flex h-14 shrink-0 items-center border-b px-4">
        <SiteLogo />
      </div>
      <nav className="flex-1 overflow-y-auto p-3">
        {navSections.map((section, si) => (
          <div key={si} className={si > 0 ? "mt-5" : undefined}>
            {section.titleKey && (
              <p className="mb-1.5 px-3 text-xs font-medium text-muted-foreground">
                {t(section.titleKey)}
              </p>
            )}
            <div className="space-y-1">
              {section.items.map((item) => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  end={item.end}
                  onClick={onNavigate}
                  className={({ isActive }) =>
                    cn(
                      "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                      isActive
                        ? "bg-primary text-primary-foreground"
                        : "text-muted-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground",
                    )
                  }
                >
                  <item.icon className="size-4 shrink-0" />
                  <span className="truncate">{t(item.i18nKey)}</span>
                </NavLink>
              ))}
            </div>
          </div>
        ))}
      </nav>
      <div className="shrink-0 border-t p-3">
        <Button
          asChild
          variant="ghost"
          size="sm"
          className="w-full justify-start gap-2 text-muted-foreground"
        >
          <Link to="/" onClick={onNavigate}>
            <Home className="size-4" />
            {t("topbar.frontHome")}
          </Link>
        </Button>
      </div>
    </div>
  )
}
