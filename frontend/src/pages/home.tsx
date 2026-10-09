// 前台首页占位：显示品牌站名与「商城即将上线」。
import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { LanguageSwitcher } from "@/components/language-switcher"
import { SiteLogo } from "@/components/site-logo"
import { useBrand } from "@/lib/brand"

export default function Home() {
  const { t } = useTranslation()
  const brand = useBrand()
  const siteName = brand?.site_name || "LiltIDC"

  return (
    <div className="flex min-h-screen flex-col">
      <header className="flex items-center justify-between px-6 py-4">
        <SiteLogo />
        <div className="flex items-center gap-1">
          <LanguageSwitcher />
          <Button asChild variant="outline" size="sm" className="ml-2">
            <Link to="/login">{t("home.adminLogin")}</Link>
          </Button>
        </div>
      </header>

      <main className="flex flex-1 flex-col items-center justify-center gap-6 px-6 text-center">
        <Badge variant="secondary" className="px-3 py-1 text-sm font-normal">
          {t("home.comingSoon")}
        </Badge>
        <h1 className="text-4xl font-bold tracking-tight md:text-6xl">{siteName}</h1>
        <p className="max-w-xl text-muted-foreground">{t("home.tagline")}</p>
      </main>

      <footer className="py-6 text-center text-sm text-muted-foreground">
        © {new Date().getFullYear()} {siteName} · {t("home.poweredBy", { name: "LiltIDC_OEM" })}
      </footer>
    </div>
  )
}
