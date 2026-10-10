// 余额与流水（批 3-β，需会员登录；契约 14.3）：
// 余额卡片 + 流水分页表（类型/带符号金额/变动后余额/关联/备注/时间）。
// 充值入口本批为「联系客服人工充值」（易支付在线充值留批 4）。
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { Loader2, Wallet } from "lucide-react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { api, type BalanceTransactionView } from "@/lib/api"
import { errorMessage } from "@/lib/error"
import { cn } from "@/lib/utils"

const PAGE_SIZE = 20

export default function BalancePage() {
  const { t } = useTranslation()
  const [credit, setCredit] = useState<string | null>(null)
  const [list, setList] = useState<BalanceTransactionView[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)

  useEffect(() => {
    let alive = true
    api.client
      .balance({ page, page_size: PAGE_SIZE })
      .then((data) => {
        if (!alive) return
        setCredit(data.credit)
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
  }, [page, reloadKey, t])

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  return (
    <div className="space-y-6">
      <div>
        <h1 className="flex items-center gap-2 text-2xl font-bold tracking-tight">
          <Wallet className="size-6" />
          {t("balance.title")}
        </h1>
      </div>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            {t("balance.currentCredit")}
          </CardTitle>
        </CardHeader>
        <CardContent className="flex flex-wrap items-end justify-between gap-3">
          <span className="text-4xl font-bold tracking-tight text-primary">
            {credit === null ? "…" : `¥${credit}`}
          </span>
          <p className="text-xs text-muted-foreground">{t("balance.rechargeHint")}</p>
        </CardContent>
      </Card>

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

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">{t("balance.transactions")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("balance.colType")}</TableHead>
                  <TableHead className="text-right">{t("balance.colAmount")}</TableHead>
                  <TableHead className="text-right">{t("balance.colAfter")}</TableHead>
                  <TableHead>{t("balance.colRemark")}</TableHead>
                  <TableHead>{t("balance.colTime")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {loading ? (
                  <TableRow>
                    <TableCell colSpan={5}>
                      <div className="flex items-center justify-center gap-2 py-10 text-muted-foreground">
                        <Loader2 className="size-4 animate-spin" />
                        <span className="text-sm">{t("common.loading")}</span>
                      </div>
                    </TableCell>
                  </TableRow>
                ) : list.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={5} className="py-10 text-center text-muted-foreground">
                      {t("balance.empty")}
                    </TableCell>
                  </TableRow>
                ) : (
                  list.map((tx) => (
                    <TableRow key={tx.id}>
                      <TableCell>{tx.type_text}</TableCell>
                      <TableCell
                        className={cn(
                          "text-right font-medium tabular-nums",
                          tx.amount.startsWith("-") ? "text-destructive" : "text-emerald-600",
                        )}
                      >
                        {tx.amount.startsWith("-") ? "" : "+"}
                        {tx.amount}
                      </TableCell>
                      <TableCell className="text-right tabular-nums text-muted-foreground">
                        {tx.balance_after}
                      </TableCell>
                      <TableCell className="max-w-64 truncate text-sm text-muted-foreground">
                        {tx.remark || "-"}
                      </TableCell>
                      <TableCell className="text-muted-foreground">{tx.created_at}</TableCell>
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
