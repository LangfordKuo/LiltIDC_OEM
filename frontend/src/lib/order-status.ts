// 订单状态视觉映射（会员端与后台共用；独立成 lib 以满足
// react-refresh/only-export-components——页面文件只导出组件）。

/** 订单状态 → Badge 视觉映射。 */
export function orderStatusVariant(
  status: string,
): "default" | "secondary" | "destructive" | "outline" {
  switch (status) {
    case "Active":
      return "default"
    case "Paid":
      return "secondary"
    case "ProvisionFailed":
      return "destructive"
    default:
      return "outline" // Pending/Cancelled/Refunded
  }
}
