// 从上游导入商品的侧拉抽屉：
// 拉取上游商品树 → 勾选 → 逐个配置本地名称/分组/各周期售价（显示成本价与利润率）→ 提交。
// 已导入过的商品会预填本地售价，再次提交 = 更新映射与售价（后端幂等）。
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Loader2 } from "lucide-react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { api, type ImportItem, type PriceMap, type ProductGroupData, type UpstreamGroup, type UpstreamProduct } from "@/lib/api"
import { errorMessage } from "@/lib/error"
import { isValidPrice, marginPercent, priceSummary, sortedPriceEntries } from "@/lib/prices"

// 单条导入配置的本地编辑态
interface ItemConfig {
  name: string
  /** "" 未选 | "new" 新建分组 | 数字字符串 = 已有分组 id */
  groupId: string
  newGroupName: string
  prices: Record<string, string>
}

interface ImportSheetProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** 已有本地分组（父组件传入，导入成功后由父组件刷新） */
  groups: ProductGroupData[]
  onImported: () => void
}

export function ImportSheet({ open, onOpenChange, groups, onImported }: ImportSheetProps) {
  const { t } = useTranslation()
  const cycleLabel = (cycle: string) => t(`products.cycles.${cycle}`, { defaultValue: cycle })

  const [data, setData] = useState<UpstreamGroup[] | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)
  // 派生 loading：打开且尚无数据且无错误（避免 effect 内同步 setState）
  const loading = open && data === null && loadError === null

  const [selected, setSelected] = useState<number[]>([])
  const [configs, setConfigs] = useState<Record<number, ItemConfig>>({})
  const [submitting, setSubmitting] = useState(false)

  // 打开时拉取上游商品树（setState 全在异步回调）
  useEffect(() => {
    if (!open) return
    let alive = true
    api.admin.upstream
      .products()
      .then((r) => {
        if (alive) setData(r.groups)
      })
      .catch((err: unknown) => {
        if (alive) setLoadError(errorMessage(err, t("common.unknownError")))
      })
    return () => {
      alive = false
    }
  }, [open, reloadKey, t])

  function handleRetry() {
    setLoadError(null)
    setReloadKey((k) => k + 1)
  }

  function findProduct(pid: number): UpstreamProduct | null {
    for (const g of data ?? []) {
      for (const p of g.products) {
        if (p.id === pid) return p
      }
    }
    return null
  }

  function toggleSelect(p: UpstreamProduct, checked: boolean) {
    if (checked) {
      setSelected((prev) => (prev.includes(p.id) ? prev : [...prev, p.id]))
      setConfigs((prev) => ({
        ...prev,
        [p.id]: {
          name: p.imported && p.local_name ? p.local_name : p.name,
          groupId: "",
          newGroupName: "",
          // 已导入 → 预填本地售价；未导入 → 留空强制填写（防误按成本价提交）
          prices:
            p.imported && p.local_sale_prices
              ? { ...p.local_sale_prices }
              : Object.fromEntries(Object.keys(p.prices).map((c) => [c, ""])),
        },
      }))
    } else {
      setSelected((prev) => prev.filter((id) => id !== p.id))
    }
  }

  function patchConfig(pid: number, patch: Partial<ItemConfig>) {
    setConfigs((prev) => ({ ...prev, [pid]: { ...prev[pid], ...patch } }))
  }

  async function handleSubmit() {
    if (selected.length === 0) {
      toast.error(t("products.importSheet.selectNone"))
      return
    }
    const items: ImportItem[] = []
    for (const pid of selected) {
      const cfg = configs[pid]
      const p = findProduct(pid)
      if (!cfg || !p) continue
      const displayName = cfg.name.trim() || p.name
      // 分组校验
      if (cfg.groupId === "" || (cfg.groupId === "new" && !cfg.newGroupName.trim())) {
        toast.error(t("products.importSheet.groupRequired", { name: displayName }))
        return
      }
      // 售价校验：每个周期必填且 > 0
      for (const price of Object.values(cfg.prices)) {
        if (!isValidPrice(price)) {
          toast.error(t("products.importSheet.priceRequired", { name: displayName }))
          return
        }
      }
      items.push({
        upstream_pid: pid,
        name: displayName,
        group_id: cfg.groupId === "new" ? 0 : Number(cfg.groupId),
        group_name: cfg.groupId === "new" ? cfg.newGroupName.trim() : "",
        sale_prices: cfg.prices,
        status: p.imported ? p.local_status : 1,
      })
    }
    if (items.length === 0) return

    setSubmitting(true)
    try {
      const r = await api.admin.upstream.importProducts(items)
      toast.success(t("products.importSheet.done", { created: r.created, updated: r.updated }))
      setSelected([])
      setConfigs({})
      onOpenChange(false)
      onImported()
    } catch (err) {
      toast.error(errorMessage(err, t("common.unknownError")))
    } finally {
      setSubmitting(false)
    }
  }

  const selectedProducts = selected
    .map((pid) => findProduct(pid))
    .filter((p): p is UpstreamProduct => p !== null)

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="w-full overflow-y-auto sm:max-w-2xl">
        <SheetHeader>
          <SheetTitle>{t("products.importSheet.title")}</SheetTitle>
          <SheetDescription>{t("products.importSheet.description")}</SheetDescription>
        </SheetHeader>

        <div className="mt-4 space-y-6">
          {loadError && (
            <Alert variant="destructive">
              <AlertDescription className="flex items-center justify-between gap-3">
                <span className="break-all">{loadError}</span>
                <Button size="sm" variant="outline" onClick={handleRetry}>
                  {t("common.retry")}
                </Button>
              </AlertDescription>
            </Alert>
          )}

          {loading && (
            <div className="flex items-center justify-center gap-2 py-12 text-muted-foreground">
              <Loader2 className="size-4 animate-spin" />
              <span className="text-sm">{t("products.importSheet.loading")}</span>
            </div>
          )}

          {/* 上游商品树 */}
          {data && data.length === 0 && !loading && (
            <p className="py-8 text-center text-sm text-muted-foreground">
              {t("products.importSheet.empty")}
            </p>
          )}
          {data?.map((group) => (
            <div key={group.id} className="space-y-2">
              <h3 className="text-sm font-semibold text-muted-foreground">{group.name}</h3>
              <div className="space-y-1">
                {group.products.map((p) => {
                  const noCycles = Object.keys(p.prices).length === 0
                  return (
                    <label
                      key={p.id}
                      className="flex cursor-pointer items-start gap-3 rounded-md border p-3 hover:bg-accent/50"
                    >
                      <Checkbox
                        checked={selected.includes(p.id)}
                        onCheckedChange={(v) => toggleSelect(p, v === true)}
                        disabled={noCycles}
                        className="mt-0.5"
                        aria-label={p.name}
                      />
                      <div className="min-w-0 flex-1">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-medium">{p.name}</span>
                          {p.type && <Badge variant="outline">{p.type}</Badge>}
                          {p.imported && (
                            <Badge variant="secondary">{t("products.importSheet.imported")}</Badge>
                          )}
                        </div>
                        <p className="mt-1 text-xs text-muted-foreground">
                          {t("products.costPrice")}:{" "}
                          {noCycles ? "-" : priceSummary(p.prices, cycleLabel)}
                        </p>
                      </div>
                    </label>
                  )
                })}
              </div>
            </div>
          ))}

          {/* 选中商品配置区 */}
          {selectedProducts.length > 0 && (
            <>
              <Separator />
              <div className="space-y-3">
                <h3 className="text-sm font-semibold">
                  {t("products.importSheet.configureTitle", { count: selectedProducts.length })}
                </h3>
                {selectedProducts.map((p) => {
                  const cfg = configs[p.id]
                  if (!cfg) return null
                  return (
                    <Card key={p.id}>
                      <CardContent className="space-y-3 pt-4">
                        <div className="text-sm font-medium">{p.name}</div>
                        <div className="grid gap-3 sm:grid-cols-2">
                          <div className="space-y-1.5">
                            <Label>{t("products.importSheet.localName")}</Label>
                            <Input
                              value={cfg.name}
                              onChange={(e) => patchConfig(p.id, { name: e.target.value })}
                              disabled={submitting}
                            />
                          </div>
                          <div className="space-y-1.5">
                            <Label>{t("products.importSheet.group")}</Label>
                            <Select
                              value={cfg.groupId}
                              onValueChange={(v) => patchConfig(p.id, { groupId: v })}
                              disabled={submitting}
                            >
                              <SelectTrigger aria-label={t("products.importSheet.group")}>
                                <SelectValue placeholder={t("products.group")} />
                              </SelectTrigger>
                              <SelectContent>
                                {groups.map((g) => (
                                  <SelectItem key={g.id} value={String(g.id)}>
                                    {g.name}
                                  </SelectItem>
                                ))}
                                <SelectItem value="new">
                                  {t("products.importSheet.createGroup")}
                                </SelectItem>
                              </SelectContent>
                            </Select>
                            {cfg.groupId === "new" && (
                              <Input
                                value={cfg.newGroupName}
                                onChange={(e) =>
                                  patchConfig(p.id, { newGroupName: e.target.value })
                                }
                                placeholder={t("products.importSheet.newGroupName")}
                                disabled={submitting}
                                className="mt-1.5"
                              />
                            )}
                          </div>
                        </div>
                        {/* 各周期售价：成本提示 + 利润率 */}
                        <div className="space-y-2">
                          {sortedPriceEntries(p.prices as PriceMap).map(([cycle, cost]) => {
                            const sale = cfg.prices[cycle] ?? ""
                            const margin = marginPercent(cost, sale)
                            return (
                              <div key={cycle} className="flex items-center gap-2">
                                <span className="w-20 shrink-0 text-sm text-muted-foreground">
                                  {cycleLabel(cycle)}
                                </span>
                                <Input
                                  value={sale}
                                  onChange={(e) =>
                                    patchConfig(p.id, {
                                      prices: { ...cfg.prices, [cycle]: e.target.value },
                                    })
                                  }
                                  disabled={submitting}
                                  inputMode="decimal"
                                  placeholder="0.00"
                                  className="max-w-36"
                                  aria-label={t("products.importSheet.salePriceCycle", { cycle: cycleLabel(cycle) })}
                                />
                                <span className="text-xs text-muted-foreground">
                                  {t("products.importSheet.costHint", { price: cost })}
                                  {margin !== null &&
                                    ` · ${t("products.importSheet.marginHint", { percent: margin })}`}
                                </span>
                              </div>
                            )
                          })}
                        </div>
                      </CardContent>
                    </Card>
                  )
                })}
              </div>
            </>
          )}

          {/* 提交栏 */}
          <div className="sticky bottom-0 flex items-center justify-between gap-3 border-t bg-background pt-4">
            <span className="text-sm text-muted-foreground">
              {t("products.importSheet.selected", { count: selected.length })}
            </span>
            <Button onClick={handleSubmit} disabled={submitting || selected.length === 0}>
              {submitting && <Loader2 className="animate-spin" />}
              {submitting
                ? t("products.importSheet.submitting")
                : t("products.importSheet.submit")}
            </Button>
          </div>
        </div>
      </SheetContent>
    </Sheet>
  )
}
