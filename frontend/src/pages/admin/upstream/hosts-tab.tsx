// 上游主机页签：手动拉取上游 /host/list 做只读对账展示。
// 手动触发（不随挂载自动请求）：上游可能未配置/较慢，且对账是低频操作。
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { Loader2, Server } from "lucide-react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { api, type UpstreamHostList } from "@/lib/api"
import { errorMessage } from "@/lib/error"

const PAGE_SIZE = 20

/** 上游状态 → Badge 视觉映射（未知状态用 outline）。 */
function statusVariant(status: string): "default" | "secondary" | "destructive" | "outline" {
  switch (status) {
    case "Active":
      return "default"
    case "Pending":
      return "secondary"
    case "Suspended":
    case "Cancelled":
    case "Fraud":
    case "Deleted":
      return "destructive"
    default:
      return "outline"
  }
}

export function HostsTab() {
  const { t } = useTranslation()

  const [data, setData] = useState<UpstreamHostList | null>(null)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)
  const [loadError, setLoadError] = useState<string | null>(null)

  async function load(targetPage: number) {
    setLoading(true)
    setLoadError(null)
    try {
      const list = await api.admin.upstream.hosts(targetPage, PAGE_SIZE)
      setData(list)
      setPage(targetPage)
    } catch (err) {
      setLoadError(errorMessage(err, t("common.unknownError")))
    } finally {
      setLoading(false)
    }
  }

  const totalPages = data ? Math.max(1, Math.ceil(data.sum / PAGE_SIZE)) : 1

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Server className="size-4" />
          {t("upstream.hosts.title")}
        </CardTitle>
        <CardDescription>{t("upstream.hosts.description")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-center gap-3">
          <Button onClick={() => void load(page)} disabled={loading}>
            {loading ? <Loader2 className="animate-spin" /> : <Server />}
            {t("upstream.hosts.load")}
          </Button>
          {data && (
            <span className="text-sm text-muted-foreground">
              {t("upstream.hosts.sum", { sum: data.sum })}
            </span>
          )}
        </div>

        {loadError && (
          <Alert variant="destructive">
            <AlertDescription className="flex items-center justify-between gap-3">
              <span className="break-all">{loadError}</span>
              <Button size="sm" variant="outline" onClick={() => void load(page)}>
                {t("common.retry")}
              </Button>
            </AlertDescription>
          </Alert>
        )}

        {data && (
          <>
            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("upstream.hosts.id")}</TableHead>
                    <TableHead>{t("upstream.hosts.domain")}</TableHead>
                    <TableHead>{t("upstream.hosts.username")}</TableHead>
                    <TableHead>{t("upstream.hosts.status")}</TableHead>
                    <TableHead>{t("upstream.hosts.nextduedate")}</TableHead>
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
                  ) : data.list.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={5} className="py-10 text-center text-muted-foreground">
                        {t("upstream.hosts.empty")}
                      </TableCell>
                    </TableRow>
                  ) : (
                    data.list.map((host, i) => {
                      const status = String(host["domainstatus"] ?? "")
                      const statusInfo = data.domainstatus?.[status]
                      return (
                        <TableRow key={String(host["id"] ?? i)}>
                          <TableCell className="font-mono text-xs">{String(host["id"] ?? "")}</TableCell>
                          <TableCell className="max-w-56 truncate">{String(host["domain"] ?? "")}</TableCell>
                          <TableCell className="max-w-40 truncate">{String(host["username"] ?? "")}</TableCell>
                          <TableCell>
                            <Badge variant={statusVariant(status)}>
                              {statusInfo?.name || status || "-"}
                            </Badge>
                          </TableCell>
                          <TableCell className="text-muted-foreground">
                            {String(host["nextduedate"] ?? "-")}
                          </TableCell>
                        </TableRow>
                      )
                    })
                  )}
                </TableBody>
              </Table>
            </div>
            <div className="flex items-center justify-end gap-2">
              <Button
                variant="outline"
                size="sm"
                disabled={loading || page <= 1}
                onClick={() => void load(page - 1)}
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
                onClick={() => void load(page + 1)}
              >
                {t("upstream.hosts.nextPage")}
              </Button>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}
