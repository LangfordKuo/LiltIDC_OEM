// 应用外壳：BrowserRouter + 路由树。
// 路由与守卫结构集中在 src/routes/app-routes.tsx（不含 Router，便于测试）。
import { BrowserRouter } from "react-router-dom"

import { AppRoutes } from "@/routes/app-routes"

export default function App() {
  return (
    <BrowserRouter>
      <AppRoutes />
    </BrowserRouter>
  )
}
