// 站点 Logo + 站名（品牌信息来自 /api/public/brand，未加载时显示默认图标与兜底站名）。
import { Server } from "lucide-react"

import { useBrand } from "@/lib/brand"
import { cn } from "@/lib/utils"

interface SiteLogoProps {
  className?: string
  /** 是否显示站名文字 */
  showName?: boolean
}

export function SiteLogo({ className, showName = true }: SiteLogoProps) {
  const brand = useBrand()
  return (
    <span className={cn("flex items-center gap-2", className)}>
      {brand?.logo ? (
        <img src={brand.logo} alt="logo" className="size-8 rounded-md object-contain" />
      ) : (
        <span className="flex size-8 items-center justify-center rounded-md bg-primary text-primary-foreground">
          <Server className="size-5" />
        </span>
      )}
      {showName && (
        <span className="text-lg font-semibold tracking-tight">
          {brand?.site_name || "LiltIDC"}
        </span>
      )}
    </span>
  )
}
