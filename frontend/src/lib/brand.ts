// 品牌信息（站名/Logo/主色）加载与应用。
//
// 主色应用方式（docs/architecture.md ADR-008）：
// 运行时把十六进制主色写入 CSS 变量 --primary/--ring，并按相对亮度
// 自动选择黑/白前景写入 --primary-foreground，shadcn 全组件随之换肤。
import { useSyncExternalStore } from "react"

export interface BrandInfo {
  site_name: string
  logo: string
  primary_color: string
}

let cached: BrandInfo | null = null

const listeners = new Set<() => void>()

function emit() {
  for (const fn of listeners) fn()
}

/** 同步读取缓存的品牌信息（可能为 null = 尚未加载成功）。 */
export function getBrand(): BrandInfo | null {
  return cached
}

function subscribe(fn: () => void): () => void {
  listeners.add(fn)
  return () => {
    listeners.delete(fn)
  }
}

/** React Hook：订阅品牌信息。 */
export function useBrand(): BrandInfo | null {
  return useSyncExternalStore(subscribe, getBrand, getBrand)
}

/**
 * 从后端刷新品牌信息并应用到 CSS 变量（bootstrap 与安装完成后调用）。
 * 该接口未安装态也可访问；失败时静默（保留默认样式）。
 */
export async function refreshBrand(): Promise<BrandInfo | null> {
  try {
    const resp = await fetch("/api/public/brand")
    if (!resp.ok) return null
    const json = (await resp.json()) as { data?: BrandInfo }
    if (!json?.data) return null
    cached = json.data
    applyBrand(json.data)
    emit()
    return cached
  } catch {
    return null
  }
}

/** 把品牌信息应用到 document（CSS 变量 + 标题）。 */
export function applyBrand(brand: BrandInfo) {
  const root = document.documentElement
  const color = normalizeHex(brand.primary_color)
  if (color) {
    root.style.setProperty("--primary", color)
    root.style.setProperty("--ring", color)
    root.style.setProperty("--primary-foreground", readableForeground(color))
  }
  if (brand.site_name) {
    document.title = brand.site_name
  }
}

/** 校验并归一化 #RRGGBB；非法返回 null。 */
function normalizeHex(value: string): string | null {
  if (/^#[0-9a-fA-F]{6}$/.test(value)) return value.toLowerCase()
  return null
}

/** 按 WCAG 相对亮度选择前景色（浅色背景配深色字，反之配白字）。 */
function readableForeground(hex: string): string {
  const r = parseInt(hex.slice(1, 3), 16) / 255
  const g = parseInt(hex.slice(3, 5), 16) / 255
  const b = parseInt(hex.slice(5, 7), 16) / 255
  const lin = (c: number) => (c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4))
  const luminance = 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b)
  return luminance > 0.45 ? "#18181b" : "#ffffff"
}
