// 价格展示工具：计费周期规范排序、价格摘要、利润率计算。
// 周期枚举与后端 model.BillingCycles / 契约文档 8.x 保持一致。
import type { PriceMap } from "@/lib/api"

// 与后端 model.BillingCycles / 上游 product_pricings 周期键完全一致
// （lyew.com 实测 17 键；semiannually 无下划线，本地规范键统一采用上游键名）
export const CYCLE_ORDER = [
  "onetime",
  "hour",
  "day",
  "ontrial",
  "monthly",
  "quarterly",
  "semiannually",
  "annually",
  "biennially",
  "triennially",
  "fourly",
  "fively",
  "sixly",
  "sevenly",
  "eightly",
  "ninely",
  "tenly",
]

/** 按规范周期序排序价格条目（未知周期排末尾、按字母序）。 */
export function sortedPriceEntries(prices: PriceMap): [string, string][] {
  return Object.entries(prices).sort((a, b) => {
    const ia = CYCLE_ORDER.indexOf(a[0])
    const ib = CYCLE_ORDER.indexOf(b[0])
    if (ia !== -1 && ib !== -1) return ia - ib
    if (ia !== -1) return -1
    if (ib !== -1) return 1
    return a[0].localeCompare(b[0])
  })
}

/** 价格摘要文本："月付 15.00 / 年付 150.00"（cycleLabel 由调用方注入 i18n）。 */
export function priceSummary(prices: PriceMap, cycleLabel: (cycle: string) => string): string {
  return sortedPriceEntries(prices)
    .map(([cycle, price]) => `${cycleLabel(cycle)} ${price}`)
    .join(" / ")
}

/** 利润率百分比文本（(售价-成本)/成本×100）；无法计算时返回 null。 */
export function marginPercent(cost: string | undefined, sale: string): string | null {
  const c = Number(cost)
  const s = Number(sale)
  if (!cost || Number.isNaN(c) || c <= 0) return null
  if (!sale.trim() || Number.isNaN(s)) return null
  return (((s - c) / c) * 100).toFixed(1)
}

/** 校验价格文本是有效正数。 */
export function isValidPrice(price: string): boolean {
  const trimmed = price.trim()
  if (!trimmed) return false
  const n = Number(trimmed)
  return Number.isFinite(n) && n > 0
}

/**
 * 十进制金额文本 → 整数分（BigInt）。纯字符串解析，不经 float——
 * 金额运算禁止浮点（与后端 money 包同一纪律，ADR-027）。非法输入返回 null。
 */
export function priceToCents(price: string): bigint | null {
  const m = /^\+?(\d+)(?:\.(\d{1,2}))?$/.exec(price.trim())
  if (!m) return null
  const frac = (m[2] ?? "").padEnd(2, "0")
  return BigInt(m[1]) * 100n + BigInt(frac)
}

/** 整数分 → 两位小数金额文本。 */
export function centsToPrice(cents: bigint): string {
  const neg = cents < 0n
  const abs = neg ? -cents : cents
  const s = `${abs / 100n}.${String(abs % 100n).padStart(2, "0")}`
  return neg ? `-${s}` : s
}

/** 单价 × 数量（字符串金额乘法，整数分域运算，杜绝浮点漂移）；非法单价返回 null。 */
export function multiplyPrice(price: string, qty: number): string | null {
  const cents = priceToCents(price)
  if (cents === null || !Number.isInteger(qty) || qty < 0) return null
  return centsToPrice(cents * BigInt(qty))
}
