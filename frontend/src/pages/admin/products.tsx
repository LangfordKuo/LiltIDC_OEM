// 商品管理页（替换批 1 占位页）：本地商品列表（分组筛选/分页）+ 编辑对话框 +
// 上下架开关 + 手动同步库存（上游 proinfo）+ 从上游导入（Sheet）。
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Download, Loader2, Pencil, RefreshCw } from "lucide-react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { api, type ProductData, type ProductGroupData } from "@/lib/api"
import { errorMessage } from "@/lib/error"
import { isValidPrice, priceSummary, sortedPriceEntries } from "@/lib/prices"

import { ImportSheet } from "./products/import-sheet"

const PAGE_SIZE = 20

export default function ProductsPage() {
  const { t } = useTranslation()
  const cycleLabel = (cycle: string) => t(`products.cycles.${cycle}`, { defaultValue: cycle })

  const [list, setList] = useState<ProductData[]>([])
  const [total, setTotal] = useState(0)
  const [groups, setGroups] = useState<ProductGroupData[]>([])
  const [groupFilter, setGroupFilter] = useState("all")
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)
  const [syncing, setSyncing] = useState(false)
  const [importOpen, setImportOpen] = useState(false)

  // 编辑对话框
  const [editing, setEditing] = useState<ProductData | null>(null)
  const [editName, setEditName] = useState("")
  const [editGroupId, setEditGroupId] = useState("")
  const [editSort, setEditSort] = useState("0")
  const [editPrices, setEditPrices] = useState<Record<string, string>>({})
  const [editSaving, setEditSaving] = useState(false)

  // 加载列表 + 分组（setState 全在异步回调，react-hooks/set-state-in-effect）
  useEffect(() => {
    let alive = true
    Promise.all([
      api.admin.products.list({
        group_id: groupFilter === "all" ? undefined : Number(groupFilter),
        page,
        page_size: PAGE_SIZE,
      }),
      api.admin.products.groups(),
    ])
      .then(([data, g]) => {
        if (!alive) return
        setList(data.list)
        setTotal(data.total)
        setGroups(g.list)
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
  }, [groupFilter, page, reloadKey, t])

  function reload() {
    setLoading(true)
    setReloadKey((k) => k + 1)
  }

  async function handleSyncStock() {
    setSyncing(true)
    try {
      const r = await api.admin.products.syncStock()
      toast.success(t("products.syncStockDone", { synced: r.synced, total: r.total }))
      reload()
    } catch (err) {
      toast.error(errorMessage(err, t("common.unknownError")))
    } finally {
      setSyncing(false)
    }
  }

  async function handleToggleStatus(p: ProductData, on: boolean) {
    try {
      const updated = await api.admin.products.updateStatus(p.id, on ? 1 : 0)
      setList((prev) => prev.map((x) => (x.id === p.id ? { ...x, status: updated.status } : x)))
      toast.success(t("products.statusSaved"))
    } catch (err) {
      toast.error(errorMessage(err, t("common.unknownError")))
    }
  }

  function openEdit(p: ProductData) {
    setEditing(p)
    setEditName(p.name)
    setEditGroupId(String(p.group_id))
    setEditSort(String(p.sort))
    setEditPrices({ ...p.sale_prices })
  }

  async function handleSaveEdit() {
    if (!editing) return
    if (!editName.trim()) {
      toast.error(t("install.errRequired", { field: t("products.name") }))
      return
    }
    for (const price of Object.values(editPrices)) {
      if (!isValidPrice(price)) {
        toast.error(t("products.importSheet.priceRequired", { name: editName }))
        return
      }
    }
    setEditSaving(true)
    try {
      await api.admin.products.update(editing.id, {
        name: editName.trim(),
        group_id: Number(editGroupId) || undefined,
        sale_prices: editPrices,
        sort: Number(editSort) || 0,
      })
      toast.success(t("products.saved"))
      setEditing(null)
      reload()
    } catch (err) {
      toast.error(errorMessage(err, t("common.unknownError")))
    } finally {
      setEditSaving(false)
    }
  }

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">{t("products.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("products.total", { total })}</p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" onClick={handleSyncStock} disabled={syncing || loading}>
            {syncing ? <Loader2 className="animate-spin" /> : <RefreshCw />}
            {t("products.syncStock")}
          </Button>
          <Button onClick={() => setImportOpen(true)}>
            <Download />
            {t("products.import")}
          </Button>
        </div>
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
            <Select
              value={groupFilter}
              onValueChange={(v) => {
                setGroupFilter(v)
                setPage(1)
              }}
            >
              <SelectTrigger className="w-48" aria-label={t("products.group")}>
                <SelectValue placeholder={t("products.group")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t("products.allGroups")}</SelectItem>
                {groups.map((g) => (
                  <SelectItem key={g.id} value={String(g.id)}>
                    {g.name}
                  </SelectItem>
                ))}
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
                  <TableHead>{t("products.name")}</TableHead>
                  <TableHead>{t("products.group")}</TableHead>
                  <TableHead>{t("products.salePrice")}</TableHead>
                  <TableHead>{t("products.stock")}</TableHead>
                  <TableHead>{t("products.status")}</TableHead>
                  <TableHead>{t("products.sort")}</TableHead>
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
                      {t("products.empty")}
                    </TableCell>
                  </TableRow>
                ) : (
                  list.map((p) => (
                    <TableRow key={p.id}>
                      <TableCell>
                        <div className="font-medium">{p.name}</div>
                        <div className="text-xs text-muted-foreground">
                          {t("products.upstreamPid")}: {p.upstream_pid}
                          {p.type && ` · ${p.type}`}
                        </div>
                      </TableCell>
                      <TableCell className="text-muted-foreground">{p.group_name || "-"}</TableCell>
                      <TableCell className="max-w-64 text-sm text-muted-foreground">
                        <span className="line-clamp-2" title={priceSummary(p.sale_prices, cycleLabel)}>
                          {priceSummary(p.sale_prices, cycleLabel)}
                        </span>
                      </TableCell>
                      <TableCell>
                        {p.stock_control ? (
                          <Badge variant={p.stock_qty > 0 ? "secondary" : "destructive"}>
                            {p.stock_qty}
                          </Badge>
                        ) : (
                          <span className="text-muted-foreground">{t("products.stockUnlimited")}</span>
                        )}
                      </TableCell>
                      <TableCell>
                        <Switch
                          checked={p.status === 1}
                          onCheckedChange={(on) => void handleToggleStatus(p, on)}
                          aria-label={`${t("products.status")} #${p.id}`}
                        />
                      </TableCell>
                      <TableCell className="text-muted-foreground">{p.sort}</TableCell>
                      <TableCell>
                        <Button variant="ghost" size="icon" onClick={() => openEdit(p)}>
                          <Pencil />
                        </Button>
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

      {/* 编辑对话框 */}
      <Dialog open={editing !== null} onOpenChange={(open) => !open && setEditing(null)}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("products.edit")}</DialogTitle>
            <DialogDescription>
              {editing && `${t("products.upstreamPid")}: ${editing.upstream_pid}`}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="prod-name">{t("products.name")}</Label>
              <Input
                id="prod-name"
                value={editName}
                onChange={(e) => setEditName(e.target.value)}
                disabled={editSaving}
              />
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label>{t("products.group")}</Label>
                <Select value={editGroupId} onValueChange={setEditGroupId} disabled={editSaving}>
                  <SelectTrigger aria-label={t("products.group")}>
                    <SelectValue placeholder={t("products.group")} />
                  </SelectTrigger>
                  <SelectContent>
                    {groups.map((g) => (
                      <SelectItem key={g.id} value={String(g.id)}>
                        {g.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="prod-sort">{t("products.sort")}</Label>
                <Input
                  id="prod-sort"
                  value={editSort}
                  onChange={(e) => setEditSort(e.target.value)}
                  inputMode="numeric"
                  disabled={editSaving}
                />
              </div>
            </div>
            {/* 各周期售价（含成本参考） */}
            <div className="space-y-2">
              <Label>{t("products.salePrice")}</Label>
              <div className="grid gap-2">
                {sortedPriceEntries(editPrices).map(([cycle, price]) => (
                  <div key={cycle} className="flex items-center gap-2">
                    <span className="w-20 shrink-0 text-sm text-muted-foreground">
                      {cycleLabel(cycle)}
                    </span>
                    <Input
                      value={price}
                      onChange={(e) =>
                        setEditPrices((prev) => ({ ...prev, [cycle]: e.target.value }))
                      }
                      disabled={editSaving}
                      inputMode="decimal"
                      className="max-w-36"
                      aria-label={`${cycleLabel(cycle)} ${t("products.salePrice")}`}
                    />
                    <span className="text-xs text-muted-foreground">
                      {t("products.costPrice")}: {editing?.cost_prices?.[cycle] ?? "-"}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditing(null)} disabled={editSaving}>
              {t("common.cancel")}
            </Button>
            <Button onClick={handleSaveEdit} disabled={editSaving}>
              {editSaving && <Loader2 className="animate-spin" />}
              {t("common.save")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 从上游导入 */}
      <ImportSheet
        open={importOpen}
        onOpenChange={setImportOpen}
        groups={groups}
        onImported={reload}
      />
    </div>
  )
}
