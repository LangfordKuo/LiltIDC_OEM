// HTML 处理工具（批 3-β）：上游商品 description 是 HTML 原文，
// 直接 dangerouslySetInnerHTML 会有存储型 XSS 风险——渲染前必须 sanitize。
import DOMPurify from "dompurify"

/**
 * 消毒 HTML（白名单标签/属性，剥离 script/事件处理器/javascript: URL）。
 * 用于商品详情页 dangerouslySetInnerHTML 前的强制处理。
 */
export function sanitizeHtml(dirty: string): string {
  return DOMPurify.sanitize(dirty, {
    // 商品描述只需要排版标签；禁 target/外链属性注入
    ALLOWED_TAGS: [
      "p", "br", "b", "strong", "i", "em", "u", "s", "small", "sub", "sup",
      "ul", "ol", "li", "dl", "dt", "dd",
      "h1", "h2", "h3", "h4", "h5", "h6",
      "blockquote", "pre", "code", "hr", "span", "div", "table",
      "thead", "tbody", "tr", "th", "td",
    ],
    ALLOWED_ATTR: ["colspan", "rowspan", "width", "height", "align"],
  })
}

/** 提取 HTML 的纯文本摘要（列表卡片用）：去标签 → 压缩空白 → 截断加省略号。 */
export function htmlExcerpt(html: string, maxLen = 80): string {
  const div = document.createElement("div")
  div.innerHTML = sanitizeHtml(html)
  const text = (div.textContent || "").replace(/\s+/g, " ").trim()
  if (text.length <= maxLen) return text
  return `${text.slice(0, maxLen)}…`
}
