// 订单详情（批 3-β，需会员登录）：订单信息 + 条目（含开通信息快照）+
// 支付（余额）/取消操作；Paid（开通中）状态每 5 秒自动刷新直至到达终态
// ——开通是异步转发的，前端以轮询呈现进度（与后端轮询上游同节奏）。
import { useEffect, useRef, useState } from "react"
import { Link, useParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { ArrowLeft, Loader2, RefreshCw } from "lucide-react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Separator } from "@/components/ui/separator"
import { api, type OrderView } from "@/lib/api"
import { errorMessage } from "@/lib/error"
import { orderStatusVariant } from "@/lib/order-status"

/** 开通中自动刷新间隔（后端轮询上游 10s/次，前端 5s 拉本地状态足够跟手）。 */
const AUTO_REFRESH_MS = 5000

export default function ClientOrderDetailPage() {
  const { t } = useTranslation()
  const { id } = useParams()

  const [order, setOrder] = useState<OrderView | null>(null)
  const [credit, setCredit] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [acting, setActing] = useState(false)
  const [reloadNonce, setReloadNonce] = useState(0)
  const timerRef = useRef<number | null>(null)

  // 非法 ID 在渲染期派生（不进 state——规避 effect 内同步 setState）
  const orderId = Number(id)
  const idValid = Number.isInteger(orderId) && orderId > 0

  // 加载详情 + 余额（setState 全在异步回调；reloadNonce 驱动手动刷新与轮询）
  useEffect(() => {
    if (!idValid) return
    let alive = true
    api.client
      .orderDetail(orderId)
      .then((data) => {
        if (!alive) return
        setOrder(data)
        setError(null)
      })
      .catch((err: unknown) => {
        if (alive) setError(errorMessage(err, t("common.unknownError")))
      })
      .finally(() => {
        if (alive) setLoading(false)
      })
    // 余额（支付面板展示；失败静默——不阻断详情）
    api.client
      .balance({ page: 1, page_size: 1 })
      .then((b) => {
        if (alive) setCredit(b.credit)
      })
      .catch(() => {
        if (alive) setCredit(null)
      })
    return () => {
      alive = false
    }
  }, [idValid, orderId, reloadNonce, t])

  // Paid（开通中）→ 自动轮询刷新，到终态停止（setState 在 interval 回调，异步安全）
  useEffect(() => {
    if (order?.status !== "Paid") return
    timerRef.current = window.setInterval(() => setReloadNonce((n) => n + 1), AUTO_REFRESH_MS)
    return () => {
      if (timerRef.current) window.clearInterval(timerRef.current)
    }
  }, [order?.status])

  async function handlePay() {
    setActing(true)
    try {
      const updated = await api.client.payOrder(orderId, "balance")
      setOrder(updated)
      toast.success(t("orderDetail.payDone"))
      setReloadNonce((n) => n + 1) // 余额已变动，重新拉取
    } catch (err) {
      toast.error(errorMessage(err, t("common.unknownError")))
      setReloadNonce((n) => n + 1)
    } finally {
      setActing(false)
    }
  }

  async function handleCancel() {
    setActing(true)
    try {
      const updated = await api.client.cancelOrder(orderId)
      setOrder(updated)
      toast.success(t("clientOrders.cancelDone"))
    } catch (err) {
      toast.error(errorMessage(err, t("common.unknownError")))
    } finally {
      setActing(false)
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
  if (!idValid || error || !order) {
    return (
      <div className="space-y-4">
        <Alert variant="destructive">
          <AlertDescription>{error || t("clientOrders.notFound")}</AlertDescription>
        </Alert>
        <Button asChild variant="outline">
          <Link to="/orders">
            <ArrowLeft />
            {t("orderDetail.backToList")}
          </Link>
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Button asChild variant="ghost" size="sm" className="-ml-2 text-muted-foreground">
          <Link to="/orders">
            <ArrowLeft />
            {t("orderDetail.backToList")}
          </Link>
        </Button>
        <Button variant="ghost" size="sm" onClick={() => setReloadNonce((n) => n + 1)}>
          <RefreshCw className="size-4" />
          {t("common.refresh")}
        </Button>
      </div>

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="flex flex-wrap items-center gap-3 text-lg">
            <span className="font-mono">{order.order_no}</span>
            <Badge variant={orderStatusVariant(order.status)}>{order.status_text}</Badge>
            {order.status === "Paid" && (
              <span className="flex items-center gap-1 text-sm font-normal text-muted-foreground">
                <Loader2 className="size-4 animate-spin" />
                {t("orderDetail.provisioning")}
              </span>
            )}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-2 text-sm sm:grid-cols-2 lg:grid-cols-4">
            <div>
              <span className="text-muted-foreground">{t("orderDetail.amount")}</span>
              <div className="mt-0.5 text-base font-semibold tabular-nums">¥{order.amount}</div>
            </div>
            <div>
              <span className="text-muted-foreground">{t("orderDetail.payMethod")}</span>
              <div className="mt-0.5">
                {order.pay_method === "balance"
                  ? t("orderDetail.payBalance")
                  : order.pay_method || "-"}
              </div>
            </div>
            <div>
              <span className="text-muted-foreground">{t("orderDetail.payTime")}</span>
              <div className="mt-0.5">{order.pay_time || "-"}</div>
            </div>
            <div>
              <span className="text-muted-foreground">{t("clientOrders.colCreated")}</span>
              <div className="mt-0.5">{order.created_at}</div>
            </div>
          </div>
          {order.remark && (
            <Alert>
              <AlertDescription className="break-all text-xs">{order.remark}</AlertDescription>
            </Alert>
          )}

          {/* 支付面板（仅 Pending） */}
          {order.status === "Pending" && (
            <>
              <Separator />
              <div className="flex flex-wrap items-center justify-between gap-3 rounded-md border bg-muted/40 p-3">
                <div className="text-sm">
                  <div>
                    {t("orderDetail.balanceNow")}:{" "}
                    <span className="font-semibold tabular-nums">
                      {credit === null ? "…" : `¥${credit}`}
                    </span>
                  </div>
                  <div className="text-xs text-muted-foreground">{t("orderDetail.payHint")}</div>
                </div>
                <div className="flex gap-2">
                  <Button variant="outline" disabled={acting} onClick={() => void handleCancel()}>
                    {t("clientOrders.cancel")}
                  </Button>
                  <Button disabled={acting} onClick={() => void handlePay()}>
                    {acting ? <Loader2 className="animate-spin" /> : t("orderDetail.payNow")}
                  </Button>
                </div>
              </div>
            </>
          )}
        </CardContent>
      </Card>

      {/* 条目与开通信息 */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base">{t("orderDetail.items")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {order.items.map((it) => (
            <div key={it.id} className="rounded-md border p-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div>
                  <span className="font-medium">{it.product_name}</span>
                  <span className="ml-2 text-sm text-muted-foreground">
                    {it.cycle_text} × {it.qty}
                  </span>
                </div>
                <div className="flex items-center gap-2">
                  <Badge variant={it.provision_status === "active" ? "default" : "outline"}>
                    {it.provision_status_text}
                  </Badge>
                  <span className="text-sm font-semibold tabular-nums">¥{it.amount}</span>
                </div>
              </div>
              {it.provision_info &&
                (it.provision_info.ip ||
                  it.provision_info.hostname ||
                  it.provision_info.error ||
                  it.provision_info.next_due_date) && (
                  <div className="mt-3 grid gap-x-6 gap-y-1 rounded-md bg-muted/40 p-3 text-sm">
                    {it.provision_info.hostname && (
                      <div className="flex gap-2">
                        <span className="shrink-0 text-muted-foreground">
                          {t("orderDetail.infoHostname")}
                        </span>
                        <span className="break-all font-mono text-xs">
                          {it.provision_info.hostname}
                        </span>
                      </div>
                    )}
                    {it.provision_info.ip && (
                      <div className="flex gap-2">
                        <span className="shrink-0 text-muted-foreground">
                          {t("orderDetail.infoIp")}
                        </span>
                        <span className="font-mono text-xs">{it.provision_info.ip}</span>
                      </div>
                    )}
                    {it.provision_info.username && (
                      <div className="flex gap-2">
                        <span className="shrink-0 text-muted-foreground">
                          {t("orderDetail.infoUsername")}
                        </span>
                        <span className="font-mono text-xs">{it.provision_info.username}</span>
                      </div>
                    )}
                    {it.provision_info.password && (
                      <div className="flex gap-2">
                        <span className="shrink-0 text-muted-foreground">
                          {t("orderDetail.infoPassword")}
                        </span>
                        <span className="font-mono text-xs">{it.provision_info.password}</span>
                      </div>
                    )}
                    {it.provision_info.next_due_date && (
                      <div className="flex gap-2">
                        <span className="shrink-0 text-muted-foreground">
                          {t("orderDetail.infoDueDate")}
                        </span>
                        <span>{it.provision_info.next_due_date}</span>
                      </div>
                    )}
                    {it.provision_info.upstream_order_num && (
                      <div className="flex gap-2">
                        <span className="shrink-0 text-muted-foreground">
                          {t("orderDetail.infoUpstreamOrder")}
                        </span>
                        <span className="font-mono text-xs">
                          {it.provision_info.upstream_order_num}
                        </span>
                      </div>
                    )}
                    {it.provision_info.error && (
                      <div className="flex gap-2 text-destructive">
                        <span className="shrink-0">{t("orderDetail.infoError")}</span>
                        <span className="break-all text-xs">{it.provision_info.error}</span>
                      </div>
                    )}
                  </div>
                )}
            </div>
          ))}
          {order.status === "ProvisionFailed" && (
            <p className="text-sm text-muted-foreground">{t("orderDetail.failedHint")}</p>
          )}
        </CardContent>
      </Card>

      <Button asChild variant="link" size="sm" className="text-muted-foreground">
        <Link to="/orders">{t("orderDetail.backToList")}</Link>
      </Button>
    </div>
  )
}
