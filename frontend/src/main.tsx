// 应用入口：bootstrap 阶段先预取「安装状态 + 品牌信息」再首帧渲染，
// 使路由守卫可以在渲染期同步读取安装状态缓存，避免 useEffect 异步守卫闪回
// （见 docs/architecture.md ADR-010）。
import { StrictMode } from "react"
import { createRoot } from "react-dom/client"

import App from "./App"
import { Toaster } from "@/components/ui/sonner"
import { refreshBrand } from "@/lib/brand"
import { refreshInstallState } from "@/lib/install-state"
import { i18nReady } from "./i18n"

import "./index.css"

async function bootstrap() {
  // 并行预取；失败时各自静默降级（install-state=null 未知态，brand=null 默认样式）。
  // i18nReady 一并等待：确保首帧渲染前语言包就绪，杜绝首帧闪 key。
  await Promise.allSettled([i18nReady, refreshInstallState(), refreshBrand()])

  const el = document.getElementById("root")
  if (!el) {
    throw new Error("找不到 #root 挂载点")
  }
  createRoot(el).render(
    <StrictMode>
      <App />
      <Toaster richColors position="top-center" />
    </StrictMode>,
  )
}

void bootstrap()
