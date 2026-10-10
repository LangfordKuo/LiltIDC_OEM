// 我的订单（批 3-β，需会员登录；契约 13.3）：状态筛选 + 分页列表，
// 行操作：去支付（Pending）/ 取消（Pending）/ 详情。
import { useEffect, useState } from "react"
import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Loader2, Receipt } from "lucide-react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { api, type OrderView } from "@/lib/api"
import { errorMessage } from "@/lib/error"
import { orderStatusVariant } from "@/lib/order-status"

const PAGE_SIZE = 20

export default function ClientOrdersPage() {
  const { t } = useTranslation()
  const [list, setList] = useState<OrderView[]>([])
  const [total, setTotal] = useState(0)
  const [statusFilter, setStatusFilter] = useState("all")
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)
  const [actingId, setActingId] = useState(0)

  useEffect(() => {
    let alive = true
    api.client
      .orders({
        status: statusFilter === "all" ? undefined : statusFilter,
        page,
        page_size: PAGE_SIZE,
      })
      .then((data) => {
        if (!alive) return
        setList(data.list)
        setTotal(data.total)
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
  }, [statusFilter, page, reloadKey, t])

  function reload() {
    setLoading(true)
    setReloadKey((k) => k + 1)
  }

  async function handleCancel(o: OrderView) {
    setActingId(o.id)
    try {
      await api.client.cancelOrder(o.id)
      toast.success(t("clientOrders.cancelDone"))
      reload()
    } catch (err) {
      toast.error(errorMessage(err, t("common.unknownError")))
    } finally {
      setActingId(0)
    }
  }

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  return (
    <div className="space-y-6">
      <div>
        <h1 className="flex items-center gap-2 text-2xl font-bold tracking-tight">
          <Receipt className="size-6" />
          {t("clientOrders.title")}
        </h1>
        <p className="text-sm text-muted-foreground">{t("clientOrders.total", { total })}</p>
      </div>

      <Card>
        <CardContent className="space-y-4 pt-6">
          {error && (
            <Alert variant="destructive">
              <AlertDescription className="flex items-center justify-between gap-3">
                <span className="break-all">{error}</span>
                <Button size="sm" variant="outline" onClick={reload}>
                  {t("common.retry")}
                </Button>
              </AlertDescription>
            </Alert>
          )}

          <div className="flex flex-wrap items-center gap-2">
            <Select
              value={statusFilter}
              onValueChange={(v) => {
                setStatusFilter(v)
                setPage(1)
              }}
            >
              <SelectTrigger className="w-52" aria-label={t("clientOrders.filterStatus")}>
                <SelectValue placeholder={t("clientOrders.filterStatus")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t("clientOrders.statusAll")}</SelectItem>
                <SelectItem value="Pending">{t("clientOrders.statusPending")}</SelectItem>
                <SelectItem value="Paid">{t("clientOrders.statusPaid")}</SelectItem>
                <SelectItem value="Active">{t("clientOrders.statusActive")}</SelectItem>
                <SelectItem value="ProvisionFailed">
                  {t("clientOrders.statusProvisionFailed")}
                </SelectItem>
                <SelectItem value="Cancelled">{t("clientOrders.statusCancelled")}</SelectItem>
                <SelectItem value="Refunded">{t("clientOrders.statusRefunded")}</SelectItem>
              </SelectContent>
            </Select>
            <Button variant="ghost" size="icon" onClick={reload} aria-label={t("common.refresh")}>
              <Loader2 className={loading ? "animate-spin" : ""} />
            </Button>
          </div>

          <div className="rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("clientOrders.colOrderNo")}</TableHead>
                  <TableHead>{t("clientOrders.colProduct")}</TableHead>
                  <TableHead className="text-right">{t("clientOrders.colAmount")}</TableHead>
                  <TableHead>{t("clientOrders.colStatus")}</TableHead>
                  <TableHead>{t("clientOrders.colCreated")}</TableHead>
                  <TableHead className="w-40">{t("common.actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {loading ? (
                  <TableRow>
                    <TableCell colSpan={6}>
                      <div className="flex items-center justify-center gap-2 py-10 text-muted-foreground">
                        <Loader2 className="size-4 animate-spin" />
                        <span className="text-sm">{t("common.loading")}</span>
                      </div>
                    </TableCell>
                  </TableRow>
                ) : list.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={6} className="py-10 text-center text-muted-foreground">
                      {t("clientOrders.empty")}
                    </TableCell>
                  </TableRow>
                ) : (
                  list.map((o) => (
                    <TableRow key={o.id}>
                      <TableCell className="font-medium">{o.order_no}</TableCell>
                      <TableCell className="max-w-52 truncate text-sm">
                        {o.items.map((it) => `${it.product_name} × ${it.qty}`).join(", ") || "-"}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">¥{o.amount}</TableCell>
                      <TableCell>
                        <Badge variant={orderStatusVariant(o.status)}>{o.status_text}</Badge>
                      </TableCell>
                      <TableCell className="text-muted-foreground">{o.created_at}</TableCell>
                      <TableCell>
                        <div className="flex items-center gap-1">
                          {o.status === "Pending" && (
                            <>
                              <Button asChild size="sm">
                                <Link to={`/orders/${o.id}`}>{t("clientOrders.pay")}</Link>
                              </Button>
                              <Button
                                size="sm"
                                variant="outline"
                                disabled={actingId === o.id}
                                onClick={() => void handleCancel(o)}
                              >
                                {t("clientOrders.cancel")}
                              </Button>
                            </>
                          )}
                          {o.status !== "Pending" && (
                            <Button asChild size="sm" variant="outline">
                              <Link to={`/orders/${o.id}`}>{t("clientOrders.detail")}</Link>
                            </Button>
                          )}
                        </div>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>

          {totalPages > 1 && (
            <div className="flex items-center justify-end gap-2">
              <Button
                variant="outline"
                size="sm"
                disabled={loading || page <= 1}
                onClick={() => setPage((p) => p - 1)}
              >
                {t("upstream.hosts.prevPage")}
              </Button>
              <span className="text-sm text-muted-foreground">
                {t("upstream.hosts.pageInfo", { page })} / {totalPages}
              </span>
              <Button
                variant="outline"
                size="sm"
                disabled={loading || page >= totalPages}
                onClick={() => setPage((p) => p + 1)}
              >
                {t("upstream.hosts.nextPage")}
              </Button>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
