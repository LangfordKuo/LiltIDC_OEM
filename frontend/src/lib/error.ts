// 错误信息提取助手：把任意异常转换为可直接展示给用户的文本。
import { ApiError } from "@/lib/api"

export function errorMessage(err: unknown, fallback: string): string {
  if (err instanceof ApiError) {
    // httpStatus=0 为网络层失败，message 已是可读中文
    return err.message || fallback
  }
  if (err instanceof Error && err.message) {
    return err.message
  }
  return fallback
}
