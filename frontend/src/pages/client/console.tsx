// 我的主机（批 3-β，需会员登录）：本批展示 状态/IP/到期时间/所属商品
//（数据源 hosts 表：开通轮询回填 + 上游 host/sync 推送 upsert）。
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { Loader2, Server } from "lucide-react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { api, type ClientHostView } from "@/lib/api"
import { errorMessage } from "@/lib/error"

const PAGE_SIZE = 20

/** 已知上游主机状态枚举（i18n 键覆盖全集；未知值原样展示防吐 key）。 */
const KNOWN_HOST_STATUSES = ["Pending", "Active", "Suspended", "Cancelled", "Fraud", "Deleted"]

/** 主机状态 → Badge 视觉映射（上游 domainstatus 枚举）。 */
function statusVariant(status: string): "default" | "secondary" | "destructive" | "outline" {
  switch (status) {
    case "Active":
      return "default"
    case "Pending":
      return "secondary"
    case "Suspended":
    case "Fraud":
      return "destructive"
    default:
      return "outline" // Cancelled/Deleted 等
  }
}

export default function ConsolePage() {
  const { t } = useTranslation()
  const [list, setList] = useState<ClientHostView[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)

  useEffect(() => {
    let alive = true
    api.client
      .hosts({ page, page_size: PAGE_SIZE })
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
  }, [page, reloadKey, t])

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  return (
    <div className="space-y-6">
      <div>
        <h1 className="flex items-center gap-2 text-2xl font-bold tracking-tight">
          <Server className="size-6" />
          {t("console.title")}
        </h1>
        <p className="text-sm text-muted-foreground">{t("console.total", { total })}</p>
      </div>

      <Card>
        <CardContent className="space-y-4 pt-6">
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
          <div className="rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("console.colProduct")}</TableHead>
                  <TableHead>{t("console.colStatus")}</TableHead>
                  <TableHead>{t("console.colIp")}</TableHead>
                  <TableHead>{t("console.colDueDate")}</TableHead>
                  <TableHead>{t("console.colSynced")}</TableHead>
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
                      {t("console.empty")}
                    </TableCell>
                  </TableRow>
                ) : (
                  list.map((h) => (
                    <TableRow key={h.id}>
                      <TableCell>
                        <div className="font-medium">{h.product_name || "-"}</div>
                        {h.domain && (
                          <div className="text-xs text-muted-foreground">{h.domain}</div>
                        )}
                      </TableCell>
                      <TableCell>
                        <Badge variant={statusVariant(h.domain_status)}>
                          {h.domain_status
                            ? KNOWN_HOST_STATUSES.includes(h.domain_status)
                              ? t(`console.status.${h.domain_status}`)
                              : h.domain_status
                            : "-"}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-sm">
                        <div>{h.dedicated_ip || "-"}</div>
                        {h.assigned_ips && h.assigned_ips !== h.dedicated_ip && (
                          <div className="text-xs text-muted-foreground">{h.assigned_ips}</div>
                        )}
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {h.next_due_date || "-"}
                      </TableCell>
                      <TableCell className="text-muted-foreground">{h.synced_at}</TableCell>
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
