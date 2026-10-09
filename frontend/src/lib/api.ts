// 统一 API 客户端。
//
// 职责（契约见 docs/api-contract.md 第 0 章）：
// - baseURL：/api（开发期由 Vite 代理到 127.0.0.1:8080，生产由 Nginx 反代）；
// - JWT：localStorage 存取，请求自动携带 Authorization: Bearer；
// - 统一拆包 {status,msg,data} / {status,code,msg}；
// - 全局副作用：401 unauthorized → 清 token 跳 /login；409 not_installed → 刷新缓存跳 /install。
import { setInstallState } from "@/lib/install-state"

/** 结构化 API 错误：httpStatus=0 表示网络层失败（后端不可达）。 */
export class ApiError extends Error {
  readonly httpStatus: number
  readonly code: string

  constructor(httpStatus: number, code: string, message: string) {
    super(message)
    this.name = "ApiError"
    this.httpStatus = httpStatus
    this.code = code
  }
}

const TOKEN_KEY = "liltidc_token"

export function getToken(): string | null {
  try {
    return localStorage.getItem(TOKEN_KEY)
  } catch {
    return null
  }
}

export function setToken(token: string) {
  localStorage.setItem(TOKEN_KEY, token)
}

export function clearToken() {
  localStorage.removeItem(TOKEN_KEY)
}

/** 跳转（整页刷新，绕开 React Router 以保证状态干净），已在目标路径时不重复跳。 */
function redirectTo(path: string) {
  if (window.location.pathname.startsWith(path)) return
  window.location.assign(path)
}

interface RequestOptions {
  method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE"
  body?: unknown
  /** 附加查询参数（值为空串/undefined 时忽略） */
  query?: Record<string, string | number | undefined>
  signal?: AbortSignal
}

/** 发送请求并拆包，非 2xx 或 status!=200 抛 ApiError。 */
async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  let url = `/api${path}`
  if (options.query) {
    const qs = new URLSearchParams()
    for (const [k, v] of Object.entries(options.query)) {
      if (v !== undefined && v !== "") qs.set(k, String(v))
    }
    const s = qs.toString()
    if (s) url += `?${s}`
  }

  const headers: Record<string, string> = {}
  if (options.body !== undefined) headers["Content-Type"] = "application/json"
  const token = getToken()
  if (token) headers["Authorization"] = `Bearer ${token}`

  let resp: Response
  try {
    resp = await fetch(url, {
      method: options.method ?? "GET",
      headers,
      body: options.body !== undefined ? JSON.stringify(options.body) : undefined,
      signal: options.signal,
    })
  } catch {
    throw new ApiError(0, "network_error", "无法连接服务器，请检查后端服务是否已启动")
  }

  // 拆包（响应体可能不是 JSON，如网关 502 页面 → json=null）
  interface Envelope {
    status?: number
    msg?: string
    code?: string
    data?: T
  }
  const json = (await resp.json().catch(() => null)) as Envelope | null

  if (resp.ok && json?.status === 200) {
    return json.data as T
  }

  const code = json?.code ?? "unknown"
  const msg = json?.msg ?? `请求失败（HTTP ${resp.status}）`

  // ---- 全局副作用 ----
  if (code === "not_installed") {
    setInstallState(false)
    redirectTo("/install")
  } else if (code === "unauthorized") {
    clearToken()
    redirectTo("/login")
  }

  throw new ApiError(resp.status, code, msg)
}

// ================= 接口定义（与 docs/api-contract.md 对齐） =================

export interface DbTestRequest {
  host: string
  port: number
  user: string
  password: string
  dbname?: string
}

export interface InstallCompleteRequest {
  database: {
    host: string
    port: number
    user: string
    password: string
    dbname: string
  }
  site_name: string
  logo?: string
  primary_color: string
  admin: {
    username: string
    password: string
    email?: string
    real_name?: string
  }
}

export interface InstallCompleteResult {
  installed: boolean
  restart_required: boolean
  admin_id: number
}

export interface AdminInfo {
  id: number
  username: string
  real_name: string
  email: string
  last_login_at: string | null
}

export interface LoginResult {
  token: string
  expires_at: string
  admin: AdminInfo
}

export interface NotifySettingsData {
  smsbao: { username: string; password_set: boolean; sign: string }
  smtp: {
    host: string
    port: number
    username: string
    password_set: boolean
    from: string
    tls: boolean
  }
}

export interface SaveNotifySettingsRequest {
  smsbao?: { username: string; password: string; sign: string }
  smtp?: {
    host: string
    port: number
    username: string
    password: string
    from: string
    tls: boolean
  }
}

export interface NotifyTemplate {
  id: number
  scene: string
  channel: string
  lang: string
  subject: string
  content: string
  enabled: boolean
  updated_at: string
}

export const api = {
  install: {
    status: () => request<{ installed: boolean }>("/install/status"),
    dbtest: (body: DbTestRequest) =>
      request<{ version: string }>("/install/dbtest", { method: "POST", body }),
    complete: (body: InstallCompleteRequest) =>
      request<InstallCompleteResult>("/install/complete", { method: "POST", body }),
  },
  public: {
    brand: () =>
      request<{ site_name: string; logo: string; primary_color: string }>("/public/brand"),
  },
  auth: {
    login: (username: string, password: string) =>
      request<LoginResult>("/auth/login", { method: "POST", body: { username, password } }),
    me: () => request<AdminInfo>("/auth/me"),
  },
  admin: {
    notify: {
      getSettings: () => request<NotifySettingsData>("/admin/notify/settings"),
      saveSettings: (body: SaveNotifySettingsRequest) =>
        request<{ saved: boolean }>("/admin/notify/settings", { method: "PUT", body }),
      testSms: (to: string, content?: string) =>
        request<{ sent: boolean }>("/admin/notify/test-sms", {
          method: "POST",
          body: { to, content: content ?? "" },
        }),
      testEmail: (to: string, subject?: string, content?: string) =>
        request<{ sent: boolean }>("/admin/notify/test-email", {
          method: "POST",
          body: { to, subject: subject ?? "", content: content ?? "" },
        }),
      smsBalance: () => request<{ sent: number; remain: number }>("/admin/notify/sms-balance"),
      listTemplates: (filters?: { scene?: string; channel?: string; lang?: string }) =>
        request<{ list: NotifyTemplate[] }>("/admin/notify/templates", { query: filters ?? {} }),
      updateTemplate: (
        id: number,
        body: { subject?: string; content?: string; enabled?: boolean },
      ) => request<NotifyTemplate>(`/admin/notify/templates/${id}`, { method: "PUT", body }),
    },
  },
}
