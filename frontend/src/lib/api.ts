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
// 会员 token 与管理员 token 分开存储（批 3-α；后端 JWT issuer 双签发方隔离）
const CLIENT_TOKEN_KEY = "liltidc_client_token"

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

export function getClientToken(): string | null {
  try {
    return localStorage.getItem(CLIENT_TOKEN_KEY)
  } catch {
    return null
  }
}

export function setClientToken(token: string) {
  localStorage.setItem(CLIENT_TOKEN_KEY, token)
}

export function clearClientToken() {
  localStorage.removeItem(CLIENT_TOKEN_KEY)
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
  /** 鉴权域：admin=管理员 token（默认）；client=会员 token（/api/client/* 鉴权接口） */
  scope?: "admin" | "client"
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
  const token = options.scope === "client" ? getClientToken() : getToken()
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
    if (options.scope === "client") {
      // 会员域：清会员 token 并跳会员登录页（与管理员登录页隔离，ADR-024）
      clearClientToken()
      redirectTo("/client-login")
    } else {
      clearToken()
      redirectTo("/login")
    }
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

// ---- 批 2：上游对接与商品域（契约文档第 9/10 章） ----

export interface UpstreamConfigData {
  hostname: string
  username: string
  api_key_set: boolean
  downstream_token: string
  sync_path: string
  /** 本站对外地址（批 3-β：开通后 setdownstream 登记的 downstream_url；空=回退后端监听地址） */
  site_url: string
}

export interface UpstreamCurrency {
  code: string
  prefix: string
  suffix: string
}

export interface UpstreamTestResult {
  connected: boolean
  credit: string
  currency: UpstreamCurrency | null
  msg: string
}

/** 周期 → 价格文本（如 {monthly: "10.00"}） */
export type PriceMap = Record<string, string>

export interface UpstreamProduct {
  id: number
  type: string
  name: string
  description: string
  /** 上游成本价 */
  prices: PriceMap
  imported: boolean
  local_product_id: number
  local_name: string
  local_sale_prices: PriceMap | null
  local_status: number
}

export interface UpstreamGroup {
  id: number
  name: string
  products: UpstreamProduct[]
}

export interface StockItem {
  upstream_pid: number
  name: string
  stock_control: boolean
  qty: number
}

export interface ImportItem {
  upstream_pid: number
  name: string
  group_id: number
  group_name: string
  sale_prices: PriceMap
  status?: number
}

export interface ProductData {
  id: number
  group_id: number
  group_name: string
  upstream_pid: number
  name: string
  type: string
  cost_prices: PriceMap
  sale_prices: PriceMap
  stock_qty: number
  stock_control: boolean
  status: number
  sort: number
  updated_at: string
}

export interface ProductGroupData {
  id: number
  name: string
  sort: number
  status: number
}

export interface UpstreamHostList {
  page: number
  limit: number
  sum: number
  list: Record<string, unknown>[]
  domainstatus: Record<string, { name: string; color: string }>
}

// ---- 批 3-α：会员域（契约文档第 12 章） ----

export interface CaptchaData {
  captcha_id: string
  image: string // data:image/png;base64,...
  expires_in: number
}

export interface ClientInfo {
  id: number
  username: string
  email: string
  phone: string
  real_name: string
}

export interface ClientLoginResult {
  token: string
  expires_at: string
  client: ClientInfo
}

export interface ClientProfile {
  id: number
  username: string
  email: string
  phone: string
  real_name: string
  company: string
  address: string
  created_at: string
}

export interface ClientView {
  id: number
  username: string
  email: string
  phone: string
  real_name: string
  company: string
  /** 余额（十进制字符串文本，批 3-β） */
  credit: string
  status: number
  /** 后端按 Accept-Language 输出的状态文案，前端直接展示 */
  status_text: string
  upstream_uid: number | null
  upstream_linked_at: string
  last_login_at: string
  created_at: string
}

// ---- 批 3-β：商城 / 订单 / 余额（契约文档第 13/14 章） ----

/** 商城商品卡片（公开接口，免鉴权） */
export interface StoreProduct {
  id: number
  name: string
  /** 上游 HTML 描述原文（列表页提取纯文本摘要；详情页 sanitize 后渲染） */
  description: string
  type: string
  group_id: number
  group_name: string
  sale_prices: PriceMap
  /** 起价（各周期最低售价文本） */
  price_from: string
  stock_qty: number
  stock_control: boolean
  in_stock: boolean
}

export interface StoreGroup {
  id: number
  name: string
  products: StoreProduct[]
}

/** 商城商品详情（description 为上游 HTML 原文，渲染前必须 sanitize） */
export interface StoreProductDetail extends StoreProduct {
  /** 有售价的周期键（后端按规范序排好） */
  cycles: string[]
}

/** 订单条目开通信息快照（provision_info JSON，字段全部可选） */
export interface ProvisionInfo {
  hostname?: string
  password?: string
  currency_id?: number
  host_ids?: number[]
  ip?: string
  assigned_ips?: string
  username?: string
  host_password?: string
  next_due_date?: string
  domain_status?: string
  upstream_order_num?: string
  settled_at?: string
  activated_at?: string
  attempts?: number
  error?: string
}

export interface OrderItemView {
  id: number
  product_id: number
  product_name: string
  upstream_pid: number
  cycle: string
  cycle_text: string
  qty: number
  unit_price: string
  amount: string
  upstream_order_id: number
  upstream_host_id: number
  provision_status: "pending" | "provisioning" | "active" | "failed"
  provision_status_text: string
  provision_info: ProvisionInfo | null
}

export interface OrderView {
  id: number
  order_no: string
  client_id: number
  client_username?: string
  status: "Pending" | "Paid" | "Active" | "ProvisionFailed" | "Cancelled" | "Refunded"
  status_text: string
  amount: string
  balance_used: string
  pay_method: string
  pay_time: string
  cancel_time: string
  remark: string
  created_at: string
  items: OrderItemView[]
}

export interface OrderAdminDetail extends OrderView {
  transactions: BalanceTransactionView[]
}

export interface BalanceTransactionView {
  id: number
  type: "recharge_admin" | "consume" | "refund" | "adjust"
  type_text: string
  amount: string
  balance_after: string
  related_type: string
  related_id: number
  remark: string
  admin_id: number | null
  created_at: string
}

export interface ClientBalanceData {
  credit: string
  list: BalanceTransactionView[]
  total: number
  page: number
  page_size: number
}

/** 会员端主机视图（我的主机；不含密码） */
export interface ClientHostView {
  id: number
  upstream_host_id: number
  product_id: number | null
  product_name: string
  domain: string
  domain_status: string
  dedicated_ip: string
  assigned_ips: string
  username: string
  next_due_date: string
  synced_at: string
}

export const api = {
  /** 商城前台公开接口（免鉴权，批 3-β 契约 13.1） */
  store: {
    products: () => request<{ groups: StoreGroup[] }>("/store/products"),
    productDetail: (id: number) => request<StoreProductDetail>(`/store/products/${id}`),
  },
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
  /** 会员前台域（/api/client/*；鉴权接口用 scope:"client" 的会员 token） */
  client: {
    captcha: () => request<CaptchaData>("/client/captcha"),
    sendCode: (body: {
      purpose: string
      channel: "sms" | "email"
      to: string
      username?: string
      email?: string
      captcha_id?: string
      captcha_answer?: string
    }) => request<{ sent: boolean; expires_in: number }>("/client/auth/send-code", { method: "POST", body }),
    register: (body: {
      username: string
      password: string
      confirm_password: string
      email?: string
      phone?: string
      verify_channel: "sms" | "email"
      verify_code: string
    }) => request<{ id: number; username: string }>("/client/auth/register", { method: "POST", body }),
    login: (username: string, password: string) =>
      request<ClientLoginResult>("/client/auth/login", { method: "POST", body: { username, password } }),
    profile: () => request<ClientProfile>("/client/profile", { scope: "client" }),
    updateProfile: (body: {
      real_name?: string
      company?: string
      address?: string
      email?: string
      phone?: string
    }) => request<ClientProfile>("/client/profile", { method: "PUT", body, scope: "client" }),
    changePassword: (body: {
      old_password: string
      new_password: string
      confirm_password: string
    }) => request<{ changed: boolean }>("/client/change-password", {
      method: "POST",
      body,
      scope: "client",
    }),
    /** 余额与流水（批 3-β，契约 14.3） */
    balance: (params?: { page?: number; page_size?: number }) =>
      request<ClientBalanceData>("/client/balance", { query: params ?? {}, scope: "client" }),
    /** 下单（单商品立即购买，契约 13.2） */
    createOrder: (body: { product_id: number; cycle: string; qty: number }) =>
      request<OrderView>("/client/orders", { method: "POST", body, scope: "client" }),
    orders: (params?: { status?: string; page?: number; page_size?: number }) =>
      request<{ list: OrderView[]; total: number }>("/client/orders", {
        query: params ?? {},
        scope: "client",
      }),
    orderDetail: (id: number) =>
      request<OrderView>(`/client/orders/${id}`, { scope: "client" }),
    payOrder: (id: number, method: "balance" = "balance") =>
      request<OrderView>(`/client/orders/${id}/pay`, {
        method: "POST",
        body: { method },
        scope: "client",
      }),
    cancelOrder: (id: number) =>
      request<OrderView>(`/client/orders/${id}/cancel`, { method: "POST", scope: "client" }),
    /** 我的主机（契约 13.7） */
    hosts: (params?: { page?: number; page_size?: number }) =>
      request<{ list: ClientHostView[]; total: number }>("/client/hosts", {
        query: params ?? {},
        scope: "client",
      }),
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
    upstream: {
      getConfig: () => request<UpstreamConfigData>("/admin/upstream/config"),
      saveConfig: (body: {
        hostname: string
        username: string
        api_key: string
        /** 本站对外地址（批 3-β；下游登记 downstream_url，可留空回退监听地址） */
        site_url?: string
      }) => request<{ saved: boolean }>("/admin/upstream/config", { method: "PUT", body }),
      test: () => request<UpstreamTestResult>("/admin/upstream/test", { method: "POST" }),
      credit: () =>
        request<{ credit: string; currency: UpstreamCurrency }>("/admin/upstream/credit"),
      resetToken: () =>
        request<{ downstream_token: string }>("/admin/upstream/downstream-token/reset", {
          method: "POST",
        }),
      products: () => request<{ groups: UpstreamGroup[] }>("/admin/upstream/products"),
      stock: (pids: number[]) =>
        request<{ list: StockItem[] }>("/admin/upstream/products/stock", {
          query: { pids: pids.join(",") },
        }),
      importProducts: (items: ImportItem[]) =>
        request<{ created: number; updated: number }>("/admin/upstream/products/import", {
          method: "POST",
          body: { items },
        }),
      hosts: (page = 1, limit = 20) =>
        request<UpstreamHostList>("/admin/upstream/hosts", { query: { page, limit } }),
    },
    products: {
      list: (params?: { group_id?: number; page?: number; page_size?: number }) =>
        request<{ list: ProductData[]; total: number }>("/admin/products", { query: params ?? {} }),
      groups: () => request<{ list: ProductGroupData[] }>("/admin/product-groups"),
      update: (
        id: number,
        body: { name?: string; group_id?: number; sale_prices?: PriceMap; sort?: number },
      ) => request<ProductData>(`/admin/products/${id}`, { method: "PUT", body }),
      updateStatus: (id: number, status: number) =>
        request<ProductData>(`/admin/products/${id}/status`, { method: "PUT", body: { status } }),
      syncStock: () =>
        request<{ synced: number; total: number }>("/admin/products/sync-stock", {
          method: "POST",
        }),
    },
    /** 后台会员管理（批 3-α，契约 12.6） */
    clients: {
      list: (params?: { keyword?: string; status?: string; page?: number; page_size?: number }) =>
        request<{ list: ClientView[]; total: number }>("/admin/clients", { query: params ?? {} }),
      setStatus: (id: number, status: number) =>
        request<ClientView>(`/admin/clients/${id}/status`, { method: "PUT", body: { status } }),
      resetPassword: (id: number, newPassword: string) =>
        request<{ reset: boolean }>(`/admin/clients/${id}/reset-password`, {
          method: "POST",
          body: { new_password: newPassword },
        }),
      remove: (id: number) => request<{ deleted: boolean }>(`/admin/clients/${id}`, { method: "DELETE" }),
      /** 人工充值/扣款（批 3-β，契约 14.2；amount 带符号文本如 "+50.00"/"-20.00"） */
      adjustBalance: (id: number, amount: string, remark: string) =>
        request<{ client_id: number; credit: string; transaction: BalanceTransactionView }>(
          `/admin/clients/${id}/balance`,
          { method: "POST", body: { amount, remark } },
        ),
    },
    /** 后台订单管理（批 3-β，契约 13.5） */
    orders: {
      list: (params?: { status?: string; keyword?: string; page?: number; page_size?: number }) =>
        request<{ list: OrderView[]; total: number }>("/admin/orders", { query: params ?? {} }),
      detail: (id: number) => request<OrderAdminDetail>(`/admin/orders/${id}`),
      retryProvision: (id: number) =>
        request<OrderView>(`/admin/orders/${id}/retry-provision`, { method: "POST" }),
      cancelRefund: (id: number, remark?: string) =>
        request<OrderView>(`/admin/orders/${id}/cancel-refund`, {
          method: "POST",
          body: { remark: remark ?? "" },
        }),
    },
  },
}
