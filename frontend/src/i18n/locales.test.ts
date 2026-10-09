// locale 完整性测试：中英文 key 集合必须完全一致、叶子值必须为非空字符串。
// 缺 key 或空值会让界面直接显示 key 原文 / 空白（与 i18n 冒烟测试互补）。
import { describe, expect, it } from "vitest"

import enUS from "./locales/en-US.json"
import zhCN from "./locales/zh-CN.json"

/** 递归收集全部叶子键路径（"a.b.c" 形式）。 */
function collectLeafKeys(obj: unknown, prefix = ""): string[] {
  const keys: string[] = []
  for (const [k, v] of Object.entries(obj as Record<string, unknown>)) {
    const path = prefix ? `${prefix}.${k}` : k
    if (v !== null && typeof v === "object") {
      keys.push(...collectLeafKeys(v, path))
    } else {
      keys.push(path)
    }
  }
  return keys
}

/** 按路径取叶子值。 */
function getPath(obj: unknown, path: string): unknown {
  let cur = obj as Record<string, unknown>
  for (const part of path.split(".")) {
    cur = cur[part] as Record<string, unknown>
  }
  return cur
}

const zhKeys = collectLeafKeys(zhCN).sort()
const enKeys = collectLeafKeys(enUS).sort()

describe("locale 完整性", () => {
  it("zh-CN 与 en-US 的 key 集合完全一致", () => {
    const missingInEn = zhKeys.filter((k) => !enKeys.includes(k))
    const missingInZh = enKeys.filter((k) => !zhKeys.includes(k))
    expect(missingInEn, "en-US 缺失的 key").toEqual([])
    expect(missingInZh, "zh-CN 缺失的 key").toEqual([])
    expect(zhKeys).toEqual(enKeys)
  })

  it("zh-CN 所有叶子值都是非空字符串", () => {
    for (const k of zhKeys) {
      const v = getPath(zhCN, k)
      expect(typeof v, `zh-CN 的 ${k} 应为字符串`).toBe("string")
      expect((v as string).trim(), `zh-CN 的 ${k} 不应为空`).not.toBe("")
    }
  })

  it("en-US 所有叶子值都是非空字符串", () => {
    for (const k of enKeys) {
      const v = getPath(enUS, k)
      expect(typeof v, `en-US 的 ${k} 应为字符串`).toBe("string")
      expect((v as string).trim(), `en-US 的 ${k} 不应为空`).not.toBe("")
    }
  })

  it("key 数量在合理规模（防误删整个命名空间）", () => {
    expect(zhKeys.length).toBeGreaterThan(100)
  })
})
