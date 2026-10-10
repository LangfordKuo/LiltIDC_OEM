// 下游推送页签：展示 host/sync 回调地址与下游密钥（可复制/重置），
// 供管理员登记到上游魔方财务。本批不自动登记（批 3 下单链路一起做）。
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Copy, KeyRound, Loader2, Webhook } from "lucide-react"

import { Alert, AlertDescription } from "@/components/ui/alert"
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
import { api } from "@/lib/api"
import { errorMessage } from "@/lib/error"

export function PushTab() {
  const { t } = useTranslation()

  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [token, setToken] = useState("")
  const [syncPath, setSyncPath] = useState("/api/upstream/host/sync")
  const [resetOpen, setResetOpen] = useState(false)
  const [resetting, setResetting] = useState(false)

  useEffect(() => {
    let alive = true
    api.admin.upstream
      .getConfig()
      .then((cfg) => {
        if (!alive) return
        setToken(cfg.downstream_token)
        setSyncPath(cfg.sync_path)
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
  }, [t])

  // 完整回调地址 = 本站源 + 路径（生产经 Nginx 反代即公网可达地址）
  const callbackUrl = `${window.location.origin}${syncPath}`

  async function copyText(text: string) {
    try {
      await navigator.clipboard.writeText(text)
      toast.success(t("upstream.push.copied"))
    } catch {
      toast.error(t("upstream.push.copyFailed"))
    }
  }

  async function handleReset() {
    setResetting(true)
    try {
      const r = await api.admin.upstream.resetToken()
      setToken(r.downstream_token)
      setResetOpen(false)
      toast.success(t("upstream.push.resetDone"))
    } catch (err) {
      toast.error(errorMessage(err, t("common.unknownError")))
    } finally {
      setResetting(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Webhook className="size-4" />
          {t("upstream.push.title")}
        </CardTitle>
        <CardDescription>{t("upstream.push.description")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {loadError && (
          <Alert variant="destructive">
            <AlertDescription>{loadError}</AlertDescription>
          </Alert>
        )}
        {loading ? (
          <div className="flex items-center justify-center gap-2 py-12 text-muted-foreground">
            <Loader2 className="size-4 animate-spin" />
            <span className="text-sm">{t("common.loading")}</span>
          </div>
        ) : (
          <>
            <div className="space-y-2">
              <Label htmlFor="callback-url">{t("upstream.push.callbackUrl")}</Label>
              <div className="flex gap-2">
                <Input id="callback-url" readOnly value={callbackUrl} className="font-mono text-sm" />
                <Button variant="outline" size="icon" onClick={() => void copyText(callbackUrl)}>
                  <Copy />
                </Button>
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="downstream-token">{t("upstream.push.token")}</Label>
              <div className="flex gap-2">
                <Input id="downstream-token" readOnly value={token} className="font-mono text-sm" />
                <Button variant="outline" size="icon" onClick={() => void copyText(token)}>
                  <Copy />
                </Button>
                <Button variant="outline" onClick={() => setResetOpen(true)}>
                  <KeyRound />
                  {t("upstream.push.reset")}
                </Button>
              </div>
              <p className="text-xs text-muted-foreground">{t("upstream.push.tokenHelp")}</p>
            </div>
            <Alert>
              <AlertDescription>{t("upstream.push.notRegistered")}</AlertDescription>
            </Alert>
          </>
        )}
      </CardContent>

      {/* 重置确认对话框 */}
      <Dialog open={resetOpen} onOpenChange={setResetOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("upstream.push.resetConfirmTitle")}</DialogTitle>
            <DialogDescription>{t("upstream.push.resetConfirmDescription")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setResetOpen(false)} disabled={resetting}>
              {t("common.cancel")}
            </Button>
            <Button variant="destructive" onClick={handleReset} disabled={resetting}>
              {resetting && <Loader2 className="animate-spin" />}
              {t("upstream.push.reset")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  )
}
