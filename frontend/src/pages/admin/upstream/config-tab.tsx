// 上游配置页签：地址/账号/API 密钥/本站地址 + 保存 + 测试连接（真实调上游登录+余额，结果如实显示）。
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Cloud, Loader2, Wallet } from "lucide-react"

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
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { api, type UpstreamTestResult } from "@/lib/api"
import { errorMessage } from "@/lib/error"

export function ConfigTab() {
  const { t } = useTranslation()

  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)
  const [hostname, setHostname] = useState("")
  const [username, setUsername] = useState("")
  const [apiKey, setApiKey] = useState("")
  const [apiKeySet, setApiKeySet] = useState(false)
  // 本站对外地址（批 3-β：开通后 setdownstream 登记的 downstream_url；空=回退后端监听地址）
  const [siteUrl, setSiteUrl] = useState("")
  const [saving, setSaving] = useState(false)

  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<UpstreamTestResult | null>(null)

  // 挂载时加载配置：setState 全部在异步回调中（react-hooks/set-state-in-effect）
  useEffect(() => {
    let alive = true
    api.admin.upstream
      .getConfig()
      .then((cfg) => {
        if (!alive) return
        setHostname(cfg.hostname)
        setUsername(cfg.username)
        setApiKeySet(cfg.api_key_set)
        setSiteUrl(cfg.site_url || "")
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

  function handleRetry() {
    setLoading(true)
    setReloadKey((k) => k + 1)
  }

  async function handleSave() {
    setSaving(true)
    try {
      await api.admin.upstream.saveConfig({
        hostname,
        username,
        api_key: apiKey,
        site_url: siteUrl.trim(),
      })
      setApiKey("")
      if (apiKey) setApiKeySet(true)
      toast.success(t("upstream.config.saved"))
    } catch (err) {
      toast.error(errorMessage(err, t("common.unknownError")))
    } finally {
      setSaving(false)
    }
  }

  async function handleTest() {
    setTesting(true)
    setTestResult(null)
    try {
      // 后端设计：测试失败也返回 200 + connected=false + msg（结果即数据）
      const result = await api.admin.upstream.test()
      setTestResult(result)
    } catch (err) {
      // 仅当接口本身异常（未配置为 200 分支，不会到这里；防御网络层失败）
      setTestResult({ connected: false, credit: "", currency: null, msg: errorMessage(err, t("common.unknownError")) })
    } finally {
      setTesting(false)
    }
  }

  async function handleRefreshBalance() {
    setTesting(true)
    try {
      const credit = await api.admin.upstream.credit()
      setTestResult({ connected: true, credit: credit.credit, currency: credit.currency, msg: t("upstream.config.testOk") })
    } catch (err) {
      toast.error(errorMessage(err, t("common.unknownError")))
    } finally {
      setTesting(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Cloud className="size-4" />
          {t("upstream.config.title")}
          <Badge variant={apiKeySet ? "default" : "secondary"} className="ml-1 font-normal">
            {apiKeySet ? t("upstream.config.apiKeySet") : t("upstream.config.apiKeyNotSet")}
          </Badge>
        </CardTitle>
        <CardDescription>{t("upstream.config.description")}</CardDescription>
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
          <div className="flex items-center justify-center gap-2 py-12 text-muted-foreground">
            <Loader2 className="size-4 animate-spin" />
            <span className="text-sm">{t("common.loading")}</span>
          </div>
        ) : (
          <>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="up-hostname">{t("upstream.config.hostname")}</Label>
                <Input
                  id="up-hostname"
                  value={hostname}
                  onChange={(e) => setHostname(e.target.value)}
                  placeholder="https://lyew.com"
                  disabled={saving}
                  autoComplete="off"
                />
                <p className="text-xs text-muted-foreground">{t("upstream.config.hostnameHelp")}</p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="up-username">{t("upstream.config.username")}</Label>
                <Input
                  id="up-username"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  disabled={saving}
                  autoComplete="off"
                />
                <p className="text-xs text-muted-foreground">{t("upstream.config.usernameHelp")}</p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="up-apikey">{t("upstream.config.apiKey")}</Label>
                <Input
                  id="up-apikey"
                  type="password"
                  value={apiKey}
                  onChange={(e) => setApiKey(e.target.value)}
                  placeholder={apiKeySet ? t("upstream.config.apiKeyPlaceholder") : ""}
                  disabled={saving}
                  autoComplete="new-password"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="up-siteurl">{t("upstream.config.siteUrl")}</Label>
                <Input
                  id="up-siteurl"
                  value={siteUrl}
                  onChange={(e) => setSiteUrl(e.target.value)}
                  placeholder="https://idc.example.com"
                  disabled={saving}
                  autoComplete="off"
                />
                <p className="text-xs text-muted-foreground">{t("upstream.config.siteUrlHelp")}</p>
              </div>
            </div>

            <div className="flex flex-wrap items-center gap-2">
              <Button onClick={handleSave} disabled={saving}>
                {saving && <Loader2 className="animate-spin" />}
                {t("upstream.config.save")}
              </Button>
              <Button variant="outline" onClick={handleTest} disabled={testing}>
                {testing ? <Loader2 className="animate-spin" /> : <Wallet />}
                {testing ? t("upstream.config.testing") : t("upstream.config.test")}
              </Button>
              {testResult?.connected && (
                <Button variant="ghost" onClick={handleRefreshBalance} disabled={testing}>
                  {t("upstream.config.refreshBalance")}
                </Button>
              )}
            </div>

            {testResult && (
              <Alert variant={testResult.connected ? "default" : "destructive"}>
                <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
                  <span>
                    {/* connected 时也如实展示后端 msg（可能是「已连接但未返回余额」的部分成功提示） */}
                    {testResult.connected ? testResult.msg : `${t("upstream.config.testFail")}: ${testResult.msg}`}
                  </span>
                  {testResult.connected && testResult.credit && (
                    <span className="font-medium">
                      {t("upstream.config.balance")}:{" "}
                      {t("upstream.config.balanceValue", {
                        prefix: testResult.currency?.prefix ?? "",
                        credit: testResult.credit,
                        suffix: testResult.currency?.suffix ?? "",
                      })}
                    </span>
                  )}
                </AlertDescription>
              </Alert>
            )}
          </>
        )}
      </CardContent>
    </Card>
  )
}
