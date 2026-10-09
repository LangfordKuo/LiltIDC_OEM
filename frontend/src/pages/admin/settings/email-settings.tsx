// SMTP 邮件配置：host/port/用户名/密码/发件人/STARTTLS + 发送测试邮件。
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Loader2, Mail } from "lucide-react"

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
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { api } from "@/lib/api"
import { errorMessage } from "@/lib/error"

export function EmailSettings() {
  const { t } = useTranslation()

  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [host, setHost] = useState("")
  const [port, setPort] = useState("25")
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [from, setFrom] = useState("")
  const [tls, setTls] = useState(false)
  const [passwordSet, setPasswordSet] = useState(false)
  const [saving, setSaving] = useState(false)

  // 测试发送
  const [testOpen, setTestOpen] = useState(false)
  const [testTo, setTestTo] = useState("")
  const [testSubject, setTestSubject] = useState("")
  const [testContent, setTestContent] = useState("")
  const [testSending, setTestSending] = useState(false)
  // 加载失败后的重试令牌（自增触发 effect 重新拉取）
  const [reloadKey, setReloadKey] = useState(0)

  // 挂载时加载配置：setState 全部发生在异步回调中（react-hooks/set-state-in-effect）
  useEffect(() => {
    let alive = true
    api.admin.notify
      .getSettings()
      .then((s) => {
        if (!alive) return
        setHost(s.smtp.host)
        setPort(String(s.smtp.port))
        setUsername(s.smtp.username)
        setFrom(s.smtp.from)
        setTls(s.smtp.tls)
        setPasswordSet(s.smtp.password_set)
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
  }, [t, reloadKey])

  // 加载失败后的重试（事件处理器内 setState，规则允许）
  function handleRetry() {
    setLoading(true)
    setReloadKey((k) => k + 1)
  }

  async function handleSave() {
    const portNum = Number(port)
    if (!Number.isInteger(portNum) || portNum < 1 || portNum > 65535) {
      toast.error(t("install.errPort"))
      return
    }
    setSaving(true)
    try {
      await api.admin.notify.saveSettings({
        smtp: { host, port: portNum, username, password, from, tls },
      })
      setPassword("")
      if (password) setPasswordSet(true)
      toast.success(t("settings.email.saved"))
    } catch (err) {
      toast.error(errorMessage(err, t("common.unknownError")))
    } finally {
      setSaving(false)
    }
  }

  async function handleTestSend() {
    if (!testTo.trim()) return
    setTestSending(true)
    try {
      await api.admin.notify.testEmail(
        testTo.trim(),
        testSubject.trim() || undefined,
        testContent.trim() || undefined,
      )
      toast.success(t("settings.email.testSent"))
      setTestOpen(false)
      setTestSubject("")
      setTestContent("")
    } catch (err) {
      toast.error(errorMessage(err, t("common.unknownError")))
    } finally {
      setTestSending(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Mail className="size-4" />
          {t("settings.email.title")}
          <Badge variant={passwordSet ? "default" : "secondary"} className="ml-1 font-normal">
            {passwordSet ? t("settings.email.passwordSet") : t("settings.email.passwordNotSet")}
          </Badge>
        </CardTitle>
        <CardDescription>{t("settings.email.description")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
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
        {loading ? (
          // 数据未回填前显示加载占位，避免页签切换时的空白闪烁
          <div className="flex items-center justify-center gap-2 py-12 text-muted-foreground">
            <Loader2 className="size-4 animate-spin" />
            <span className="text-sm">{t("common.loading")}</span>
          </div>
        ) : (
          <>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="smtp-host">{t("settings.email.host")}</Label>
            <Input
              id="smtp-host"
              value={host}
              onChange={(e) => setHost(e.target.value)}
              placeholder="smtp.example.com"
              disabled={loading || saving}
              autoComplete="off"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="smtp-port">{t("settings.email.port")}</Label>
            <Input
              id="smtp-port"
              value={port}
              onChange={(e) => setPort(e.target.value)}
              inputMode="numeric"
              disabled={loading || saving}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="smtp-username">{t("settings.email.username")}</Label>
            <Input
              id="smtp-username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              disabled={loading || saving}
              autoComplete="off"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="smtp-password">{t("settings.email.password")}</Label>
            <Input
              id="smtp-password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder={passwordSet ? t("settings.email.passwordPlaceholder") : ""}
              disabled={loading || saving}
              autoComplete="new-password"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="smtp-from">{t("settings.email.from")}</Label>
            <Input
              id="smtp-from"
              value={from}
              onChange={(e) => setFrom(e.target.value)}
              placeholder="noreply@example.com"
              disabled={loading || saving}
              autoComplete="off"
            />
          </div>
          <div className="flex items-center justify-between rounded-md border p-3">
            <div className="space-y-1">
              <Label htmlFor="smtp-tls">{t("settings.email.tls")}</Label>
              <p className="text-xs text-muted-foreground">{t("settings.email.tlsHelp")}</p>
            </div>
            <Switch
              id="smtp-tls"
              checked={tls}
              onCheckedChange={setTls}
              disabled={loading || saving}
            />
          </div>
        </div>

        <div className="flex flex-wrap gap-2">
          <Button onClick={handleSave} disabled={loading || saving}>
            {saving ? (
              <>
                <Loader2 className="animate-spin" />
                {t("common.saving")}
              </>
            ) : (
              t("settings.email.save")
            )}
          </Button>
          <Button variant="outline" onClick={() => setTestOpen(true)} disabled={loading}>
            {t("settings.email.testSend")}
          </Button>
        </div>
          </>
        )}
      </CardContent>

      {/* 测试邮件对话框 */}
      <Dialog open={testOpen} onOpenChange={setTestOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("settings.email.testTitle")}</DialogTitle>
            <DialogDescription>{t("settings.email.testDescription")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="test-email-to">{t("settings.email.testTo")}</Label>
              <Input
                id="test-email-to"
                type="email"
                value={testTo}
                onChange={(e) => setTestTo(e.target.value)}
                placeholder="user@example.com"
                disabled={testSending}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="test-email-subject">{t("settings.email.testSubject")}</Label>
              <Input
                id="test-email-subject"
                value={testSubject}
                onChange={(e) => setTestSubject(e.target.value)}
                disabled={testSending}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="test-email-content">{t("settings.email.testContent")}</Label>
              <Textarea
                id="test-email-content"
                value={testContent}
                onChange={(e) => setTestContent(e.target.value)}
                rows={4}
                disabled={testSending}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setTestOpen(false)} disabled={testSending}>
              {t("common.cancel")}
            </Button>
            <Button onClick={handleTestSend} disabled={testSending || !testTo.trim()}>
              {testSending && <Loader2 className="animate-spin" />}
              {t("settings.email.testSubmit")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  )
}
