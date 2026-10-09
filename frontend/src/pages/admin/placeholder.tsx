// 通用空白占位页：后续批次交付前，商品/订单/会员/工单/财务菜单的内容区。
import { useTranslation } from "react-i18next"
import { Construction } from "lucide-react"

import { Card, CardContent } from "@/components/ui/card"

export default function PlaceholderPage({ titleKey }: { titleKey: string }) {
  const { t } = useTranslation()
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold tracking-tight">{t(titleKey)}</h1>
      <Card>
        <CardContent className="flex flex-col items-center justify-center gap-3 py-24 text-center">
          <Construction className="size-12 text-muted-foreground/40" />
          <p className="text-muted-foreground">{t("admin.placeholderDescription")}</p>
        </CardContent>
      </Card>
    </div>
  )
}
