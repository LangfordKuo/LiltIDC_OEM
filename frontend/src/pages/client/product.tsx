// 商品详情页（批 3-β，公开可浏览）：
// 描述 HTML 经 dompurify sanitize 后渲染（上游 description 是 HTML，防存储型 XSS）；
// 周期选择 + 数量 + 「立即购买」：未登录 → 跳会员登录（记录来源），已登录 → 下单 → 订单详情支付。
import { useEffect, useMemo, useState } from "react"
import { Link, useLocation, useNavigate, useParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { ArrowLeft, Loader2, Minus, Plus } from "lucide-react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { api, getClientToken, type StoreProductDetail } from "@/lib/api"
import { errorMessage } from "@/lib/error"
import { sanitizeHtml } from "@/lib/html"
import { multiplyPrice } from "@/lib/prices"

const MAX_QTY = 10 // 与后端 maxOrderQty 一致

export default function StoreProductPage() {
  const { t } = useTranslation()
  const { productId } = useParams()
  const navigate = useNavigate()
  const location = useLocation()

  const [product, setProduct] = useState<StoreProductDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [cycle, setCycle] = useState("")
  const [qty, setQty] = useState(1)
  const [buying, setBuying] = useState(false)

  // 非法 ID 在渲染期派生（不进 state——规避 effect 内同步 setState）
  const parsedId = Number(productId)
  const idValid = Number.isInteger(parsedId) && parsedId > 0

  useEffect(() => {
    if (!idValid) return
    let alive = true
    api.store
      .productDetail(parsedId)
      .then((data) => {
        if (!alive) return
        setProduct(data)
        // 默认选中第一个可售周期
        setCycle((prev) => prev || data.cycles[0] || "")
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
  }, [parsedId, idValid, t])

  // 描述 HTML 必须 sanitize 后才可注入（XSS 防线，ADR 见契约 13.1）
  const safeDescription = useMemo(
    () => (product ? sanitizeHtml(product.description) : ""),
    [product],
  )

  const unitPrice = product && cycle ? (product.sale_prices[cycle] ?? "") : ""

  async function handleBuy() {
    if (!product || !cycle) return
    if (!getClientToken()) {
      // 未登录：跳会员登录并记录来源，登录后回本页
      navigate("/client-login", { state: { from: location } })
      return
    }
    setBuying(true)
    try {
      const order = await api.client.createOrder({
        product_id: product.id,
        cycle,
        qty,
      })
      toast.success(t("product.orderCreated", { orderNo: order.order_no }))
      navigate(`/orders/${order.id}`)
    } catch (err) {
      toast.error(errorMessage(err, t("common.unknownError")))
    } finally {
      setBuying(false)
    }
  }

  if (loading && idValid) {
    return (
      <div className="flex items-center justify-center gap-2 py-20 text-muted-foreground">
        <Loader2 className="size-5 animate-spin" />
        <span>{t("common.loading")}</span>
      </div>
    )
  }
  if (!idValid || error || !product) {
    return (
      <div className="space-y-4">
        <Alert variant="destructive">
          <AlertDescription>{error || t("store.notFound")}</AlertDescription>
        </Alert>
        <Button asChild variant="outline">
          <Link to="/store">
            <ArrowLeft />
            {t("store.backToList")}
          </Link>
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <Button asChild variant="ghost" size="sm" className="-ml-2 text-muted-foreground">
        <Link to="/store">
          <ArrowLeft />
          {t("store.backToList")}
        </Link>
      </Button>

      <div className="grid gap-6 lg:grid-cols-[1fr_360px]">
        {/* 左：名称 + 描述（sanitize 后渲染） */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-xl">
              {product.name}
              {!product.in_stock && <Badge variant="secondary">{t("store.outOfStock")}</Badge>}
            </CardTitle>
            <p className="text-sm text-muted-foreground">
              {product.group_name}
              {product.stock_control && product.in_stock
                ? ` · ${t("store.stockLeft", { qty: product.stock_qty })}`
                : ""}
            </p>
          </CardHeader>
          <CardContent>
            {safeDescription ? (
              <div
                className="prose-sm max-w-none text-sm leading-6 text-foreground [&_li]:ml-4 [&_li]:list-disc"
                // 已经过 dompurify 白名单消毒（lib/html.ts），此处注入是安全的
                dangerouslySetInnerHTML={{ __html: safeDescription }}
              />
            ) : (
              <p className="text-sm text-muted-foreground">{t("store.noDescription")}</p>
            )}
          </CardContent>
        </Card>

        {/* 右：购买面板 */}
        <Card className="h-fit">
          <CardHeader>
            <CardTitle className="text-base">{t("product.buyPanel")}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label>{t("product.cycle")}</Label>
              <div className="grid grid-cols-2 gap-2">
                {product.cycles.map((c) => (
                  <Button
                    key={c}
                    type="button"
                    size="sm"
                    variant={cycle === c ? "default" : "outline"}
                    onClick={() => setCycle(c)}
                    className="flex-col items-start gap-0.5 py-2"
                  >
                    <span className="text-xs">{t(`products.cycles.${c}`)}</span>
                    <span className="text-sm font-semibold">¥{product.sale_prices[c]}</span>
                  </Button>
                ))}
              </div>
            </div>

            <div className="space-y-2">
              <Label htmlFor="qty">{t("product.qty")}</Label>
              <div className="flex items-center gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  aria-label={t("product.qtyDecrease")}
                  disabled={qty <= 1}
                  onClick={() => setQty((q) => Math.max(1, q - 1))}
                >
                  <Minus />
                </Button>
                <Input
                  id="qty"
                  type="number"
                  min={1}
                  max={MAX_QTY}
                  value={qty}
                  onChange={(e) => {
                    const n = Number(e.target.value)
                    setQty(Number.isFinite(n) ? Math.min(MAX_QTY, Math.max(1, n)) : 1)
                  }}
                  className="w-20 text-center"
                />
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  aria-label={t("product.qtyIncrease")}
                  disabled={qty >= MAX_QTY}
                  onClick={() => setQty((q) => Math.min(MAX_QTY, q + 1))}
                >
                  <Plus />
                </Button>
              </div>
            </div>

            <div className="rounded-md border bg-muted/40 p-3 text-sm">
              <div className="flex justify-between">
                <span className="text-muted-foreground">{t("product.unitPrice")}</span>
                <span>¥{unitPrice || "-"}</span>
              </div>
              <div className="mt-1 flex justify-between font-semibold">
                <span>{t("product.total")}</span>
                <span className="text-primary">
                  ¥{multiplyPrice(unitPrice, qty) ?? "-"}
                </span>
              </div>
              <p className="mt-1 text-xs text-muted-foreground">{t("product.totalHint")}</p>
            </div>

            <Button
              className="w-full"
              disabled={!cycle || !product.in_stock || buying}
              onClick={() => void handleBuy()}
            >
              {buying && <Loader2 className="animate-spin" />}
              {product.in_stock ? t("product.buyNow") : t("store.outOfStock")}
            </Button>
            <p className="text-center text-xs text-muted-foreground">{t("product.payHint")}</p>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
