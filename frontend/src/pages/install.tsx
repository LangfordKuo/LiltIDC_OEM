// 安装向导页（本批核心交付）：
// 步骤 ① 数据库连接（dbtest）→ ② 站点信息与管理账号 → ③ 完成（complete）。
// 已安装时访问本页跳回首页；「刚安装完成」状态例外，保证第 ③ 步成功页可见。
import { useState, type ReactNode } from "react"
import { Navigate, useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { Check, Database, Loader2, Palette, PartyPopper } from "lucide-react"

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
import { Separator } from "@/components/ui/separator"
import { LanguageSwitcher } from "@/components/language-switcher"
import { SiteLogo } from "@/components/site-logo"
import { ApiError, api } from "@/lib/api"
import { refreshBrand, useBrand } from "@/lib/brand"
import { errorMessage } from "@/lib/error"
import { setInstallState, useInstallState } from "@/lib/install-state"
import { cn } from "@/lib/utils"

type FieldErrors = Record<string, string>

// 与后端契约一致的校验正则（docs/api-contract.md 1.3）
const DBNAME_RE = /^[A-Za-z0-9_-]{1,64}$/
const ADMIN_USERNAME_RE = /^[A-Za-z0-9_]{3,32}$/
const COLOR_RE = /^#[0-9a-fA-F]{6}$/
const EMAIL_RE = /^[^@\s]+@[^@\s]+\.[^@\s]+$/

export default function InstallPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const brand = useBrand()
  const installed = useInstallState()

  // step: 0=数据库 1=站点与管理员 2=完成
  const [step, setStep] = useState(0)
  // 「刚完成安装」标记：豁免已安装重定向，让第 ③ 步成功页可见
  const [justInstalled, setJustInstalled] = useState(false)

  // ---- 步骤 ① 数据库 ----
  const [db, setDb] = useState({
    host: "127.0.0.1",
    port: "3306",
    user: "root",
    password: "",
    dbname: "liltidc_oem",
  })
  const [dbErrors, setDbErrors] = useState<FieldErrors>({})
  const [dbError, setDbError] = useState<string | null>(null)
  const [dbVersion, setDbVersion] = useState<string | null>(null)
  const [testing, setTesting] = useState(false)

  // ---- 步骤 ② 站点与管理员 ----
  const [site, setSite] = useState({
    siteName: brand?.site_name || "LiltIDC",
    primaryColor: brand?.primary_color || "#2563eb",
    logo: "",
    adminUsername: "admin",
    adminPassword: "",
    confirmPassword: "",
    email: "",
    realName: "",
  })
  const [siteErrors, setSiteErrors] = useState<FieldErrors>({})
  const [installError, setInstallError] = useState<string | null>(null)
  const [installing, setInstalling] = useState(false)

  // 已安装（且不是刚装完）→ 跳回首页（渲染期同步判断，无闪烁）
  if (installed === true && !justInstalled) {
    return <Navigate to="/" replace />
  }

  // ================= 步骤 ① =================
  function validateDb(): boolean {
    const e: FieldErrors = {}
    if (!db.host.trim()) e.host = t("install.errRequired", { field: t("install.dbHost") })
    const port = Number(db.port)
    if (!db.port.trim() || !Number.isInteger(port) || port < 1 || port > 65535) {
      e.port = t("install.errPort")
    }
    if (!db.user.trim()) e.user = t("install.errRequired", { field: t("install.dbUser") })
    if (db.dbname.trim() && !DBNAME_RE.test(db.dbname.trim())) {
      e.dbname = t("install.errDbName")
    }
    setDbErrors(e)
    return Object.keys(e).length === 0
  }

  async function handleDbNext() {
    if (!validateDb()) return
    setTesting(true)
    setDbError(null)
    setDbVersion(null)
    try {
      const r = await api.install.dbtest({
        host: db.host.trim(),
        port: Number(db.port),
        user: db.user.trim(),
        password: db.password,
        dbname: db.dbname.trim() || undefined,
      })
      setDbVersion(r.version)
      setStep(1)
    } catch (err) {
      // 失败原因如实展示（含底层数据库错误，如 Access denied）
      setDbError(err instanceof ApiError ? err.message : errorMessage(err, t("common.unknownError")))
    } finally {
      setTesting(false)
    }
  }

  // ================= 步骤 ② =================
  function validateSite(): boolean {
    const e: FieldErrors = {}
    const name = site.siteName.trim()
    if (!name || [...name].length > 64) e.siteName = t("install.errSiteName")
    if (!COLOR_RE.test(site.primaryColor.trim())) e.primaryColor = t("install.errColor")
    if (!ADMIN_USERNAME_RE.test(site.adminUsername.trim())) {
      e.adminUsername = t("install.errAdminUsername")
    }
    if (site.adminPassword.length < 8 || site.adminPassword.length > 72) {
      e.adminPassword = t("install.errPasswordLength")
    }
    if (site.adminPassword !== site.confirmPassword) {
      e.confirmPassword = t("install.errPasswordMismatch")
    }
    if (site.email.trim() && !EMAIL_RE.test(site.email.trim())) {
      e.email = t("install.errEmail")
    }
    setSiteErrors(e)
    return Object.keys(e).length === 0
  }

  async function handleInstall() {
    if (!validateSite()) return
    setInstalling(true)
    setInstallError(null)
    try {
      await api.install.complete({
        database: {
          host: db.host.trim(),
          port: Number(db.port),
          user: db.user.trim(),
          password: db.password,
          dbname: db.dbname.trim(),
        },
        site_name: site.siteName.trim(),
        logo: site.logo.trim(),
        primary_color: site.primaryColor.trim().toLowerCase(),
        admin: {
          username: site.adminUsername.trim(),
          password: site.adminPassword,
          email: site.email.trim() || undefined,
          real_name: site.realName.trim() || undefined,
        },
      })
      // 先豁免重定向，再切换全局安装态并刷新品牌（站名/主色立即生效）
      setJustInstalled(true)
      setInstallState(true)
      await refreshBrand()
      setStep(2)
    } catch (err) {
      setInstallError(
        err instanceof ApiError ? err.message : errorMessage(err, t("common.unknownError")),
      )
      // already_installed：说明系统在别处完成了安装，刷新状态让守卫接管
      if (err instanceof ApiError && err.code === "already_installed") {
        setJustInstalled(false)
        setInstallState(true)
      }
    } finally {
      setInstalling(false)
    }
  }

  // ================= 渲染 =================
  const steps = [
    { labelKey: "install.stepDatabase", icon: Database },
    { labelKey: "install.stepSite", icon: Palette },
    { labelKey: "install.stepDone", icon: PartyPopper },
  ]

  return (
    <div className="flex min-h-screen flex-col">
      <header className="flex items-center justify-between px-6 py-4">
        <SiteLogo />
        <div className="flex items-center gap-1">
          <LanguageSwitcher />
        </div>
      </header>

      <main className="flex flex-1 items-start justify-center px-4 pb-16 pt-4">
        <div className="w-full max-w-2xl space-y-6">
          <div className="text-center">
            <h1 className="text-2xl font-bold">{t("install.title")}</h1>
            <p className="mt-1 text-sm text-muted-foreground">{t("install.subtitle")}</p>
          </div>

          {/* 步骤进度指示 */}
          <ol className="flex items-center justify-center gap-2">
            {steps.map((s, i) => {
              const state = i < step ? "done" : i === step ? "current" : "todo"
              return (
                <li key={s.labelKey} className="flex items-center gap-2">
                  {i > 0 && <Separator className="w-8 md:w-16" />}
                  <div className="flex items-center gap-2">
                    <span
                      className={cn(
                        "flex size-8 items-center justify-center rounded-full border text-sm font-medium",
                        state === "done" && "border-primary bg-primary text-primary-foreground",
                        state === "current" && "border-primary text-primary",
                        state === "todo" && "border-muted-foreground/30 text-muted-foreground",
                      )}
                    >
                      {state === "done" ? <Check className="size-4" /> : <s.icon className="size-4" />}
                    </span>
                    <span
                      className={cn(
                        "hidden text-sm md:inline",
                        state === "current" ? "font-semibold" : "text-muted-foreground",
                      )}
                    >
                      {t(s.labelKey)}
                    </span>
                  </div>
                </li>
              )
            })}
          </ol>

          {/* ============ 步骤 ① 数据库连接 ============ */}
          {step === 0 && (
            <Card>
              <CardHeader>
                <CardTitle>{t("install.stepDatabase")}</CardTitle>
                <CardDescription>{t("install.dbNameHelp")}</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                {dbError && (
                  <Alert variant="destructive">
                    <AlertDescription className="break-all">{dbError}</AlertDescription>
                  </Alert>
                )}
                {dbVersion && (
                  <Alert>
                    <AlertDescription>{t("install.dbConnected", { version: dbVersion })}</AlertDescription>
                  </Alert>
                )}
                <div className="grid gap-4 sm:grid-cols-[2fr_1fr]">
                  <Field
                    id="db-host"
                    label={t("install.dbHost")}
                    error={dbErrors.host}
                    input={
                      <Input
                        id="db-host"
                        value={db.host}
                        onChange={(e) => setDb({ ...db, host: e.target.value })}
                        disabled={testing}
                        placeholder="127.0.0.1"
                      />
                    }
                  />
                  <Field
                    id="db-port"
                    label={t("install.dbPort")}
                    error={dbErrors.port}
                    input={
                      <Input
                        id="db-port"
                        value={db.port}
                        onChange={(e) => setDb({ ...db, port: e.target.value })}
                        disabled={testing}
                        inputMode="numeric"
                      />
                    }
                  />
                </div>
                <div className="grid gap-4 sm:grid-cols-2">
                  <Field
                    id="db-user"
                    label={t("install.dbUser")}
                    error={dbErrors.user}
                    input={
                      <Input
                        id="db-user"
                        value={db.user}
                        onChange={(e) => setDb({ ...db, user: e.target.value })}
                        disabled={testing}
                        autoComplete="off"
                      />
                    }
                  />
                  <Field
                    id="db-password"
                    label={t("install.dbPassword")}
                    input={
                      <Input
                        id="db-password"
                        type="password"
                        value={db.password}
                        onChange={(e) => setDb({ ...db, password: e.target.value })}
                        disabled={testing}
                        autoComplete="new-password"
                      />
                    }
                  />
                </div>
                <Field
                  id="db-dbname"
                  label={t("install.dbName")}
                  error={dbErrors.dbname}
                  input={
                    <Input
                      id="db-dbname"
                      value={db.dbname}
                      onChange={(e) => setDb({ ...db, dbname: e.target.value })}
                      disabled={testing}
                    />
                  }
                />
              </CardContent>
              <CardFooter className="justify-end">
                <Button onClick={handleDbNext} disabled={testing}>
                  {testing ? (
                    <>
                      <Loader2 className="animate-spin" />
                      {t("install.testing")}
                    </>
                  ) : (
                    t("install.testAndNext")
                  )}
                </Button>
              </CardFooter>
            </Card>
          )}

          {/* ============ 步骤 ② 站点与管理员 ============ */}
          {step === 1 && (
            <Card>
              <CardHeader>
                <CardTitle>{t("install.stepSite")}</CardTitle>
                <CardDescription>{t("install.installNote")}</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                {installError && (
                  <Alert variant="destructive">
                    <AlertDescription className="break-all">{installError}</AlertDescription>
                  </Alert>
                )}
                <div className="grid gap-4 sm:grid-cols-2">
                  <Field
                    id="site-name"
                    label={t("install.siteName")}
                    error={siteErrors.siteName}
                    input={
                      <Input
                        id="site-name"
                        value={site.siteName}
                        onChange={(e) => setSite({ ...site, siteName: e.target.value })}
                        disabled={installing}
                      />
                    }
                  />
                  <Field
                    id="primary-color"
                    label={t("install.primaryColor")}
                    error={siteErrors.primaryColor}
                    input={
                      <div className="flex gap-2">
                        <Input
                          id="primary-color"
                          type="color"
                          value={COLOR_RE.test(site.primaryColor) ? site.primaryColor : "#2563eb"}
                          onChange={(e) => setSite({ ...site, primaryColor: e.target.value })}
                          disabled={installing}
                          className="w-14 p-1"
                          aria-label={t("install.primaryColor")}
                        />
                        <Input
                          value={site.primaryColor}
                          onChange={(e) => setSite({ ...site, primaryColor: e.target.value })}
                          disabled={installing}
                          placeholder="#2563eb"
                          className="font-mono"
                          aria-label={`${t("install.primaryColor")} hex`}
                        />
                      </div>
                    }
                  />
                </div>
                <Field
                  id="logo"
                  label={`${t("install.logo")}（${t("common.optional")}）`}
                  hint={t("install.logoHelp")}
                  input={
                    <Input
                      id="logo"
                      value={site.logo}
                      onChange={(e) => setSite({ ...site, logo: e.target.value })}
                      disabled={installing}
                      placeholder="https://example.com/logo.png"
                    />
                  }
                />
                <Separator />
                <div className="grid gap-4 sm:grid-cols-2">
                  <Field
                    id="admin-username"
                    label={t("install.adminUsername")}
                    error={siteErrors.adminUsername}
                    hint={t("install.adminUsernameHelp")}
                    input={
                      <Input
                        id="admin-username"
                        value={site.adminUsername}
                        onChange={(e) => setSite({ ...site, adminUsername: e.target.value })}
                        disabled={installing}
                        autoComplete="off"
                      />
                    }
                  />
                  <Field
                    id="admin-email"
                    label={`${t("install.adminEmail")}（${t("common.optional")}）`}
                    error={siteErrors.email}
                    input={
                      <Input
                        id="admin-email"
                        type="email"
                        value={site.email}
                        onChange={(e) => setSite({ ...site, email: e.target.value })}
                        disabled={installing}
                        autoComplete="off"
                      />
                    }
                  />
                  <Field
                    id="admin-password"
                    label={t("install.adminPassword")}
                    error={siteErrors.adminPassword}
                    hint={t("install.adminPasswordHelp")}
                    input={
                      <Input
                        id="admin-password"
                        type="password"
                        value={site.adminPassword}
                        onChange={(e) => setSite({ ...site, adminPassword: e.target.value })}
                        disabled={installing}
                        autoComplete="new-password"
                      />
                    }
                  />
                  <Field
                    id="confirm-password"
                    label={t("install.confirmPassword")}
                    error={siteErrors.confirmPassword}
                    input={
                      <Input
                        id="confirm-password"
                        type="password"
                        value={site.confirmPassword}
                        onChange={(e) => setSite({ ...site, confirmPassword: e.target.value })}
                        disabled={installing}
                        autoComplete="new-password"
                      />
                    }
                  />
                  <Field
                    id="admin-realname"
                    label={`${t("install.adminRealName")}（${t("common.optional")}）`}
                    input={
                      <Input
                        id="admin-realname"
                        value={site.realName}
                        onChange={(e) => setSite({ ...site, realName: e.target.value })}
                        disabled={installing}
                        autoComplete="off"
                      />
                    }
                  />
                </div>
              </CardContent>
              <CardFooter className="justify-between">
                <Button variant="outline" onClick={() => setStep(0)} disabled={installing}>
                  {t("install.prev")}
                </Button>
                <Button onClick={handleInstall} disabled={installing}>
                  {installing ? (
                    <>
                      <Loader2 className="animate-spin" />
                      {t("install.installing")}
                    </>
                  ) : (
                    t("install.installNow")
                  )}
                </Button>
              </CardFooter>
            </Card>
          )}

          {/* ============ 步骤 ③ 完成 ============ */}
          {step === 2 && (
            <Card>
              <CardHeader className="items-center text-center">
                <div className="flex size-14 items-center justify-center rounded-full bg-primary text-primary-foreground">
                  <Check className="size-8" />
                </div>
                <CardTitle className="mt-2">{t("install.successTitle")}</CardTitle>
                <CardDescription>{t("install.successMessage")}</CardDescription>
              </CardHeader>
              <CardFooter className="justify-center gap-3">
                <Button variant="outline" onClick={() => navigate("/login")}>
                  {t("install.goLogin")}
                </Button>
                <Button onClick={() => navigate("/")}>{t("install.goHome")}</Button>
              </CardFooter>
            </Card>
          )}
        </div>
      </main>
    </div>
  )
}

/** 表单字段小组件：Label + 输入控件 + 错误/提示文本。 */
function Field({
  id,
  label,
  error,
  hint,
  input,
}: {
  id: string
  label: string
  error?: string
  hint?: string
  input: ReactNode
}) {
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{label}</Label>
      {input}
      {error ? (
        <p className="text-sm text-destructive">{error}</p>
      ) : hint ? (
        <p className="text-xs text-muted-foreground">{hint}</p>
      ) : null}
    </div>
  )
}
