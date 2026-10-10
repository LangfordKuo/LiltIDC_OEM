// 前台首页（批 3-β 改版）：品牌站名 + 商城入口（商城已上线，替换「即将上线」占位）。
import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { ArrowRight, ShieldCheck, ShoppingBag, Zap } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { LanguageSwitcher } from "@/components/language-switcher"
import { SiteLogo } from "@/components/site-logo"
import { getClientToken } from "@/lib/api"
import { useBrand } from "@/lib/brand"

export default function Home() {
  const { t } = useTranslation()
  const brand = useBrand()
  const siteName = brand?.site_name || "LiltIDC"
  const loggedIn = Boolean(getClientToken())

  return (
    <div className="flex min-h-screen flex-col">
      <header className="flex items-center justify-between px-6 py-4">
        <SiteLogo />
        <div className="flex items-center gap-1">
          <LanguageSwitcher />
          <Button asChild variant="ghost" size="sm" className="ml-1">
            <Link to="/client-login">{t("home.clientLogin")}</Link>
          </Button>
          <Button asChild variant="outline" size="sm">
            <Link to="/login">{t("home.adminLogin")}</Link>
          </Button>
        </div>
      </header>

      <main className="flex flex-1 flex-col items-center justify-center gap-6 px-6 text-center">
        <Badge variant="secondary" className="px-3 py-1 text-sm font-normal">
          {t("home.nowOpen")}
        </Badge>
        <h1 className="text-4xl font-bold tracking-tight md:text-6xl">{siteName}</h1>
        <p className="max-w-xl text-muted-foreground">{t("home.tagline")}</p>
        <div className="flex flex-wrap items-center justify-center gap-3">
          <Button asChild size="lg">
            <Link to={loggedIn ? "/console" : "/store"}>
              <ShoppingBag />
              {loggedIn ? t("home.enterConsole") : t("home.enterStore")}
              <ArrowRight />
            </Link>
          </Button>
          <Button asChild variant="outline" size="lg">
            <Link to="/register">{t("home.register")}</Link>
          </Button>
        </div>
        <div className="mt-8 grid max-w-3xl gap-4 text-left sm:grid-cols-3">
          <div className="rounded-lg border p-4">
            <Zap className="mb-2 size-5 text-primary" />
            <h3 className="text-sm font-semibold">{t("home.featureFast")}</h3>
            <p className="mt-1 text-xs text-muted-foreground">{t("home.featureFastDesc")}</p>
          </div>
          <div className="rounded-lg border p-4">
            <ShieldCheck className="mb-2 size-5 text-primary" />
            <h3 className="text-sm font-semibold">{t("home.featureSecure")}</h3>
            <p className="mt-1 text-xs text-muted-foreground">{t("home.featureSecureDesc")}</p>
          </div>
          <div className="rounded-lg border p-4">
            <ShoppingBag className="mb-2 size-5 text-primary" />
            <h3 className="text-sm font-semibold">{t("home.featureSelf")}</h3>
            <p className="mt-1 text-xs text-muted-foreground">{t("home.featureSelfDesc")}</p>
          </div>
        </div>
      </main>

      <footer className="py-6 text-center text-sm text-muted-foreground">
        © {new Date().getFullYear()} {siteName} · {t("home.poweredBy", { name: "LiltIDC_OEM" })}
      </footer>
    </div>
  )
}
