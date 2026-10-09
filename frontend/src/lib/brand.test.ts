// 品牌主色应用逻辑测试（ADR-008），含「语言切换后主色仍生效」的复验。
import { beforeAll, describe, expect, it } from "vitest"

import i18n, { i18nReady } from "@/i18n"
import { applyBrand } from "./brand"

beforeAll(async () => {
  await i18nReady
  await i18n.changeLanguage("zh-CN")
})

describe("applyBrand 写入 CSS 变量", () => {
  it("主色写入 --primary/--ring，前景按亮度自动选择", () => {
    applyBrand({ site_name: "测试站", logo: "", primary_color: "#0EA5E9" })
    const style = document.documentElement.style
    expect(style.getPropertyValue("--primary")).toBe("#0ea5e9") // 归一化为小写
    expect(style.getPropertyValue("--ring")).toBe("#0ea5e9")
    // #0ea5e9 相对亮度 < 0.45 → 白色前景
    expect(style.getPropertyValue("--primary-foreground")).toBe("#ffffff")
    expect(document.title).toBe("测试站")
  })

  it("浅色主色配深色前景", () => {
    applyBrand({ site_name: "X", logo: "", primary_color: "#facc15" })
    expect(document.documentElement.style.getPropertyValue("--primary-foreground")).toBe("#18181b")
  })

  it("非法主色不写入（保留原值）", () => {
    applyBrand({ site_name: "X", logo: "", primary_color: "#0ea5e9" })
    applyBrand({ site_name: "X", logo: "", primary_color: "not-a-color" })
    expect(document.documentElement.style.getPropertyValue("--primary")).toBe("#0ea5e9")
  })
})

describe("语言切换后品牌主色仍生效（返工复验项）", () => {
  it("changeLanguage('en-US') 不触碰 CSS 变量", async () => {
    applyBrand({ site_name: "岭云互联测试站", logo: "", primary_color: "#0ea5e9" })
    await i18n.changeLanguage("en-US")
    const style = document.documentElement.style
    expect(style.getPropertyValue("--primary")).toBe("#0ea5e9")
    expect(style.getPropertyValue("--primary-foreground")).toBe("#ffffff")
    expect(style.getPropertyValue("--ring")).toBe("#0ea5e9")
    await i18n.changeLanguage("zh-CN")
    expect(style.getPropertyValue("--primary")).toBe("#0ea5e9")
  })
})
