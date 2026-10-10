// 会员管理页（批 3-α，替换占位页）：
// 列表（关键词搜索/状态筛选/分页）+ 启用/禁用/标记上游异常 + 重置密码 + 软删除。
// 批 3-β 增补：余额列 + 「充值/扣款」Dialog（人工调账写流水，契约 14.2）。
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { KeyRound, Loader2, MoreHorizontal, Search, Trash2, Users, Wallet } from "lucide-react"

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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
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
import { api, type ClientView } from "@/lib/api"
import { errorMessage } from "@/lib/error"

const PAGE_SIZE = 20

/** 状态 → Badge 视觉映射（3=上游关联异常要醒目）。 */
function statusVariant(status: number): "default" | "secondary" | "destructive" | "outline" {
  switch (status) {
    case 1:
      return "default"
    case 0:
      return "secondary"
    case 3:
      return "destructive"
    default:
      return "outline" // 2=已删除等
  }
}

export default function MembersPage() {
  const { t } = useTranslation()

  const [list, setList] = useState<ClientView[]>([])
  const [total, setTotal] = useState(0)
  const [keywordInput, setKeywordInput] = useState("")
  const [keyword, setKeyword] = useState("")
  const [statusFilter, setStatusFilter] = useState("all")
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)

  // 重置密码对话框
  const [pwdTarget, setPwdTarget] = useState<ClientView | null>(null)
  const [pwd1, setPwd1] = useState("")
  const [pwd2, setPwd2] = useState("")
  const [pwdSaving, setPwdSaving] = useState(false)

  // 删除确认对话框
  const [delTarget, setDelTarget] = useState<ClientView | null>(null)
  const [deleting, setDeleting] = useState(false)

  // 充值/扣款对话框（批 3-β）
  const [balTarget, setBalTarget] = useState<ClientView | null>(null)
  const [balAmount, setBalAmount] = useState("")
  const [balRemark, setBalRemark] = useState("")
  const [balSaving, setBalSaving] = useState(false)

  // 加载列表（setState 全在异步回调，react-hooks/set-state-in-effect）
  useEffect(() => {
    let alive = true
    api.admin.clients
      .list({
        keyword: keyword || undefined,
        status: statusFilter === "all" ? undefined : statusFilter,
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

  function reload() {
    setLoading(true)
    setReloadKey((k) => k + 1)
  }

  function handleSearch() {
    setKeyword(keywordInput.trim())
    setPage(1)
  }

  async function handleSetStatus(c: ClientView, status: number) {
    try {
      const updated = await api.admin.clients.setStatus(c.id, status)
      setList((prev) => prev.map((x) => (x.id === c.id ? updated : x)))
      toast.success(t("members.statusSaved"))
    } catch (err) {
      toast.error(errorMessage(err, t("common.unknownError")))
    }
  }

  function openResetPwd(c: ClientView) {
    setPwdTarget(c)
    setPwd1("")
    setPwd2("")
  }

  async function handleResetPwd() {
    if (!pwdTarget) return
    if (pwd1 !== pwd2) {
      toast.error(t("members.pwdMismatch"))
      return
    }
    setPwdSaving(true)
    try {
      await api.admin.clients.resetPassword(pwdTarget.id, pwd1)
      toast.success(t("members.resetDone"))
      setPwdTarget(null)
    } catch (err) {
      toast.error(errorMessage(err, t("common.unknownError")))
    } finally {
      setPwdSaving(false)
    }
  }

  function openBalance(c: ClientView) {
    setBalTarget(c)
    setBalAmount("")
    setBalRemark("")
  }

  async function handleAdjustBalance() {
    if (!balTarget) return
    const raw = balAmount.trim()
    // 前端预校验：带符号十进制金额（后端 money 包严格复核）
    if (!/^[+-]?\d+(\.\d{1,2})?$/.test(raw) || Number(raw) === 0) {
      toast.error(t("members.balAmountInvalid"))
      return
    }
    setBalSaving(true)
    try {
      const res = await api.admin.clients.adjustBalance(
        balTarget.id,
        raw.startsWith("+") || raw.startsWith("-") ? raw : `+${raw}`,
        balRemark.trim(),
      )
      toast.success(t("members.balDone", { credit: res.credit }))
      setBalTarget(null)
      reload()
    } catch (err) {
      toast.error(errorMessage(err, t("common.unknownError")))
    } finally {
      setBalSaving(false)
    }
  }

  async function handleDelete() {
    if (!delTarget) return
    setDeleting(true)
    try {
      await api.admin.clients.remove(delTarget.id)
      toast.success(t("members.deleteDone"))
      setDelTarget(null)
      reload()
    } catch (err) {
      toast.error(errorMessage(err, t("common.unknownError")))
    } finally {
      setDeleting(false)
    }
  }

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  return (
    <div className="space-y-6">
      <div>
        <h1 className="flex items-center gap-2 text-2xl font-bold tracking-tight">
          <Users className="size-6" />
          {t("members.title")}
        </h1>
        <p className="text-sm text-muted-foreground">{t("members.total", { total })}</p>
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

          {/* 工具栏：搜索 + 状态筛选 + 刷新 */}
          <div className="flex flex-wrap items-center gap-2">
            <div className="relative w-64">
              <Search className="absolute left-2.5 top-2.5 size-4 text-muted-foreground" />
              <Input
                value={keywordInput}
                onChange={(e) => setKeywordInput(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && handleSearch()}
                placeholder={t("members.searchPlaceholder")}
                className="pl-8"
                aria-label={t("members.searchPlaceholder")}
              />
            </div>
            <Button variant="outline" onClick={handleSearch}>
              {t("members.search")}
            </Button>
            <Select
              value={statusFilter}
              onValueChange={(v) => {
                setStatusFilter(v)
                setPage(1)
              }}
            >
              <SelectTrigger className="w-48" aria-label={t("members.colStatus")}>
                <SelectValue placeholder={t("members.colStatus")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t("members.statusAll")}</SelectItem>
                <SelectItem value="1">{t("members.colStatus")} · 1</SelectItem>
                <SelectItem value="0">{t("members.colStatus")} · 0</SelectItem>
                <SelectItem value="3">{t("members.colStatus")} · 3</SelectItem>
                <SelectItem value="2">{t("members.statusDeleted")}</SelectItem>
              </SelectContent>
            </Select>
            <Button variant="ghost" size="icon" onClick={reload} aria-label={t("common.refresh")}>
              <Loader2 className={loading ? "animate-spin" : ""} />
            </Button>
          </div>

          {/* 列表 */}
          <div className="rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("members.colUsername")}</TableHead>
                  <TableHead>{t("members.colContact")}</TableHead>
                  <TableHead>{t("members.colRealName")}</TableHead>
                  <TableHead className="text-right">{t("members.colCredit")}</TableHead>
                  <TableHead>{t("members.colStatus")}</TableHead>
                  <TableHead>{t("members.colLastLogin")}</TableHead>
                  <TableHead>{t("members.colCreated")}</TableHead>
                  <TableHead className="w-16">{t("common.actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {loading ? (
                  <TableRow>
                    <TableCell colSpan={8}>
                      <div className="flex items-center justify-center gap-2 py-10 text-muted-foreground">
                        <Loader2 className="size-4 animate-spin" />
                        <span className="text-sm">{t("common.loading")}</span>
                      </div>
                    </TableCell>
                  </TableRow>
                ) : list.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={8} className="py-10 text-center text-muted-foreground">
                      {t("members.empty")}
                    </TableCell>
                  </TableRow>
                ) : (
                  list.map((c) => (
                    <TableRow key={c.id}>
                      <TableCell>
                        <div className="font-medium">{c.username}</div>
                        <div className="text-xs text-muted-foreground">
                          ID: {c.id}
                          {c.upstream_uid != null && ` · ${t("members.upstreamUid")}: ${c.upstream_uid}`}
                        </div>
                      </TableCell>
                      <TableCell className="text-sm text-muted-foreground">
                        <div>{c.email || "-"}</div>
                        <div>{c.phone || "-"}</div>
                      </TableCell>
                      <TableCell>
                        <div>{c.real_name || "-"}</div>
                        {c.company && (
                          <div className="text-xs text-muted-foreground">{c.company}</div>
                        )}
                      </TableCell>
                      <TableCell className="text-right font-medium tabular-nums">
                        ¥{c.credit || "0.00"}
                      </TableCell>
                      <TableCell>
                        <Badge variant={statusVariant(c.status)}>
                          {c.status_text || String(c.status)}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {c.last_login_at || t("members.neverLogin")}
                      </TableCell>
                      <TableCell className="text-muted-foreground">{c.created_at}</TableCell>
                      <TableCell>
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button variant="ghost" size="icon" aria-label={t("common.actions")}>
                              <MoreHorizontal />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end">
                            {c.status === 1 ? (
                              <DropdownMenuItem onClick={() => void handleSetStatus(c, 0)}>
                                {t("members.disable")}
                              </DropdownMenuItem>
                            ) : (
                              <DropdownMenuItem onClick={() => void handleSetStatus(c, 1)}>
                                {t("members.enable")}
                              </DropdownMenuItem>
                            )}
                            {c.status !== 3 && (
                              <DropdownMenuItem onClick={() => void handleSetStatus(c, 3)}>
                                {t("members.markUpstreamIssue")}
                              </DropdownMenuItem>
                            )}
                            <DropdownMenuItem onClick={() => openBalance(c)}>
                              <Wallet />
                              {t("members.adjustBalance")}
                            </DropdownMenuItem>
                            <DropdownMenuItem onClick={() => openResetPwd(c)}>
                              <KeyRound />
                              {t("members.resetPwd")}
                            </DropdownMenuItem>
                            <DropdownMenuItem
                              className="text-destructive"
                              onClick={() => setDelTarget(c)}
                            >
                              <Trash2 />
                              {t("members.delete")}
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
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

      {/* 重置密码对话框 */}
      <Dialog open={pwdTarget !== null} onOpenChange={(open) => !open && setPwdTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("members.resetPwdTitle")}</DialogTitle>
            <DialogDescription>
              {pwdTarget && t("members.resetPwdDesc", { name: pwdTarget.username })}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="reset-pwd1">{t("members.newPassword")}</Label>
              <Input
                id="reset-pwd1"
                type="password"
                value={pwd1}
                onChange={(e) => setPwd1(e.target.value)}
                disabled={pwdSaving}
                autoComplete="new-password"
              />
              <p className="text-xs text-muted-foreground">{t("members.pwdRule")}</p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="reset-pwd2">{t("members.confirmPassword")}</Label>
              <Input
                id="reset-pwd2"
                type="password"
                value={pwd2}
                onChange={(e) => setPwd2(e.target.value)}
                disabled={pwdSaving}
                autoComplete="new-password"
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPwdTarget(null)} disabled={pwdSaving}>
              {t("common.cancel")}
            </Button>
            <Button onClick={handleResetPwd} disabled={pwdSaving || !pwd1}>
              {pwdSaving && <Loader2 className="animate-spin" />}
              {t("members.resetPwd")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 充值/扣款对话框（批 3-β，契约 14.2：写流水，余额变更与流水同事务） */}
      <Dialog open={balTarget !== null} onOpenChange={(open) => !open && setBalTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("members.balTitle")}</DialogTitle>
            <DialogDescription>
              {balTarget &&
                t("members.balDesc", {
                  name: balTarget.username,
                  credit: balTarget.credit || "0.00",
                })}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="bal-amount">{t("members.balAmount")}</Label>
              <Input
                id="bal-amount"
                value={balAmount}
                onChange={(e) => setBalAmount(e.target.value)}
                placeholder="+50.00 / -20.00"
                disabled={balSaving}
                inputMode="decimal"
              />
              <p className="text-xs text-muted-foreground">{t("members.balAmountHelp")}</p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="bal-remark">{t("members.balRemark")}</Label>
              <Input
                id="bal-remark"
                value={balRemark}
                onChange={(e) => setBalRemark(e.target.value)}
                disabled={balSaving}
                maxLength={255}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setBalTarget(null)} disabled={balSaving}>
              {t("common.cancel")}
            </Button>
            <Button onClick={handleAdjustBalance} disabled={balSaving || !balAmount.trim()}>
              {balSaving && <Loader2 className="animate-spin" />}
              {t("members.balSubmit")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 删除确认对话框 */}
      <Dialog open={delTarget !== null} onOpenChange={(open) => !open && setDelTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("members.deleteTitle")}</DialogTitle>
            <DialogDescription>
              {delTarget && `${delTarget.username} · `}
              {t("members.deleteDesc")}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDelTarget(null)} disabled={deleting}>
              {t("common.cancel")}
            </Button>
            <Button variant="destructive" onClick={handleDelete} disabled={deleting}>
              {deleting && <Loader2 className="animate-spin" />}
              {t("members.delete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
