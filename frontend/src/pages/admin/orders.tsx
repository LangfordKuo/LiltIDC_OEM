// 后台订单管理页（批 3-β，替换占位页；契约 13.5）：
// 列表（状态筛选/订单号与会员名搜索/分页）+ 详情 Dialog
//（条目、开通信息快照、关联余额流水）+ ProvisionFailed 的「重试开通/取消并退款」。
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Loader2, MoreHorizontal, Receipt, Search } from "lucide-react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
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
import { api, type OrderAdminDetail, type OrderView } from "@/lib/api"
import { errorMessage } from "@/lib/error"
import { orderStatusVariant } from "@/lib/order-status"

const PAGE_SIZE = 20

export default function AdminOrdersPage() {
  const { t } = useTranslation()
  const [list, setList] = useState<OrderView[]>([])
  const [total, setTotal] = useState(0)
  const [keywordInput, setKeywordInput] = useState("")
  const [keyword, setKeyword] = useState("")
  const [statusFilter, setStatusFilter] = useState("all")
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)

  // 详情对话框（detail=null 即加载态；打开时在事件处理器里重置，规避 effect 内同步 setState）
  const [detailId, setDetailId] = useState(0)
  const [detail, setDetail] = useState<OrderAdminDetail | null>(null)
  const [detailNonce, setDetailNonce] = useState(0) // 操作后强制重拉详情
  const [acting, setActing] = useState(false)

  function openDetail(id: number) {
    setDetail(null)
    setDetailId(id)
  }

  useEffect(() => {
    let alive = true
    api.admin.orders
      .list({
        status: statusFilter === "all" ? undefined : statusFilter,
        keyword: keyword || undefined,
        page,
        page_size: PAGE_SIZE,
      })
      .then((data) => {
        if (!alive) return
        setList(data.list)
        setTotal(data.total)
        setLoadError(null)
      })
      .catch((err: unknown) => {
        if (alive) setLoadError(errorMessage(err, t("common.unknownError")))
      })
      .finally(() => {
        if (alive) setLoading(false)
      })
    return () => {
      alive = false
    }
  }, [keyword, statusFilter, page, reloadKey, t])

  // 打开详情时拉取完整数据（含关联流水）；setState 全在异步回调
  useEffect(() => {
    if (!detailId) return
    let alive = true
    api.admin.orders
      .detail(detailId)
      .then((d) => {
        if (alive) setDetail(d)
      })
      .catch((err: unknown) => {
        if (alive) toast.error(errorMessage(err, t("common.unknownError")))
      })
    return () => {
      alive = false
    }
  }, [detailId, detailNonce, t])

  function reload() {
    setLoading(true)
    setReloadKey((k) => k + 1)
  }

  function handleSearch() {
    setKeyword(keywordInput.trim())
    setPage(1)
  }

  async function handleRetry(o: { id: number; order_no: string }) {
    setActing(true)
    try {
      await api.admin.orders.retryProvision(o.id)
      toast.success(t("adminOrders.retryDone", { orderNo: o.order_no }))
      reload()
      setDetailNonce((n) => n + 1) // 详情对话框开着时同步刷新
    } catch (err) {
      toast.error(errorMessage(err, t("common.unknownError")))
    } finally {
      setActing(false)
    }
  }

  async function handleCancelRefund(o: { id: number; order_no: string }) {
    setActing(true)
    try {
      await api.admin.orders.cancelRefund(o.id)
      toast.success(t("adminOrders.refundDone", { orderNo: o.order_no }))
      reload()
      setDetailNonce((n) => n + 1) // 详情对话框开着时同步刷新
    } catch (err) {
      toast.error(errorMessage(err, t("common.unknownError")))
    } finally {
      setActing(false)
    }
  }

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  return (
    <div className="space-y-6">
      <div>
        <h1 className="flex items-center gap-2 text-2xl font-bold tracking-tight">
          <Receipt className="size-6" />
          {t("adminOrders.title")}
        </h1>
        <p className="text-sm text-muted-foreground">{t("adminOrders.total", { total })}</p>
      </div>

      <Card>
        <CardContent className="space-y-4 pt-6">
          {loadError && (
            <Alert variant="destructive">
              <AlertDescription className="flex items-center justify-between gap-3">
                <span className="break-all">{loadError}</span>
                <Button size="sm" variant="outline" onClick={reload}>
                  {t("common.retry")}
                </Button>
              </AlertDescription>
            </Alert>
          )}

          <div className="flex flex-wrap items-center gap-2">
            <div className="relative w-64">
              <Search className="absolute left-2.5 top-2.5 size-4 text-muted-foreground" />
              <Input
                value={keywordInput}
                onChange={(e) => setKeywordInput(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && handleSearch()}
                placeholder={t("adminOrders.searchPlaceholder")}
                className="pl-8"
                aria-label={t("adminOrders.searchPlaceholder")}
              />
            </div>
            <Button variant="outline" onClick={handleSearch}>
              {t("adminOrders.search")}
            </Button>
            <Select
              value={statusFilter}
              onValueChange={(v) => {
                setStatusFilter(v)
                setPage(1)
              }}
            >
              <SelectTrigger className="w-48" aria-label={t("clientOrders.colStatus")}>
                <SelectValue placeholder={t("clientOrders.colStatus")} />
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
                  <TableHead>{t("adminOrders.colClient")}</TableHead>
                  <TableHead>{t("clientOrders.colProduct")}</TableHead>
                  <TableHead className="text-right">{t("clientOrders.colAmount")}</TableHead>
                  <TableHead>{t("clientOrders.colStatus")}</TableHead>
                  <TableHead>{t("clientOrders.colCreated")}</TableHead>
                  <TableHead className="w-16">{t("common.actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {loading ? (
                  <TableRow>
                    <TableCell colSpan={7}>
                      <div className="flex items-center justify-center gap-2 py-10 text-muted-foreground">
                        <Loader2 className="size-4 animate-spin" />
                        <span className="text-sm">{t("common.loading")}</span>
                      </div>
                    </TableCell>
                  </TableRow>
                ) : list.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={7} className="py-10 text-center text-muted-foreground">
                      {t("adminOrders.empty")}
                    </TableCell>
                  </TableRow>
                ) : (
                  list.map((o) => (
                    <TableRow key={o.id}>
                      <TableCell className="font-mono text-sm font-medium">{o.order_no}</TableCell>
                      <TableCell className="text-sm">
                        {o.client_username || `#${o.client_id}`}
                      </TableCell>
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
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => openDetail(o.id)}
                          >
                            {t("clientOrders.detail")}
                          </Button>
                          <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                              <Button
                                variant="ghost"
                                size="icon"
                                aria-label={t("common.actions")}
                              >
                                <MoreHorizontal />
                              </Button>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent align="end">
                              <DropdownMenuItem
                                disabled={o.status !== "ProvisionFailed" || acting}
                                onClick={() => void handleRetry(o)}
                              >
                                {t("adminOrders.retry")}
                              </DropdownMenuItem>
                              <DropdownMenuItem
                                className="text-destructive"
                                disabled={o.status !== "ProvisionFailed" || acting}
                                onClick={() => void handleCancelRefund(o)}
                              >
                                {t("adminOrders.cancelRefund")}
                              </DropdownMenuItem>
                            </DropdownMenuContent>
                          </DropdownMenu>
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

      {/* 详情对话框 */}
      <Dialog open={detailId !== 0} onOpenChange={(open) => !open && setDetailId(0)}>
        <DialogContent className="max-h-[85vh] max-w-2xl overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{t("adminOrders.detailTitle")}</DialogTitle>
            <DialogDescription className="font-mono">
              {detail?.order_no || `#${detailId}`}
            </DialogDescription>
          </DialogHeader>
          {!detail ? (
            <div className="flex items-center justify-center gap-2 py-10 text-muted-foreground">
              <Loader2 className="size-4 animate-spin" />
              <span className="text-sm">{t("common.loading")}</span>
            </div>
          ) : (
            <div className="space-y-4 text-sm">
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
                <div>
                  <div className="text-xs text-muted-foreground">{t("clientOrders.colStatus")}</div>
                  <Badge variant={orderStatusVariant(detail.status)}>{detail.status_text}</Badge>
                </div>
                <div>
                  <div className="text-xs text-muted-foreground">{t("adminOrders.colClient")}</div>
                  <div>{detail.client_username || `#${detail.client_id}`}</div>
                </div>
                <div>
                  <div className="text-xs text-muted-foreground">
                    {t("clientOrders.colAmount")}
                  </div>
                  <div className="tabular-nums">¥{detail.amount}</div>
                </div>
                <div>
                  <div className="text-xs text-muted-foreground">
                    {t("adminOrders.balanceUsed")}
                  </div>
                  <div className="tabular-nums">¥{detail.balance_used}</div>
                </div>
                <div>
                  <div className="text-xs text-muted-foreground">
                    {t("orderDetail.payTime")}
                  </div>
                  <div>{detail.pay_time || "-"}</div>
                </div>
                <div>
                  <div className="text-xs text-muted-foreground">
                    {t("clientOrders.colCreated")}
                  </div>
                  <div>{detail.created_at}</div>
                </div>
              </div>

              {detail.remark && (
                <Alert>
                  <AlertDescription className="break-all text-xs">{detail.remark}</AlertDescription>
                </Alert>
              )}

              <div>
                <div className="mb-1 text-xs font-medium text-muted-foreground">
                  {t("orderDetail.items")}
                </div>
                {detail.items.map((it) => (
                  <div key={it.id} className="mb-2 rounded-md border p-2">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <span className="font-medium">
                        {it.product_name}
                        <span className="ml-2 text-xs text-muted-foreground">
                          pid={it.upstream_pid} · {it.cycle} × {it.qty}
                        </span>
                      </span>
                      <span className="tabular-nums">
                        ¥{it.unit_price} → ¥{it.amount}
                      </span>
                    </div>
                    <div className="mt-1 flex flex-wrap gap-x-4 gap-y-0.5 text-xs text-muted-foreground">
                      <span>
                        {t("adminOrders.provisionStatus")}: {it.provision_status_text}
                      </span>
                      {it.upstream_order_id > 0 && (
                        <span>
                          {t("adminOrders.upstreamInvoice")}: #{it.upstream_order_id}
                        </span>
                      )}
                      {it.upstream_host_id > 0 && (
                        <span>
                          {t("adminOrders.upstreamHost")}: #{it.upstream_host_id}
                        </span>
                      )}
                    </div>
                    {it.provision_info && (
                      <pre className="mt-1 max-h-32 overflow-auto rounded bg-muted/50 p-2 text-xs whitespace-pre-wrap">
                        {JSON.stringify(it.provision_info, null, 1)}
                      </pre>
                    )}
                  </div>
                ))}
              </div>

              {detail.transactions.length > 0 && (
                <div>
                  <div className="mb-1 text-xs font-medium text-muted-foreground">
                    {t("adminOrders.relatedTransactions")}
                  </div>
                  <div className="rounded-md border">
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>{t("balance.colType")}</TableHead>
                          <TableHead className="text-right">{t("balance.colAmount")}</TableHead>
                          <TableHead className="text-right">{t("balance.colAfter")}</TableHead>
                          <TableHead>{t("balance.colTime")}</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {detail.transactions.map((tx) => (
                          <TableRow key={tx.id}>
                            <TableCell>{tx.type_text}</TableCell>
                            <TableCell className="text-right tabular-nums">{tx.amount}</TableCell>
                            <TableCell className="text-right tabular-nums">
                              {tx.balance_after}
                            </TableCell>
                            <TableCell className="text-muted-foreground">{tx.created_at}</TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </div>
                </div>
              )}

              {detail.status === "ProvisionFailed" && (
                <div className="flex justify-end gap-2 border-t pt-3">
                  <Button variant="outline" disabled={acting} onClick={() => void handleRetry(detail)}>
                    {t("adminOrders.retry")}
                  </Button>
                  <Button
                    variant="destructive"
                    disabled={acting}
                    onClick={() => void handleCancelRefund(detail)}
                  >
                    {t("adminOrders.cancelRefund")}
                  </Button>
                </div>
              )}
            </div>
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}
