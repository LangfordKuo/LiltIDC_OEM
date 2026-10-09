// 语言切换器：中英双语，切换后由 i18next-browser-languagedetector 持久化到 localStorage。
import { useTranslation } from "react-i18next"
import { Globe } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

const LANGUAGES = [
  { code: "zh-CN", labelKey: "language.zhCN" },
  { code: "en-US", labelKey: "language.enUS" },
] as const

export function LanguageSwitcher() {
  const { t, i18n } = useTranslation()
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" aria-label={t("language.switch")}>
          <Globe />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {LANGUAGES.map((lang) => (
          <DropdownMenuItem
            key={lang.code}
            onClick={() => void i18n.changeLanguage(lang.code)}
            className={i18n.language === lang.code ? "font-semibold" : undefined}
          >
            {t(lang.labelKey)}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
