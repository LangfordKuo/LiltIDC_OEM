// 后台侧边栏菜单结构（批 1 为结构占位，除仪表盘与系统设置外内容为空白页）。
import {
  LayoutDashboard,
  LifeBuoy,
  Package,
  Settings,
  ShoppingCart,
  Users,
  Wallet,
  type LucideIcon,
} from "lucide-react"

export interface NavItem {
  /** 路由路径 */
  to: string
  /** 菜单图标 */
  icon: LucideIcon
  /** i18n 键 */
  i18nKey: string
  /** NavLink 精确匹配（仅 /admin 首页需要） */
  end?: boolean
}

export const navItems: NavItem[] = [
  { to: "/admin", icon: LayoutDashboard, i18nKey: "nav.dashboard", end: true },
  { to: "/admin/products", icon: Package, i18nKey: "nav.products" },
  { to: "/admin/orders", icon: ShoppingCart, i18nKey: "nav.orders" },
  { to: "/admin/members", icon: Users, i18nKey: "nav.members" },
  { to: "/admin/tickets", icon: LifeBuoy, i18nKey: "nav.tickets" },
  { to: "/admin/finance", icon: Wallet, i18nKey: "nav.finance" },
  { to: "/admin/settings", icon: Settings, i18nKey: "nav.settings" },
]
