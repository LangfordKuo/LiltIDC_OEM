// Vitest 配置：合并 vite.config.ts（复用 @ 别名与 react 插件），jsdom 环境。
// `npm run test` = `vitest run`（一次跑完退出，非 watch）。
import { defineConfig, mergeConfig } from "vitest/config"

import viteConfig from "./vite.config.ts"

export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      environment: "jsdom",
      setupFiles: ["./src/test/setup.ts"],
      include: ["src/**/*.test.{ts,tsx}"],
      // 每个测试文件后自动还原 mock 与全局 stub（fetch 等），避免用例间串扰
      restoreMocks: true,
      unstubGlobals: true,
    },
  }),
)
