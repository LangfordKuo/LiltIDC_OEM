// 安装状态模块级缓存。
//
// 设计（docs/architecture.md ADR-010）：
// - bootstrap（main.tsx）在首次渲染前拉取 /api/install/status 写入缓存；
// - 路由守卫在「渲染期」通过 useInstallState() 同步读取，避免 useEffect 异步守卫造成的闪回；
// - api 客户端收到 409 not_installed 时更新缓存并跳转 /install。
import { useSyncExternalStore } from "react"

// null = 未知（网络失败等），true/false = 后端权威状态
let cached: boolean | null = null

const listeners = new Set<() => void>()

function emit() {
  for (const fn of listeners) fn()
}

/** 同步读取缓存的安装状态（渲染期安全）。 */
export function getInstallState(): boolean | null {
  return cached
}

/** 更新缓存并通知订阅者。 */
export function setInstallState(value: boolean | null) {
  if (cached === value) return
  cached = value
  emit()
}

function subscribe(fn: () => void): () => void {
  listeners.add(fn)
  return () => {
    listeners.delete(fn)
  }
}

/** React Hook：订阅安装状态（渲染期同步读，无闪烁）。 */
export function useInstallState(): boolean | null {
  return useSyncExternalStore(subscribe, getInstallState, getInstallState)
}

/**
 * 从后端刷新安装状态（bootstrap 与安装完成后调用）。
 * 直接用 fetch，避免与 api.ts 循环依赖；失败时保持 null（未知），不误跳安装页。
 */
export async function refreshInstallState(): Promise<boolean | null> {
  try {
    const resp = await fetch("/api/install/status")
    if (!resp.ok) return null
    const json = (await resp.json()) as {
      data?: { installed?: boolean }
    }
    const installed = json?.data?.installed === true
    setInstallState(installed)
    return installed
  } catch {
    setInstallState(null)
    return null
  }
}
