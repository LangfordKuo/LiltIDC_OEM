// i18n 冒烟测试（硬性门禁，见 docs/architecture.md ADR-014）。
//
// 防回归目标：批 1 曾因 supportedLngs 只列地区码 + nonExplicitSupportedLngs
// 削码比对，导致 isSupportedCode("zh-CN")===false → toResolveHierarchy 返回
// 空数组 → 全站 t() 原样吐 key，且 tsc/build/lint 三道关全部无法发现。
// 本测试直接断言语言解析链路的每一环 + 真实译文。
import { beforeAll, describe, expect, it } from "vitest"

import i18n, { i18nReady } from "./index"
import enUS from "./locales/en-US.json"
import zhCN from "./locales/zh-CN.json"

// 覆盖四个不同命名空间的代表性 key
const SMOKE_KEYS = ["common.save", "home.tagline", "nav.dashboard", "settings.tabSms"] as const

/** 按 "a.b.c" 路径从 locale 对象取叶子值。 */
function getPath(obj: unknown, path: string): string {
  let cur = obj as Record<string, unknown>
  for (const part of path.split(".")) {
    cur = cur[part] as Record<string, unknown>
  }
  return cur as unknown as string
}

beforeAll(async () => {
  await i18nReady
  // jsdom 的 navigator.language 是 en-US，显式固定为中文再断言
  await i18n.changeLanguage("zh-CN")
})

describe("i18n 语言解析链路（批 1 P0 缺陷根因位点）", () => {
  it('languageUtils.isSupportedCode("zh-CN") 必须为 true', () => {
    expect(i18n.services.languageUtils.isSupportedCode("zh-CN")).toBe(true)
  })

  it('languageUtils.isSupportedCode("en-US") 必须为 true', () => {
    expect(i18n.services.languageUtils.isSupportedCode("en-US")).toBe(true)
  })

  it('toResolveHierarchy("zh-CN") 非空且包含 zh-CN', () => {
    const hierarchy = i18n.services.languageUtils.toResolveHierarchy("zh-CN")
    expect(hierarchy.length).toBeGreaterThan(0)
    expect(hierarchy).toContain("zh-CN")
  })
})

describe("i18n 冒烟：t() 返回真实译文而非 key", () => {
  it("中文态：四个代表 key 均等于 zh-CN.json 的译文", () => {
    for (const key of SMOKE_KEYS) {
      const value = i18n.t(key)
      expect(value, `t("${key}") 不应等于 key 本身`).not.toBe(key)
      expect(value).toBe(getPath(zhCN, key))
    }
  })

  it("changeLanguage('en-US') 后：同批 key 返回英文译文且与中文不同", async () => {
    await i18n.changeLanguage("en-US")
    expect(i18n.resolvedLanguage).toBe("en-US")
    for (const key of SMOKE_KEYS) {
      const value = i18n.t(key)
      expect(value, `t("${key}") 不应等于 key 本身`).not.toBe(key)
      expect(value).toBe(getPath(enUS, key))
      expect(value, `t("${key}") 中英文译文不应相同`).not.toBe(getPath(zhCN, key))
    }
  })

  it("切回 zh-CN 后恢复中文译文", async () => {
    await i18n.changeLanguage("zh-CN")
    expect(i18n.t("home.tagline")).toBe(getPath(zhCN, "home.tagline"))
  })

  it("document.documentElement.lang 跟随语言切换（P1 缺陷回归位点）", async () => {
    await i18n.changeLanguage("en-US")
    expect(document.documentElement.lang).toBe("en-US")
    await i18n.changeLanguage("zh-CN")
    expect(document.documentElement.lang).toBe("zh-CN")
  })

  it("带插值的 key 正常渲染变量", () => {
    expect(i18n.t("home.poweredBy", { name: "LiltIDC_OEM" })).toContain("LiltIDC_OEM")
    expect(i18n.t("home.poweredBy", { name: "LiltIDC_OEM" })).not.toContain("{{name}}")
  })
})
