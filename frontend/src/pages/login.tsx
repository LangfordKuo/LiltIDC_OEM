// 管理员登录页：真实调用 /api/auth/login，成功后存 token 并进入 /admin。
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
import { ApiError, api, setToken } from "@/lib/api"
import { errorMessage } from "@/lib/error"

export default function LoginPage() {
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
      setError(t("login.usernameRequired"))
      return
    }
    if (!password) {
      setError(t("login.passwordRequired"))
      return
    }
    setError(null)
    setLoading(true)
    try {
      const result = await api.auth.login(username.trim(), password)
      setToken(result.token)
      const name = result.admin.real_name || result.admin.username
      toast.success(t("login.welcome", { name }))
      // 优先回到守卫记录的来源页（仅允许 /admin 前缀，防开放重定向）
      const from = (location.state as { from?: { pathname?: string } } | null)?.from?.pathname
      navigate(from && from.startsWith("/admin") ? from : "/admin", { replace: true })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : errorMessage(err, t("common.unknownError")))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="flex min-h-screen flex-col">
      <header className="flex items-center justify-between px-6 py-4">
        <SiteLogo />
        <div className="flex items-center gap-1">
          <LanguageSwitcher />
        </div>
      </header>

      <main className="flex flex-1 items-center justify-center px-4 pb-16">
        <Card className="w-full max-w-md">
          <CardHeader className="text-center">
            <CardTitle className="text-xl">{t("login.title")}</CardTitle>
            <CardDescription>{t("login.subtitle")}</CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleSubmit} className="space-y-4">
              {error && (
                <Alert variant="destructive">
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              )}
              <div className="space-y-2">
                <Label htmlFor="username">{t("login.username")}</Label>
                <Input
                  id="username"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  autoComplete="username"
                  autoFocus
                  disabled={loading}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="password">{t("login.password")}</Label>
                <Input
                  id="password"
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
                  t("login.submit")
                )}
              </Button>
            </form>
          </CardContent>
          <CardFooter className="justify-center">
            <Button asChild variant="link" size="sm" className="text-muted-foreground">
              <Link to="/">
                <ArrowLeft />
                {t("login.backHome")}
              </Link>
            </Button>
          </CardFooter>
        </Card>
      </main>
    </div>
  )
}
