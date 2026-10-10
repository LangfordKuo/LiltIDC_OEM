// 会员注册页（批 3-β；后端 API 批 3-α 已就绪，契约 12.0-12.2）：
// 两步注册——① 发送验证码（email/sms 通道；SMTP/短信未配置时如实展示后端错误）
// ② 提交注册。同 IP 当日发送 ≥5 条后端要求图形验证码（captcha_required），
// 前端按错误码动态加载图形码输入。注册成功后引导去会员登录（不自动登录）。
import { useCallback, useEffect, useRef, useState } from "react"
import { Link, useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { ArrowLeft, Loader2, Mail, MessageSquare } from "lucide-react"

import { Alert, AlertDescription } from "@/components/ui/alert"
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
import { LanguageSwitcher } from "@/components/language-switcher"
import { SiteLogo } from "@/components/site-logo"
import { ApiError, api, type CaptchaData } from "@/lib/api"
import { errorMessage } from "@/lib/error"

/** 重发倒计时秒数（与后端 60s 频控一致）。 */
const RESEND_SECONDS = 60

export default function ClientRegisterPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()

  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [confirmPassword, setConfirmPassword] = useState("")
  const [email, setEmail] = useState("")
  const [phone, setPhone] = useState("")
  const [channel, setChannel] = useState<"email" | "sms">("email")
  const [code, setCode] = useState("")
  const [sending, setSending] = useState(false)
  const [countdown, setCountdown] = useState(0)
  const [captcha, setCaptcha] = useState<CaptchaData | null>(null)
  const [captchaAnswer, setCaptchaAnswer] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const timerRef = useRef<number | null>(null)

  // 倒计时（组件卸载时清理）
  useEffect(() => {
    if (countdown <= 0) return
    timerRef.current = window.setTimeout(() => setCountdown((c) => c - 1), 1000)
    return () => {
      if (timerRef.current) window.clearTimeout(timerRef.current)
    }
  }, [countdown])

  const loadCaptcha = useCallback(async () => {
    try {
      const data = await api.client.captcha()
      setCaptcha(data)
      setCaptchaAnswer("")
    } catch {
      setCaptcha(null)
    }
  }, [])

  async function handleSendCode() {
    setError(null)
    const to = channel === "email" ? email.trim() : phone.trim()
    if (!to) {
      setError(channel === "email" ? t("register.emailRequired") : t("register.phoneRequired"))
      return
    }
    if (channel === "sms" && !username.trim()) {
      // 短信通道注册前后端要求用户名预占查重（契约 12.1）
      setError(t("register.usernameRequired"))
      return
    }
    setSending(true)
    try {
      await api.client.sendCode({
        purpose: "register",
        channel,
        to,
        username: channel === "sms" ? username.trim() : undefined,
        email: channel === "email" ? to : undefined,
        captcha_id: captcha?.captcha_id,
        captcha_answer: captcha ? captchaAnswer.trim() : undefined,
      })
      toast.success(t("register.codeSent"))
      setCountdown(RESEND_SECONDS)
      setCaptcha(null)
      setCaptchaAnswer("")
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : errorMessage(err, t("common.unknownError"))
      setError(msg)
      // 需要图形验证码（或图形码错误）→ 加载/刷新图形码
      if (err instanceof ApiError && (err.code === "captcha_required" || err.code === "captcha_invalid")) {
        await loadCaptcha()
      }
    } finally {
      setSending(false)
    }
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    if (password !== confirmPassword) {
      setError(t("register.pwdMismatch"))
      return
    }
    setSubmitting(true)
    try {
      await api.client.register({
        username: username.trim(),
        password,
        confirm_password: confirmPassword,
        email: channel === "email" ? email.trim() : email.trim() || undefined,
        phone: channel === "sms" ? phone.trim() : phone.trim() || undefined,
        verify_channel: channel,
        verify_code: code.trim(),
      })
      toast.success(t("register.success"))
      navigate("/client-login")
    } catch (err) {
      setError(err instanceof ApiError ? err.message : errorMessage(err, t("common.unknownError")))
    } finally {
      setSubmitting(false)
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
            <CardTitle className="text-xl">{t("register.title")}</CardTitle>
            <CardDescription>{t("register.subtitle")}</CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleSubmit} className="space-y-4">
              {error && (
                <Alert variant="destructive">
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              )}
              <div className="space-y-2">
                <Label htmlFor="reg-username">{t("register.username")}</Label>
                <Input
                  id="reg-username"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  autoComplete="username"
                  disabled={submitting}
                />
                <p className="text-xs text-muted-foreground">{t("register.usernameHelp")}</p>
              </div>
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="reg-password">{t("register.password")}</Label>
                  <Input
                    id="reg-password"
                    type="password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    autoComplete="new-password"
                    disabled={submitting}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="reg-confirm">{t("register.confirmPassword")}</Label>
                  <Input
                    id="reg-confirm"
                    type="password"
                    value={confirmPassword}
                    onChange={(e) => setConfirmPassword(e.target.value)}
                    autoComplete="new-password"
                    disabled={submitting}
                  />
                </div>
              </div>
              <p className="text-xs text-muted-foreground">{t("register.pwdRule")}</p>

              {/* 验证通道切换 */}
              <div className="flex gap-2">
                <Button
                  type="button"
                  size="sm"
                  variant={channel === "email" ? "default" : "outline"}
                  onClick={() => setChannel("email")}
                >
                  <Mail />
                  {t("register.channelEmail")}
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant={channel === "sms" ? "default" : "outline"}
                  onClick={() => setChannel("sms")}
                >
                  <MessageSquare />
                  {t("register.channelSms")}
                </Button>
              </div>

              <div className="space-y-2">
                <Label htmlFor="reg-email">{t("register.email")}</Label>
                <Input
                  id="reg-email"
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  autoComplete="email"
                  disabled={submitting}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="reg-phone">
                  {t("register.phone")}
                  <span className="ml-1 text-xs text-muted-foreground">
                    {channel === "sms" ? "" : `(${t("common.optional")})`}
                  </span>
                </Label>
                <Input
                  id="reg-phone"
                  value={phone}
                  onChange={(e) => setPhone(e.target.value)}
                  autoComplete="tel"
                  disabled={submitting}
                />
              </div>

              {/* 验证码 + 发送按钮（倒计时） */}
              <div className="space-y-2">
                <Label htmlFor="reg-code">{t("register.verifyCode")}</Label>
                <div className="flex gap-2">
                  <Input
                    id="reg-code"
                    inputMode="numeric"
                    maxLength={6}
                    value={code}
                    onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))}
                    disabled={submitting}
                    className="flex-1"
                  />
                  <Button
                    type="button"
                    variant="outline"
                    className="shrink-0"
                    disabled={sending || countdown > 0}
                    onClick={() => void handleSendCode()}
                  >
                    {sending ? (
                      <Loader2 className="animate-spin" />
                    ) : countdown > 0 ? (
                      t("register.resendIn", { seconds: countdown })
                    ) : (
                      t("register.sendCode")
                    )}
                  </Button>
                </div>
              </div>

              {/* 图形验证码（后端 captcha_required/captcha_invalid 时动态出现） */}
              {captcha && (
                <div className="space-y-2">
                  <Label htmlFor="reg-captcha">{t("register.captcha")}</Label>
                  <div className="flex gap-2">
                    <Input
                      id="reg-captcha"
                      value={captchaAnswer}
                      onChange={(e) => setCaptchaAnswer(e.target.value)}
                      maxLength={4}
                      className="flex-1"
                    />
                    <button
                      type="button"
                      onClick={() => void loadCaptcha()}
                      title={t("register.captchaRefresh")}
                      className="shrink-0"
                    >
                      <img
                        src={captcha.image}
                        alt={t("register.captchaAlt")}
                        className="h-9 w-30 rounded-md border"
                      />
                    </button>
                  </div>
                </div>
              )}

              <Button type="submit" className="w-full" disabled={submitting}>
                {submitting ? (
                  <>
                    <Loader2 className="animate-spin" />
                    {t("common.submitting")}
                  </>
                ) : (
                  t("register.submit")
                )}
              </Button>
            </form>
          </CardContent>
          <div className="flex justify-center gap-4 pb-6 text-sm">
            <Button asChild variant="link" size="sm" className="text-muted-foreground">
              <Link to="/client-login">
                <ArrowLeft />
                {t("register.toLogin")}
              </Link>
            </Button>
          </div>
        </Card>
      </main>
    </div>
  )
}
