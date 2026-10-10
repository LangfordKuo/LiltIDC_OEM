// 会员登录页（批 3-β；契约 12.3）：用户名/邮箱/手机号 + 密码三合一登录，
// 成功存会员 token（liltidc_client_token，与管理员 token 隔离）并回到来源页。
import { useState } from "react"
import { Link, useLocation, useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { ArrowLeft, Loader2 } from "lucide-react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { LanguageSwitcher } from "@/components/language-switcher"
import { SiteLogo } from "@/components/site-logo"
import { ApiError, api, setClientToken } from "@/lib/api"
import { errorMessage } from "@/lib/error"

export default function ClientLoginPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const location = useLocation()

  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!username.trim()) {
      setError(t("clientLogin.usernameRequired"))
      return
    }
    if (!password) {
      setError(t("clientLogin.passwordRequired"))
      return
    }
    setError(null)
    setLoading(true)
    try {
      const result = await api.client.login(username.trim(), password)
      setClientToken(result.token)
      toast.success(t("clientLogin.welcome", { name: result.client.username }))
      // 回到守卫记录的来源页（防开放重定向：仅允许站内相对路径且非登录/注册页）
      const from = (location.state as { from?: { pathname?: string } } | null)?.from?.pathname
      const target =
        from && from.startsWith("/") && !from.startsWith("/client-login") && !from.startsWith("/register")
          ? from
          : "/console"
      navigate(target, { replace: true })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : errorMessage(err, t("common.unknownError")))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="flex min-h-screen flex-col">
      <header className="flex items-center justify-between px-6 py-4">
        <Link to="/store">
          <SiteLogo />
        </Link>
        <LanguageSwitcher />
      </header>

      <main className="flex flex-1 items-center justify-center px-4 pb-16">
        <Card className="w-full max-w-md">
          <CardHeader className="text-center">
            <CardTitle className="text-xl">{t("clientLogin.title")}</CardTitle>
            <CardDescription>{t("clientLogin.subtitle")}</CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleSubmit} className="space-y-4">
              {error && (
                <Alert variant="destructive">
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              )}
              <div className="space-y-2">
                <Label htmlFor="cl-username">{t("clientLogin.username")}</Label>
                <Input
                  id="cl-username"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  autoComplete="username"
                  autoFocus
                  disabled={loading}
                />
                <p className="text-xs text-muted-foreground">{t("clientLogin.usernameHelp")}</p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="cl-password">{t("clientLogin.password")}</Label>
                <Input
                  id="cl-password"
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  autoComplete="current-password"
                  disabled={loading}
                />
              </div>
              <Button type="submit" className="w-full" disabled={loading}>
                {loading ? (
                  <>
                    <Loader2 className="animate-spin" />
                    {t("common.submitting")}
                  </>
                ) : (
                  t("clientLogin.submit")
                )}
              </Button>
            </form>
          </CardContent>
          <CardFooter className="justify-center gap-2">
            <Button asChild variant="link" size="sm" className="text-muted-foreground">
              <Link to="/store">
                <ArrowLeft />
                {t("clientLogin.backStore")}
              </Link>
            </Button>
            <Button asChild variant="link" size="sm">
              <Link to="/register">{t("clientLogin.toRegister")}</Link>
            </Button>
          </CardFooter>
        </Card>
      </main>
    </div>
  )
}
