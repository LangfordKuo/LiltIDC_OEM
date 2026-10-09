// 路由守卫组件。
//
// 关键设计（docs/architecture.md ADR-010）：安装状态在 bootstrap 阶段已预取进
// 模块级缓存，守卫在「渲染期」通过 useInstallState() 同步读取，
// 首帧即可正确重定向，不会先渲染受保护页再闪回。
import type { ReactNode } from "react"
import { Navigate, Outlet, useLocation } from "react-router-dom"

import { getToken } from "@/lib/api"
import { useInstallState } from "@/lib/install-state"

/**
 * 已安装才可访问；未安装跳 /install（null=未知时放行，由 API 层 409 兜底跳转）。
 *
 * 两种使用形态：
 * 1. 包裹子元素：`<RequireInstalled><Page /></RequireInstalled>`
 * 2. 作为无 path 的顶层布局路由：`<Route element={<RequireInstalled />}>`
 *    （无 children 时渲染 <Outlet />，当前 App 采用此形态保护除 /install 外全部路由）
 */
export function RequireInstalled({ children }: { children?: ReactNode }) {
  const installed = useInstallState()
  if (installed === false) {
    return <Navigate to="/install" replace />
  }
  return <>{children ?? <Outlet />}</>
}

/** 需登录才可访问；无 token 跳 /login 并记录来源路径。 */
export function RequireAuth({ children }: { children: ReactNode }) {
  const location = useLocation()
  if (!getToken()) {
    return <Navigate to="/login" replace state={{ from: location }} />
  }
  return <>{children}</>
}
