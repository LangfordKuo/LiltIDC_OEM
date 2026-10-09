// 仪表盘占位页：欢迎语 + 后续批次预告。
import { useTranslation } from "react-i18next"
import { BarChart3, FileText, ShoppingCart, Users } from "lucide-react"

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { useBrand } from "@/lib/brand"

export default function DashboardPage() {
  const { t } = useTranslation()
  const brand = useBrand()

  // 占位统计卡片（批 2+ 接真实数据）
  const placeholders = [
    { icon: ShoppingCart, key: "nav.orders" },
    { icon: Users, key: "nav.members" },
    { icon: FileText, key: "nav.tickets" },
    { icon: BarChart3, key: "nav.finance" },
  ]

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold tracking-tight">{t("admin.dashboardTitle")}</h1>
        <p className="text-muted-foreground">
          {t("admin.dashboardWelcome", { name: brand?.site_name || "LiltIDC" })}
        </p>
      </div>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {placeholders.map((p) => (
          <Card key={p.key}>
            <CardHeader className="pb-2">
              <CardDescription className="flex items-center gap-2">
                <p.icon className="size-4" />
                {t(p.key)}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <p className="text-3xl font-bold text-muted-foreground/40">--</p>
            </CardContent>
          </Card>
        ))}
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t("admin.dashboardTitle")}</CardTitle>
          <CardDescription>{t("admin.dashboardPlaceholder")}</CardDescription>
        </CardHeader>
      </Card>
    </div>
  )
}
