// 消息模板管理：列表（场景/通道/语言过滤）+ 编辑对话框 + 启用开关。
// 占位符语法说明见 docs/api-contract.md 8.3。
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Bell, Loader2, Pencil } from "lucide-react"

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
import { Textarea } from "@/components/ui/textarea"
import { api, type NotifyTemplate } from "@/lib/api"
import { errorMessage } from "@/lib/error"

// Select 组件不能用空字符串作 value，用 "all" 哨兵值代表不过滤
const ALL = "all"

const SCENE_KEYS: Record<string, string> = {
  register_verify: "settings.templates.sceneRegisterVerify",
  bill_notify: "settings.templates.sceneBillNotify",
}

interface Filters {
  scene: string
  channel: string
  lang: string
}

/** 按过滤条件拉取模板列表（纯请求，无 setState，供 effect 与刷新按钮共用）。 */
async function fetchList(f: Filters): Promise<NotifyTemplate[]> {
  const r = await api.admin.notify.listTemplates({
    scene: f.scene === ALL ? undefined : f.scene,
    channel: f.channel === ALL ? undefined : f.channel,
    lang: f.lang === ALL ? undefined : f.lang,
  })
  return r.list
}

export function TemplatesSettings() {
  const { t, i18n } = useTranslation()

  const [list, setList] = useState<NotifyTemplate[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [filters, setFilters] = useState<Filters>({ scene: ALL, channel: ALL, lang: ALL })
  // 刷新令牌：按钮点击自增以重触发加载 effect（setState 在事件处理器中，符合规则）
  const [reloadKey, setReloadKey] = useState(0)

  // 编辑对话框状态
  const [editing, setEditing] = useState<NotifyTemplate | null>(null)
  const [editSubject, setEditSubject] = useState("")
  const [editContent, setEditContent] = useState("")
  const [editEnabled, setEditEnabled] = useState(true)
  const [editSaving, setEditSaving] = useState(false)

  // 加载模板：setState 全部发生在异步回调中（react-hooks/set-state-in-effect）
  useEffect(() => {
    let alive = true
    fetchList(filters)
      .then((l) => {
        if (!alive) return
        setList(l)
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
  }, [filters, reloadKey, t])

  function handleFilterChange(patch: Partial<Filters>) {
    setLoading(true)
    setFilters((f) => ({ ...f, ...patch }))
  }

  function handleRefresh() {
    setLoading(true)
    setReloadKey((k) => k + 1)
  }

  async function handleToggleEnabled(tpl: NotifyTemplate, enabled: boolean) {
    try {
      const updated = await api.admin.notify.updateTemplate(tpl.id, { enabled })
      setList((prev) => prev.map((x) => (x.id === updated.id ? updated : x)))
    } catch (err) {
      toast.error(errorMessage(err, t("common.unknownError")))
    }
  }

  function openEdit(tpl: NotifyTemplate) {
    setEditing(tpl)
    setEditSubject(tpl.subject)
    setEditContent(tpl.content)
    setEditEnabled(tpl.enabled)
  }

  async function handleSaveEdit() {
    if (!editing) return
    if (!editContent.trim()) {
      toast.error(t("settings.templates.contentRequired"))
      return
    }
    setEditSaving(true)
    try {
      const updated = await api.admin.notify.updateTemplate(editing.id, {
        subject: editSubject,
        content: editContent,
        enabled: editEnabled,
      })
      setList((prev) => prev.map((x) => (x.id === updated.id ? updated : x)))
      setEditing(null)
      toast.success(t("settings.templates.saved"))
    } catch (err) {
      toast.error(errorMessage(err, t("common.unknownError")))
    } finally {
      setEditSaving(false)
    }
  }

  function sceneLabel(scene: string) {
    return SCENE_KEYS[scene] ? t(SCENE_KEYS[scene]) : scene
  }

  function formatTime(value: string) {
    const d = new Date(value)
    if (Number.isNaN(d.getTime())) return value
    return d.toLocaleString(i18n.language === "en-US" ? "en-US" : "zh-CN", {
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Bell className="size-4" />
          {t("settings.templates.title")}
        </CardTitle>
        <CardDescription>{t("settings.templates.description")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {loadError && (
          <Alert variant="destructive">
            <AlertDescription className="flex items-center justify-between gap-3">
              <span className="break-all">{loadError}</span>
              <Button size="sm" variant="outline" onClick={handleRefresh}>
                {t("common.retry")}
              </Button>
            </AlertDescription>
          </Alert>
        )}

        {/* 过滤器 */}
        <div className="flex flex-wrap items-center gap-2">
          <Select value={filters.scene} onValueChange={(v) => handleFilterChange({ scene: v })}>
            <SelectTrigger className="w-44" aria-label={t("settings.templates.scene")}>
              <SelectValue placeholder={t("settings.templates.scene")} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>{t("settings.templates.filterAll")}</SelectItem>
              <SelectItem value="register_verify">
                {t("settings.templates.sceneRegisterVerify")}
              </SelectItem>
              <SelectItem value="bill_notify">
                {t("settings.templates.sceneBillNotify")}
              </SelectItem>
            </SelectContent>
          </Select>
          <Select
            value={filters.channel}
            onValueChange={(v) => handleFilterChange({ channel: v })}
          >
            <SelectTrigger className="w-36" aria-label={t("settings.templates.channel")}>
              <SelectValue placeholder={t("settings.templates.channel")} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>{t("settings.templates.filterAll")}</SelectItem>
              <SelectItem value="sms">{t("settings.templates.channelSms")}</SelectItem>
              <SelectItem value="email">{t("settings.templates.channelEmail")}</SelectItem>
            </SelectContent>
          </Select>
          <Select value={filters.lang} onValueChange={(v) => handleFilterChange({ lang: v })}>
            <SelectTrigger className="w-36" aria-label={t("settings.templates.lang")}>
              <SelectValue placeholder={t("settings.templates.lang")} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>{t("settings.templates.filterAll")}</SelectItem>
              <SelectItem value="zh-CN">{t("language.zhCN")}</SelectItem>
              <SelectItem value="en-US">{t("language.enUS")}</SelectItem>
            </SelectContent>
          </Select>
          <Button
            variant="ghost"
            size="icon"
            onClick={handleRefresh}
            aria-label={t("common.refresh")}
          >
            <Loader2 className={loading ? "animate-spin" : ""} />
          </Button>
        </div>

        {/* 模板列表 */}
        <div className="rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("settings.templates.scene")}</TableHead>
                <TableHead>{t("settings.templates.channel")}</TableHead>
                <TableHead>{t("settings.templates.lang")}</TableHead>
                <TableHead className="max-w-64">{t("settings.templates.subject")}</TableHead>
                <TableHead>{t("settings.templates.status")}</TableHead>
                <TableHead>{t("settings.templates.updatedAt")}</TableHead>
                <TableHead className="w-20">{t("common.actions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading ? (
                // 加载占位：避免页签切换时表格只剩表头的空白闪烁
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
                    {t("settings.templates.empty")}
                  </TableCell>
                </TableRow>
              ) : (
                list.map((tpl) => (
                  <TableRow key={tpl.id}>
                    <TableCell>
                      <Badge variant="outline">{sceneLabel(tpl.scene)}</Badge>
                    </TableCell>
                    <TableCell>
                      <Badge variant={tpl.channel === "sms" ? "default" : "secondary"}>
                        {tpl.channel === "sms"
                          ? t("settings.templates.channelSms")
                          : t("settings.templates.channelEmail")}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-muted-foreground">{tpl.lang}</TableCell>
                    <TableCell className="max-w-64 truncate text-muted-foreground">
                      {tpl.channel === "sms" ? (
                        <span className="text-xs">{tpl.content.slice(0, 40)}</span>
                      ) : (
                        tpl.subject
                      )}
                    </TableCell>
                    <TableCell>
                      <Switch
                        checked={tpl.enabled}
                        onCheckedChange={(v) => void handleToggleEnabled(tpl, v)}
                        aria-label={`${t("settings.templates.status")} #${tpl.id}`}
                      />
                    </TableCell>
                    <TableCell className="text-muted-foreground">{formatTime(tpl.updated_at)}</TableCell>
                    <TableCell>
                      <Button variant="ghost" size="icon" onClick={() => openEdit(tpl)}>
                        <Pencil />
                      </Button>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
      </CardContent>

      {/* 编辑对话框 */}
      <Dialog open={editing !== null} onOpenChange={(open) => !open && setEditing(null)}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>
              {t("settings.templates.edit")}
              {editing && (
                <span className="ml-2 text-sm font-normal text-muted-foreground">
                  #{editing.id} · {sceneLabel(editing.scene)} · {editing.channel} · {editing.lang}
                </span>
              )}
            </DialogTitle>
            <DialogDescription>{t("settings.templates.placeholdersHelp")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="tpl-subject">{t("settings.templates.subject")}</Label>
              <Input
                id="tpl-subject"
                value={editSubject}
                onChange={(e) => setEditSubject(e.target.value)}
                placeholder={t("settings.templates.subjectPlaceholder")}
                disabled={editSaving || editing?.channel === "sms"}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="tpl-content">{t("settings.templates.content")}</Label>
              <Textarea
                id="tpl-content"
                value={editContent}
                onChange={(e) => setEditContent(e.target.value)}
                rows={6}
                disabled={editSaving}
                className="font-mono text-sm"
              />
            </div>
            <div className="flex items-center justify-between rounded-md border p-3">
              <Label htmlFor="tpl-enabled">{t("common.enabled")}</Label>
              <Switch
                id="tpl-enabled"
                checked={editEnabled}
                onCheckedChange={setEditEnabled}
                disabled={editSaving}
              />
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
    </Card>
  )
}
