// 后台侧边栏菜单结构（分组 + 项）。
// 批 2 起「上游对接」独立分组；商品管理已接真实页面，其余仍为占位。
import {
  Cloud,
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

export interface NavSection {
  /** 分组标题 i18n 键（可选；无标题则紧凑排列） */
  titleKey?: string
  items: NavItem[]
}

export const navSections: NavSection[] = [
  {
    items: [
      { to: "/admin", icon: LayoutDashboard, i18nKey: "nav.dashboard", end: true },
      { to: "/admin/products", icon: Package, i18nKey: "nav.products" },
      { to: "/admin/orders", icon: ShoppingCart, i18nKey: "nav.orders" },
      { to: "/admin/members", icon: Users, i18nKey: "nav.members" },
      { to: "/admin/tickets", icon: LifeBuoy, i18nKey: "nav.tickets" },
      { to: "/admin/finance", icon: Wallet, i18nKey: "nav.finance" },
    ],
  },
  {
    titleKey: "nav.sectionUpstream",
    items: [{ to: "/admin/upstream", icon: Cloud, i18nKey: "nav.upstream" }],
  },
  {
    items: [{ to: "/admin/settings", icon: Settings, i18nKey: "nav.settings" }],
  },
]
