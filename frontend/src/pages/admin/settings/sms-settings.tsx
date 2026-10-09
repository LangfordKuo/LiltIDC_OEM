// 短信宝配置：账号/密码(MD5)/签名 + 发送测试短信 + 余额查询。
// 测试结果如实展示（成功 toast / 失败显示映射后的可读错误）。
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Loader2, MessageSquare, Wallet } from "lucide-react"

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
import { Separator } from "@/components/ui/separator"
import { Textarea } from "@/components/ui/textarea"
import { api } from "@/lib/api"
import { errorMessage } from "@/lib/error"

export function SmsSettings() {
  const { t } = useTranslation()

  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [sign, setSign] = useState("")
  const [passwordSet, setPasswordSet] = useState(false)
  const [saving, setSaving] = useState(false)

  // 测试发送
  const [testOpen, setTestOpen] = useState(false)
  const [testTo, setTestTo] = useState("")
  const [testContent, setTestContent] = useState("")
  const [testSending, setTestSending] = useState(false)

  // 余额查询
  const [balance, setBalance] = useState<{ sent: number; remain: number } | null>(null)
  const [balanceLoading, setBalanceLoading] = useState(false)
  // 加载失败后的重试令牌（自增触发 effect 重新拉取）
  const [reloadKey, setReloadKey] = useState(0)

  // 挂载时加载配置：setState 全部发生在异步回调中，
  // 满足 react-hooks/set-state-in-effect（避免同步级联渲染）
  useEffect(() => {
    let alive = true
    api.admin.notify
      .getSettings()
      .then((s) => {
        if (!alive) return
        setUsername(s.smsbao.username)
        setSign(s.smsbao.sign)
        setPasswordSet(s.smsbao.password_set)
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
    setSaving(true)
    try {
      await api.admin.notify.saveSettings({ smsbao: { username, password, sign } })
      setPassword("")
      if (password) setPasswordSet(true)
      toast.success(t("settings.sms.saved"))
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
      await api.admin.notify.testSms(testTo.trim(), testContent.trim() || undefined)
      toast.success(t("settings.sms.testSent"))
      setTestOpen(false)
      setTestContent("")
    } catch (err) {
      // 失败原因如实显示（短信宝错误码已在后端映射为可读信息）
      toast.error(errorMessage(err, t("common.unknownError")))
    } finally {
      setTestSending(false)
    }
  }

  async function handleBalance() {
    setBalanceLoading(true)
    try {
      const r = await api.admin.notify.smsBalance()
      setBalance(r)
    } catch (err) {
      setBalance(null)
      toast.error(errorMessage(err, t("common.unknownError")))
    } finally {
      setBalanceLoading(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <MessageSquare className="size-4" />
          {t("settings.sms.title")}
          <Badge variant={passwordSet ? "default" : "secondary"} className="ml-1 font-normal">
            {passwordSet ? t("settings.sms.passwordSet") : t("settings.sms.passwordNotSet")}
          </Badge>
        </CardTitle>
        <CardDescription>{t("settings.sms.description")}</CardDescription>
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
            <Label htmlFor="sms-username">{t("settings.sms.username")}</Label>
            <Input
              id="sms-username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              disabled={loading || saving}
              autoComplete="off"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="sms-password">{t("settings.sms.password")}</Label>
            <Input
              id="sms-password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder={passwordSet ? t("settings.sms.passwordPlaceholder") : ""}
              disabled={loading || saving}
              autoComplete="new-password"
            />
            <p className="text-xs text-muted-foreground">{t("settings.sms.passwordHelp")}</p>
          </div>
          <div className="space-y-2">
            <Label htmlFor="sms-sign">{t("settings.sms.sign")}</Label>
            <Input
              id="sms-sign"
              value={sign}
              onChange={(e) => setSign(e.target.value)}
              disabled={loading || saving}
              autoComplete="off"
            />
            <p className="text-xs text-muted-foreground">{t("settings.sms.signHelp")}</p>
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
              t("settings.sms.save")
            )}
          </Button>
          <Button variant="outline" onClick={() => setTestOpen(true)} disabled={loading}>
            {t("settings.sms.testSend")}
          </Button>
          <Button variant="outline" onClick={handleBalance} disabled={loading || balanceLoading}>
            <Wallet />
            {balanceLoading ? <Loader2 className="animate-spin" /> : null}
            {t("settings.sms.balance")}
          </Button>
        </div>

        {balance && (
          <p className="text-sm text-muted-foreground">
            {t("settings.sms.balanceResult", { sent: balance.sent, remain: balance.remain })}
          </p>
        )}

        <Separator />
        <p className="text-xs text-muted-foreground">{t("settings.sms.testDescription")}</p>
          </>
        )}
      </CardContent>

      {/* 测试短信对话框 */}
      <Dialog open={testOpen} onOpenChange={setTestOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("settings.sms.testTitle")}</DialogTitle>
            <DialogDescription>{t("settings.sms.testDescription")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="test-sms-to">{t("settings.sms.testTo")}</Label>
              <Input
                id="test-sms-to"
                value={testTo}
                onChange={(e) => setTestTo(e.target.value)}
                placeholder="13800138000"
                disabled={testSending}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="test-sms-content">{t("settings.sms.testContent")}</Label>
              <Textarea
                id="test-sms-content"
                value={testContent}
                onChange={(e) => setTestContent(e.target.value)}
                rows={3}
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
              {t("settings.sms.testSubmit")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  )
}
