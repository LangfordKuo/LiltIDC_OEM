// i18n 初始化（docs/architecture.md ADR-007）：
// - 检测顺序：localStorage（键 liltidc_lang）→ 浏览器语言；
// - 浏览器 zh / en 自动落到 zh-CN / en-US；回退语言 zh-CN；
// - 手动切换写入 localStorage 持久化。
//
// ⚠ 关键陷阱（i18next v26，批 1 全站译文失效的根因，勿回退此修复）：
// 开启 nonExplicitSupportedLngs 后，languageUtils.isSupportedCode() 会先把
// 语言码削成裸码再比对（"zh-CN"→"zh"），而 supportedLngs 是 LanguageUtil
// 构造时的快照（运行时改 languageUtils.options 无效）。若 supportedLngs 只列
// 地区码 ["zh-CN","en-US"]，削出的 "zh" 不在其中 → isSupportedCode 恒为
// false → toResolveHierarchy 返回空数组 → 所有 t() 原样吐 key。
// 因此 supportedLngs 必须同时列出【地区码 + 裸码】。
import i18n from "i18next"
import LanguageDetector from "i18next-browser-languagedetector"
import { initReactI18next } from "react-i18next"

import enUS from "./locales/en-US.json"
import zhCN from "./locales/zh-CN.json"

// 导出 init Promise：bootstrap 与测试可 await，确保首帧/断言前 i18n 就绪
export const i18nReady = i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources: {
      "zh-CN": { translation: zhCN },
      "en-US": { translation: enUS },
    },
    // 地区码 + 裸码都要列（裸码供 nonExplicitSupportedLngs 削码后比对命中）
    supportedLngs: ["zh-CN", "en-US", "zh", "en"],
    nonExplicitSupportedLngs: true,
    fallbackLng: "zh-CN",
    interpolation: {
      escapeValue: false, // React 默认已转义
    },
    detection: {
      order: ["localStorage", "navigator"],
      lookupLocalStorage: "liltidc_lang",
      caches: ["localStorage"],
    },
  })

// ---- <html lang> 同步 ----
// 无障碍/SEO/浏览器翻译提示都依赖 documentElement.lang；语言切换时必须跟随。
// 首屏：init 完成后立即按检测结果设置一次；此后：监听 languageChanged 事件。
function applyDocumentLang(lng: string) {
  if (lng) {
    document.documentElement.lang = lng
  }
}

i18n.on("languageChanged", applyDocumentLang)
void i18nReady.then(() => applyDocumentLang(i18n.language))

export default i18n
