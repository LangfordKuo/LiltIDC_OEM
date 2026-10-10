// 商城列表页（批 3-β，公开可浏览）：本地已上架商品按分组展示，
// 卡片含名称/描述纯文本摘要（sanitize 后提取）/起价/各周期售价/库存态。
import { useEffect, useState } from "react"
import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { Loader2, PackageOpen, ShoppingBag } from "lucide-react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { api, type StoreGroup } from "@/lib/api"
import { errorMessage } from "@/lib/error"
import { htmlExcerpt } from "@/lib/html"
import { sortedPriceEntries } from "@/lib/prices"

export default function StorePage() {
  const { t } = useTranslation()
  const [groups, setGroups] = useState<StoreGroup[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)

  useEffect(() => {
    let alive = true
    api.store
      .products()
      .then((data) => {
        if (!alive) return
        setGroups(data.groups)
        setError(null)
      })
      .catch((err: unknown) => {
        if (alive) setError(errorMessage(err, t("common.unknownError")))
      })
      .finally(() => {
        if (alive) setLoading(false)
      })
    return () => {
      alive = false
    }
  }, [reloadKey, t])

  return (
    <div className="space-y-8">
      <div>
        <h1 className="flex items-center gap-2 text-2xl font-bold tracking-tight">
          <ShoppingBag className="size-6" />
          {t("store.title")}
        </h1>
        <p className="text-sm text-muted-foreground">{t("store.subtitle")}</p>
      </div>

      {error && (
        <Alert variant="destructive">
          <AlertDescription className="flex items-center justify-between gap-3">
            <span className="break-all">{error}</span>
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                setLoading(true)
                setReloadKey((k) => k + 1)
              }}
            >
              {t("common.retry")}
            </Button>
          </AlertDescription>
        </Alert>
      )}

      {loading ? (
        <div className="flex items-center justify-center gap-2 py-20 text-muted-foreground">
          <Loader2 className="size-5 animate-spin" />
          <span>{t("common.loading")}</span>
        </div>
      ) : groups.length === 0 ? (
        <div className="flex flex-col items-center gap-3 py-20 text-muted-foreground">
          <PackageOpen className="size-10" />
          <p>{t("store.empty")}</p>
        </div>
      ) : (
        groups.map((group) => (
          <section key={group.id} className="space-y-3">
            <h2 className="text-lg font-semibold">{group.name}</h2>
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {group.products.map((p) => {
                const excerpt = htmlExcerpt(p.description, 72)
                const cycles = sortedPriceEntries(p.sale_prices).slice(0, 4)
                return (
                  <Card key={p.id} className="flex flex-col transition-shadow hover:shadow-md">
                    <CardHeader className="pb-2">
                      <CardTitle className="flex items-center justify-between gap-2 text-base">
                        <span className="truncate">{p.name}</span>
                        {!p.in_stock && (
                          <Badge variant="secondary" className="shrink-0">
                            {t("store.outOfStock")}
                          </Badge>
                        )}
                      </CardTitle>
                    </CardHeader>
                    <CardContent className="flex flex-1 flex-col gap-3">
                      <p className="line-clamp-2 min-h-10 text-sm text-muted-foreground">
                        {excerpt || t("store.noDescription")}
                      </p>
                      <div className="flex flex-wrap gap-1">
                        {cycles.map(([cycle, price]) => (
                          <Badge key={cycle} variant="outline" className="font-normal">
                            {t(`products.cycles.${cycle}`)} ¥{price}
                          </Badge>
                        ))}
                      </div>
                      <div className="mt-auto flex items-center justify-between">
                        <span className="text-sm text-muted-foreground">
                          {p.price_from
                            ? t("store.priceFrom", { price: p.price_from })
                            : t("store.noPrice")}
                        </span>
                        <Button asChild size="sm" disabled={!p.in_stock}>
                          <Link to={`/store/${p.id}`}>{t("store.viewDetail")}</Link>
                        </Button>
                      </div>
                    </CardContent>
                  </Card>
                )
              })}
            </div>
          </section>
        ))
      )}
    </div>
  )
}
