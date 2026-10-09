// Vitest 全局 setup：
// - 注册 @testing-library/jest-dom 的扩展断言（toBeInTheDocument 等）；
// - jsdom 缺少 window.matchMedia，为依赖媒体查询的组件/库提供最小实现；
// - 每个用例后卸载 React 树，避免跨用例污染。
import "@testing-library/jest-dom/vitest"

import { cleanup } from "@testing-library/react"
import { afterEach } from "vitest"

// matchMedia polyfill（jsdom 未实现；任何读取系统偏好媒体的代码都会用到）
Object.defineProperty(window, "matchMedia", {
  writable: true,
  value: (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  }),
})

afterEach(() => {
  cleanup()
})
