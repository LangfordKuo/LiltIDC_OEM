# LiltIDC_OEM API 契约文档

> 本文档是前后端唯一契约来源。**先写契约，再写代码**。
> 后续批次（商品/订单/会员/工单/财务/支付/上游对接）按功能域在本文档追加章节，不改动已有章节编号语义。

- 文档版本：批 1（项目骨架与地基）
- 更新日期：2026-10-10

---

## 0. 通用约定

### 0.1 基础地址

| 环境 | API 基础地址 | 说明 |
|---|---|---|
| 开发 | `http://127.0.0.1:5173/api`（Vite 代理） | Vite dev server 将 `/api` 代理到 `http://127.0.0.1:8080` |
| 生产 | `https://<域名>/api` | Nginx 托管前端静态产物并反代 `/api` 到后端 |

### 0.2 统一响应体

**成功响应**（HTTP 200）：

```json
{
  "status": 200,
  "msg": "ok",
  "data": { }
}
```

**错误响应**（HTTP 4xx/5xx，HTTP 状态码与 body 中 `status` 一致）：

```json
{
  "status": 409,
  "code": "not_installed",
  "msg": "系统尚未安装"
}
```

约定：
- 成功响应**不含** `code` 字段，`msg` 恒为 `"ok"`，业务数据一律放 `data`。
- 错误响应**必含** `code`（字符串错误码，见 0.4），`msg` 为人类可读信息（可直接展示给管理员；开发期可能是英文底层错误原文）。
- 时间字段一律为 RFC3339 字符串（如 `2026-10-10T12:00:00+08:00`），可为 `null`。
- 布尔字段用 JSON `true`/`false`。

### 0.3 认证方式

- 管理员接口使用 JWT（HS256），请求头：`Authorization: Bearer <token>`。
- token 由 `POST /api/auth/login` 签发，有效期由配置 `[jwt] expire_hours` 决定（默认 24 小时）。
- Claims：`admin_id`（uint）、`username`（string）、`iss`（固定 `liltidc-oem`）、`iat`、`exp`。
- 前端将 token 存于 `localStorage`（键 `liltidc_token`）。
- 认证失败统一返回 401 `unauthorized`；前端收到 401 应清除 token 并跳转 `/login`。

### 0.4 错误码总表（批 1/2）

| code | HTTP 状态 | 含义 |
|---|---|---|
| `not_installed` | 409 | 系统未安装，除安装/公共接口外全部返回此错误；前端据此跳转 `/install` |
| `already_installed` | 409 | 系统已安装，安装接口拒绝重复执行（防重装攻击） |
| `invalid_param` | 400 | 请求参数缺失或非法，`msg` 指明具体字段 |
| `db_connect_failed` | 400（dbtest）/ 500（complete） | 数据库连接失败，`msg` 含底层驱动可读原因（如 Access denied / connection refused） |
| `db_version_unsupported` | 400 | MySQL 版本低于 5.7 |
| `migration_failed` | 500 | 建库建表（迁移）失败 |
| `config_write_failed` | 500 | 写配置文件失败（权限/磁盘），`msg` 含路径与原因 |
| `unauthorized` | 401 | 未携带 token / token 无效或过期 |
| `invalid_credentials` | 401 | 登录用户名或密码错误（不区分两者，防用户枚举） |
| `account_disabled` | 403 | 管理员账号已被禁用 |
| `not_found` | 404 | 资源或路由不存在 |
| `template_not_found` | 404 | 通知模板不存在（scene+channel+lang 无匹配） |
| `template_disabled` | 400 | 通知模板已停用 |
| `notify_not_configured` | 400 | 短信/邮件通道未配置（账号为空） |
| `sms_send_failed` | 502 | 短信宝发送失败，`msg` 为映射后的可读错误（见 8.4） |
| `sms_balance_query_failed` | 502 | 短信宝余额查询失败 |
| `email_send_failed` | 502 | SMTP 邮件发送失败，`msg` 含底层原因 |
| `internal_error` | 500 | 服务器内部错误（含 panic 恢复） |
| `upstream_not_configured` | 400 | 上游地址/账号/密钥未配置（批 2） |
| `upstream_auth_failed` | 502 | 上游登录失败——账号或 API 密钥错误（批 2） |
| `upstream_request_failed` | 502 | 上游请求失败——网络/超时/WAF 拦截/业务错误，`msg` 含底层原因（批 2） |
| `signature_invalid` | 403 | host/sync 推送签名验证失败（批 2，见 9.9） |
| `product_not_found` | 404 | 本地商品不存在（批 2） |
| `username_exists` | 400 | 会员用户名已占用（批 3-α，仅注册/验证码预占场景） |
| `email_exists` | 400 | 会员邮箱已占用（批 3-α） |
| `phone_exists` | 400 | 会员手机号已占用（批 3-α） |
| `verify_code_invalid` | 400 | 验证码错误或已过期（批 3-α） |
| `rate_limited` | 429 | 触发频控（验证码发送限额），`msg` 含等待秒数或配额说明（批 3-α） |
| `captcha_required` | 400 | 需要图形验证码（同 IP 当日发送 ≥5 条后强制）（批 3-α） |
| `captcha_invalid` | 400 | 图形验证码错误或已过期（校验即焚）（批 3-α） |
| `client_not_found` | 404 | 会员不存在（批 3-α） |

### 0.5 未安装门禁

- 后端配置 `[install] installed = false`（且进程运行时状态为未安装）时：
  - 白名单外的所有 `/api/*` 请求返回 **HTTP 409** `{"status":409,"code":"not_installed","msg":"系统尚未安装"}`。
  - 白名单：`GET /api/install/status`、`POST /api/install/dbtest`、`POST /api/install/complete`、`GET /api/public/brand`、`GET /api/healthz`，以及所有非 `/api/` 前缀路径（静态资源）。
- 安装完成后进程内原子切换为已安装态，**无需重启进程**（`restart_required = false`）。

---

## 1. 安装向导域 `/api/install`

### 1.1 查询安装状态

- `GET /api/install/status`
- 鉴权：无。**未安装/已安装状态均可访问**（不触发门禁）。
- 响应：

```json
{ "status": 200, "msg": "ok", "data": { "installed": false } }
```

### 1.2 数据库连接测试

- `POST /api/install/dbtest`
- 鉴权：无（仅未安装态可执行；已安装态返回 409 `already_installed`）。
- 行为：用提交的凭据连接 MySQL 服务器（**不连接、不创建具体数据库**），读取版本号并校验 >= 5.7。
- 请求体：

```json
{
  "host": "127.0.0.1",
  "port": 3306,
  "user": "root",
  "password": "secret",
  "dbname": "liltidc_oem"
}
```

| 字段 | 类型 | 必填 | 校验 |
|---|---|---|---|
| host | string | 是 | 非空 |
| port | int | 是 | 1–65535 |
| user | string | 是 | 非空 |
| password | string | 否 | 可为空 |
| dbname | string | 否 | 若填写则须匹配 `^[A-Za-z0-9_-]{1,64}$` |

- 成功响应：

```json
{ "status": 200, "msg": "ok", "data": { "version": "5.7.44" } }
```

- 失败响应示例（密码错误，HTTP 400）：

```json
{
  "status": 400,
  "code": "db_connect_failed",
  "msg": "数据库连接失败: Error 1045 (28000): Access denied for user 'root'@'localhost' (using password: YES)"
}
```

- 错误码：`invalid_param` / `db_connect_failed`（msg 携带底层可读原因）/ `db_version_unsupported`（msg 携带实际版本号）/ `already_installed`。

### 1.3 完成安装

- `POST /api/install/complete`
- 鉴权：无（一次性接口；**已安装态返回 409 `already_installed`**，防重装攻击）。
- 行为（一次性事务式流程）：
  1. 校验参数；
  2. 连接 MySQL，校验版本 >= 5.7；
  3. `CREATE DATABASE IF NOT EXISTS`（utf8mb4 / utf8mb4_general_ci）；
  4. GORM AutoMigrate 建表（见第 6 章）；
  5. 创建管理员（bcrypt 哈希）；
  6. 种子写入 `settings`（站点品牌 + 从 config.toml 继承的短信/邮件配置）与 `notify_templates` 默认模板；
  7. 回写 `config.toml`（数据库/品牌/JWT secret 自动生成/`installed = true`）；
  8. 进程内原子切换为已安装态（DB 句柄、通知管理器热装载，**不要求重启**）。
- 请求体：

```json
{
  "database": {
    "host": "127.0.0.1",
    "port": 3306,
    "user": "root",
    "password": "secret",
    "dbname": "liltidc_oem"
  },
  "site_name": "LiltIDC",
  "logo": "",
  "primary_color": "#2563eb",
  "admin": {
    "username": "admin",
    "password": "Admin@12345",
    "email": "admin@example.com",
    "real_name": "管理员"
  }
}
```

| 字段 | 类型 | 必填 | 校验 |
|---|---|---|---|
| database.* | — | 是 | 同 1.2，且 dbname 必填 |
| site_name | string | 是 | 1–64 字符 |
| logo | string | 否 | URL 或空 |
| primary_color | string | 是 | `^#[0-9a-fA-F]{6}$` |
| admin.username | string | 是 | `^[A-Za-z0-9_]{3,32}$` |
| admin.password | string | 是 | 8–72 字符（bcrypt 上限 72 字节） |
| admin.email | string | 否 | 合法邮箱格式 |
| admin.real_name | string | 否 | ≤ 64 字符 |

- 成功响应：

```json
{
  "status": 200,
  "msg": "ok",
  "data": { "installed": true, "restart_required": false, "admin_id": 1 }
}
```

- 错误码：`already_installed` / `invalid_param`（含"管理员用户名已存在"）/ `db_connect_failed` / `db_version_unsupported` / `migration_failed` / `config_write_failed`。
- 失败恢复说明：若第 7 步写配置文件失败（数据库已建好），接口返回 `config_write_failed`；此时可修复文件权限后重试（流程幂等：建库 `IF NOT EXISTS`、AutoMigrate 幂等、管理员重名会明确报错），或手工复制 `config.example.toml` 为 `config.toml` 填入数据库配置并置 `installed = true` 后重启。

---

## 2. 公共域 `/api/public`

### 2.1 获取品牌信息

- `GET /api/public/brand`
- 鉴权：无。**未安装态也可访问**（安装页需要样式），此时读取 `config.toml` 的 `[brand]`；已安装态优先读 `settings` 表（键 `site_name`/`logo`/`primary_color`），缺失回退配置。
- 响应：

```json
{
  "status": 200,
  "msg": "ok",
  "data": { "site_name": "LiltIDC", "logo": "", "primary_color": "#2563eb" }
}
```

前端行为约定：拿到 `primary_color`（十六进制）后运行时覆盖 CSS 变量 `--primary`，并按亮度自动计算 `--primary-foreground`（黑/白）与 `--ring`，见 docs/architecture.md。

---

## 3. 健康检查 `/api/healthz`

- `GET /api/healthz`
- 鉴权：无，未安装态可访问（供 Nginx/运维探活）。
- 响应：

```json
{ "status": 200, "msg": "ok", "data": { "ok": true, "installed": false, "version": "0.1.0" } }
```

---

## 4. 认证域 `/api/auth`

> 本章接口受未安装门禁保护：未安装时返回 409 `not_installed`。

### 4.1 管理员登录

- `POST /api/auth/login`
- 鉴权：无（受门禁保护）。
- 请求体：`{ "username": "admin", "password": "Admin@12345" }`（均必填）
- 成功响应：

```json
{
  "status": 200,
  "msg": "ok",
  "data": {
    "token": "<JWT>",
    "expires_at": "2026-10-11T12:00:00+08:00",
    "admin": { "id": 1, "username": "admin", "real_name": "管理员", "email": "admin@example.com" }
  }
}
```

- 副作用：更新 `admins.last_login_at`。
- 错误码：`invalid_credentials`（401，用户名不存在与密码错误返回同一错误）/ `account_disabled`（403）。

### 4.2 获取当前登录管理员

- `GET /api/auth/me`
- 鉴权：JWT。
- 响应：

```json
{
  "status": 200,
  "msg": "ok",
  "data": {
    "id": 1, "username": "admin", "real_name": "管理员",
    "email": "admin@example.com", "last_login_at": "2026-10-10T12:00:00+08:00"
  }
}
```

- 错误码：`unauthorized`。

---

## 5. 管理后台 · 通知域 `/api/admin/notify`

> 本章全部接口要求：已安装 + JWT 管理员认证。批 1 只做基础设施与配置，不做业务触发。

### 5.1 读取通知配置

- `GET /api/admin/notify/settings`
- 数据来源：`settings` 表（安装时已从 config.toml 种子化；运行时以数据库为准）。
- 响应（**密码字段不回传明文/哈希，只回传是否已设置**）：

```json
{
  "status": 200,
  "msg": "ok",
  "data": {
    "smsbao": { "username": "smsuser", "password_set": true, "sign": "岭云互联" },
    "smtp": {
      "host": "smtp.example.com", "port": 465, "username": "noreply@example.com",
      "password_set": true, "from": "noreply@example.com", "tls": true
    }
  }
}
```

### 5.2 保存通知配置

- `PUT /api/admin/notify/settings`
- 请求体（与 5.1 data 同构，但用 `password` 字段提交新密码；**留空 = 不修改**）：

```json
{
  "smsbao": { "username": "smsuser", "password": "", "sign": "岭云互联" },
  "smtp": { "host": "smtp.example.com", "port": 465, "username": "noreply@example.com",
            "password": "", "from": "noreply@example.com", "tls": true }
}
```

- 短信宝密码规则：提交值若为 32 位十六进制则视为 MD5 直接存储；否则后端自动做 MD5（`password_md5` 语义，见 8.2）。
- 端口校验：1–65535；`from`/`username` 若填写须为合法邮箱（宽松校验）。
- 响应：`{ "status": 200, "msg": "ok", "data": { "saved": true } }`
- 错误码：`invalid_param`。

### 5.3 发送测试短信

- `POST /api/admin/notify/test-sms`
- 请求体：`{ "to": "13800138000", "content": "可选，留空使用默认测试文案" }`
- 行为：读取当前数据库中的短信宝配置直连发送（不经模板）；内容自动前缀 `【签名】`。
- 成功：`{ "status": 200, "msg": "ok", "data": { "sent": true } }`
- 错误码：`notify_not_configured` / `invalid_param` / `sms_send_failed`（msg 为映射后的可读错误，如实显示）。

### 5.4 发送测试邮件

- `POST /api/admin/notify/test-email`
- 请求体：`{ "to": "user@example.com", "subject": "可选", "content": "可选，支持 HTML" }`
- 成功：`{ "status": 200, "msg": "ok", "data": { "sent": true } }`
- 错误码：`notify_not_configured` / `invalid_param` / `email_send_failed`。

### 5.5 查询短信余额

- `GET /api/admin/notify/sms-balance`
- 行为：调用短信宝 `query` 接口。
- 成功：`{ "status": 200, "msg": "ok", "data": { "sent": 100, "remain": 500 } }`（sent=已发送条数，remain=剩余条数）
- 错误码：`notify_not_configured` / `sms_balance_query_failed`。

### 5.6 模板列表

- `GET /api/admin/notify/templates`
- 查询参数（全部可选）：`scene`、`channel`、`lang`。
- 响应：

```json
{
  "status": 200,
  "msg": "ok",
  "data": {
    "list": [
      {
        "id": 1, "scene": "register_verify", "channel": "sms", "lang": "zh-CN",
        "subject": "", "content": "您的验证码是 {code}，10 分钟内有效，请勿泄露给他人。",
        "enabled": true, "updated_at": "2026-10-10T12:00:00+08:00"
      }
    ]
  }
}
```

### 5.7 更新模板

- `PUT /api/admin/notify/templates/:id`
- 请求体（`scene`/`channel`/`lang` 不可改）：`{ "subject": "…", "content": "…", "enabled": true }`，三个字段均可选，缺省不修改。
- 校验：`content` 若提交则不可为空；短信模板忽略 `subject`。
- 响应 data 为更新后的完整模板对象（同 5.6 列表项）。
- 错误码：`invalid_param` / `template_not_found` / `not_found`。

---

## 6. 数据库表结构（批 1 最小集）

> MySQL 5.7 / utf8mb4 / utf8mb4_general_ci / InnoDB。由 GORM AutoMigrate 创建（选型理由见 docs/architecture.md）。
> 注意 MySQL 5.7 特性约束：不使用 CHECK 约束做业务校验（5.7 解析但不生效）、不使用窗口函数、不依赖 JSON 函数——一切校验在 Go 层完成。

### 6.1 `admins` 管理员表

| 列 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint unsigned | PK, AUTO_INCREMENT | |
| username | varchar(64) | UNIQUE, NOT NULL | 登录名 |
| password_hash | varchar(100) | NOT NULL | bcrypt 哈希 |
| email | varchar(128) | | 邮箱 |
| real_name | varchar(64) | | 真实姓名/昵称 |
| status | tinyint | NOT NULL, 默认 1 | 1=启用 0=禁用 |
| last_login_at | datetime(3) NULL | | 最后登录时间 |
| created_at | datetime(3) | | |
| updated_at | datetime(3) | | |

### 6.2 `settings` 通用设置表（key/value）

| 列 | 类型 | 约束 | 说明 |
|---|---|---|---|
| `key` | varchar(128) | PK | 设置键（GORM/查询均以反引号包裹，避开 MySQL 保留字） |
| value | text | | 设置值（字符串化；布尔用 "true"/"false"，数字用十进制字符串） |
| updated_at | datetime(3) | | |

批 1 使用的键：`site_name`、`logo`、`primary_color`、`smsbao_username`、`smsbao_password_md5`、`smsbao_sign`、`smtp_host`、`smtp_port`、`smtp_username`、`smtp_password`、`smtp_from`、`smtp_tls`。

批 2 新增键：`upstream_hostname`、`upstream_username`、`upstream_api_key`（上游凭据运行时真源，config.toml `[upstream]` 仅为种子）、`downstream_token`（host/sync 验签密钥，首次读取时自动生成）。

### 6.3 `notify_templates` 通知模板表

| 列 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint unsigned | PK, AUTO_INCREMENT | |
| scene | varchar(64) | 联合唯一 idx(scene,channel,lang) | 场景枚举，见 8.1 |
| channel | varchar(16) | 同上 | `sms` / `email` |
| lang | varchar(16) | 同上 | `zh-CN` / `en-US` |
| subject | varchar(255) | | 邮件主题（短信忽略） |
| content | text | NOT NULL | 模板内容，含 `{placeholder}` 占位符 |
| enabled | tinyint(1) | NOT NULL, 默认 1 | 是否启用 |
| updated_at | datetime(3) | | |

### 6.4 `product_groups` 商品分组表（批 2）

| 列 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint unsigned | PK, AUTO_INCREMENT | |
| name | varchar(128) | UNIQUE, NOT NULL | 分组名（导入时按名 FirstOrCreate，防重复建组） |
| sort | int | NOT NULL 默认 0 | 排序 |
| status | tinyint | NOT NULL 默认 1 | 1=启用 0=停用 |
| created_at / updated_at | datetime(3) | | |

### 6.5 `products` 本地商品表（批 2）

| 列 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint unsigned | PK, AUTO_INCREMENT | |
| group_id | bigint unsigned | INDEX, NOT NULL | 所属分组 |
| upstream_pid | bigint | **UNIQUE**, NOT NULL | 上游商品 ID（映射唯一：重复导入=更新，幂等） |
| name | varchar(255) | NOT NULL | 本地名称（默认上游名可改） |
| description | text | | 上游描述（HTML，已实体解码） |
| type | varchar(64) | | 上游商品 type 原样 |
| cost_prices | text | | 上游成本价 JSON 文本：`{"monthly":"10.00",...}`（金额用字符串防浮点误差；MySQL 5.7 JSON 支持有限故用 TEXT） |
| sale_prices | text | | 自定义售价 JSON 文本（结构同上，每周期必填且 > 0） |
| stock_qty | int | NOT NULL 默认 0 | 库存（proinfo 同步） |
| stock_control | tinyint(1) | NOT NULL 默认 0 | 上游是否启用库存控制 |
| status | tinyint | NOT NULL 默认 1 | 1=上架 0=下架 |
| sort | int | NOT NULL 默认 0 | 排序 |
| created_at / updated_at | datetime(3) | | |

计费周期枚举（与上游 `product_pricings` 周期键**完全一致**，lyew.com 实测 17 键，本地规范键统一采用上游键名免映射）：`onetime`（一次性）、`hour`（时付）、`day`（日付）、`ontrial`（试用）、`monthly`（月付）、`quarterly`（季付）、`semiannually`（半年付，**无下划线**）、`annually`（年付）、`biennially`（两年付）、`triennially`（三年付）、`fourly`~`tenly`（四~十年付）。

**成本价唯一真源**是 `/api/product/prodetail` 的 `product_pricings[0]`（字段表见 9.5a）；`/cart/all` 的 `pay_type` 为字符串化 JSON，**不含任何价格**——真实形状 `{"pay_type":"recurring","pay_hour_cycle":"720","pay_day_cycle":"30","pay_ontrial_*":…}`，其中 `pay_type` 字段是付费模式标记（recurring/onetime），`pay_hour_cycle`/`pay_day_cycle` 是**周期时长定义**（小时数/天数），绝不能当价格解析（批 2 第二个 P0 教训，ADR-022）。价格 `-1.00`（或 `-1`）= 该周期停售，解析时过滤；`"0.00"`（免费）保留。

### 6.6 `hosts` 主机表（批 2 最小集）

| 列 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint unsigned | PK, AUTO_INCREMENT | |
| upstream_host_id | bigint | **UNIQUE**, NOT NULL | 上游主机 ID（推送 host_id；upsert 匹配键） |
| product_id | bigint unsigned NULL | | 本地商品关联（批 3 订单链路回填） |
| client_id | bigint unsigned NULL | | 本地会员关联（批 3 会员体系回填） |
| domain | varchar(255) | | 域名 |
| username | varchar(128) | | 主机用户名 |
| password | varchar(255) | | 主机密码（上游推送即明文，原样保存） |
| dedicated_ip | varchar(128) | | 独立 IP（推送字段 dedicatedip） |
| assigned_ips | varchar(512) | | 附加 IP（推送字段 assignedips） |
| port | varchar(16) | | 端口（文本保存，上游可能推数字或字符串） |
| os / os_url | varchar(128/512) | | 操作系统 |
| next_due_date | varchar(32) | | 到期日（原样保存上游文本） |
| domain_status | varchar(32) | INDEX | Pending/Active/Suspended/Cancelled/Fraud/Deleted |
| raw | text | | 推送原文 JSON 备份（不对外输出，溯源用） |
| synced_at | datetime(3) | | 最后同步时间 |
| created_at / updated_at | datetime(3) | | |

### 6.7 安装状态存储

安装状态**不建表**，以 `config.toml` 的 `[install] installed` 为持久化真源 + 进程内 `atomic.Bool` 运行时真源。理由（先有配置才有数据库，避免鸡生蛋问题）见 docs/architecture.md。

---

## 7. 配置项说明（backend/config.toml）

> `config.toml` 不入库（.gitignore），仓库提供 `config.example.toml`。路径可用环境变量 `LILTIDC_CONFIG` 覆盖（默认 `./config.toml`）。
> 优先级说明：`[smtp]`/`[smsbao]`/`[brand]` 仅作为**安装时的种子值**；安装完成后运行时以数据库 `settings` 表为准（后台界面修改不回写 config.toml）。

```toml
[server]
addr = "127.0.0.1"     # 监听地址；生产建议 127.0.0.1（由 Nginx 反代）
port = 8080            # 监听端口
mode = "debug"         # gin 模式：debug / release / test
static_dir = ""        # 可选：前端静态产物目录（如 ../frontend/dist），留空则不托管
cors_allow_origins = ["http://localhost:5173", "http://127.0.0.1:5173"]  # CORS 白名单（ADR-018）
# CORS 跨域白名单（字符串数组，含协议与端口）。默认 ["http://localhost:5173","http://127.0.0.1:5173"]（本机 Vite dev server）。
# 生产 Nginx 同域反代不依赖 CORS；前后端分域部署时填真实前端源。留空/缺省时回落默认值。
cors_allow_origins = ["http://localhost:5173", "http://127.0.0.1:5173"]

[database]             # 安装向导完成时由安装器回写
host = "127.0.0.1"
port = 3306
user = "root"
password = ""
dbname = "liltidc_oem"
charset = "utf8mb4"

[jwt]
secret = ""               # HS256 密钥；安装时若为空自动生成 64 位十六进制随机串
expire_hours = 24         # 管理员 token 有效期（小时）
client_expire_hours = 24  # 会员 token 有效期（小时，批 3-α；会员 JWT issuer=liltidc-oem-client 与管理员隔离）

[brand]                # 未安装态的品牌来源；安装后种子进 settings 表
site_name = "LiltIDC"
logo = ""
primary_color = "#2563eb"

[smtp]                 # 邮件通道种子值
host = ""
port = 25
username = ""
password = ""
from = ""
tls = false            # true = 要求 STARTTLS

[smsbao]               # 短信宝通道种子值
username = ""
password_md5 = ""      # 短信宝密码的 MD5（32 位小写十六进制）
sign = ""              # 短信签名，发送时自动前缀【签名】

[upstream]             # 上游魔方财务（批 2 起生效）
# 运行时真源是数据库 settings（后台「上游对接」页可改）；此处仅为安装/启动种子。
hostname = ""          # 上游站点地址，如 https://lyew.com
username = ""          # 上游账号（手机号不加 +86/86 区号，加了会鉴权失败）
api_key = ""           # 上游 API 密钥（登录密码）
timeout_seconds = 15   # 单次上游请求超时（秒），默认 15

[install]
installed = false      # 安装完成时由安装器置 true；手工改回 false 可重新进入安装态
```

---

## 8. notify 枚举与占位符约定

### 8.1 scene / channel / lang 枚举

| 维度 | 取值 | 说明 |
|---|---|---|
| scene | `register_verify` | 注册验证码（批 1 只建模板，不触发） |
| scene | `bill_notify` | 账单通知（批 1 只建模板，不触发） |
| channel | `sms` | 短信（短信宝） |
| channel | `email` | 邮件（SMTP） |
| lang | `zh-CN` / `en-US` | 模板语言；请求语言无精确匹配时回退 `zh-CN` |

### 8.2 调用约定（后续批次使用）

Go 内部统一入口（`internal/notify`）：

```go
// Send 按 场景+通道+语言 加载模板 → 渲染占位符 → 经对应 provider 发送
func (m *Manager) Send(ctx context.Context, scene, channel, lang, receiver string, vars map[string]string) error
```

- `receiver`：手机号（sms）或邮箱地址（email）。
- `vars`：占位符键值对；`{site_name}` 由 Manager 自动注入（vars 中未提供时）。
- provider 配置每次发送时从 `settings` 表实时读取（后台改配置立即生效，无需重启）。
- 短信内容**不含签名**，smsbao provider 发送时自动前缀 `【sign】`。

### 8.3 占位符清单

| 占位符 | 适用场景 | 说明 |
|---|---|---|
| `{code}` | register_verify | 验证码 |
| `{username}` | bill_notify 等 | 会员用户名 |
| `{amount}` | bill_notify | 账单金额（已格式化字符串，如 `12.00`） |
| `{bill_no}` | bill_notify | 账单编号 |
| `{due_date}` | bill_notify | 到期日（`YYYY-MM-DD`） |
| `{site_name}` | 全部 | 站点名（自动注入） |

规则：渲染 = 精确字符串替换 `{key}` → value；模板中未知占位符原样保留（便于排查）。

### 8.4 短信宝错误码映射

短信宝接口返回纯文本状态码，`0` 为成功；其余映射为可读错误（进入 `sms_send_failed` 的 msg）：

| 返回码 | 可读错误 |
|---|---|
| -1 | 参数错误 |
| -2 | 用户名或密码错误 |
| -3 | 短信内容超过长度限制 |
| -4 | 手机号数量超过限制 |
| 30 | 密码错误 |
| 33 | IP 未通过验证（需在短信宝后台报备服务器 IP） |
| 40 | 手机号码格式错误 |
| 41 | 余额不足 |
| 42 | 发送过于频繁，触发限流 |
| 50 | 内容含敏感词，被拦截 |
| 51 | 同一号码发送次数超限 |
| 其它 | 未知错误码: <原始返回> |

余额查询（`/query`）：第一行 `0` 表示成功，第二行 `已发送条数,剩余条数`。

### 8.5 短信宝 HTTP 接口（provider 内部实现）

- 发送：`GET https://api.smsbao.com/sms?u=<用户名>&p=<密码MD5>&m=<手机号,多个英文逗号分隔>&c=<内容(UTF-8, URL encode)>`
- 余额：`GET https://api.smsbao.com/query?u=<用户名>&p=<密码MD5>`
- BaseURL 可在测试中注入（httptest mock），生产固定官方地址。

---

## 9. 上游对接域 `/api/admin/upstream` 与 `/api/upstream`（批 2）

> 本章接口除 9.9（host/sync 接收端）外均要求：已安装 + JWT 管理员认证。
> **批 2 只调用上游只读端点**（登录、/cart/all、/cart/credit、/api/product/proinfo、**/api/product/prodetail**、/cart/get_product_config、/host/list）；下单/开通/续费等写操作批 3 开放。

### 9.0 上游协议摘要（魔方财务 zjmf）

- 登录：`POST {上游}/zjmf_api_login`，JSON `{"username","password"}` → `{"jwt":"...","status":200,"msg":"鉴权成功","is_aff":"1"}`；账号为手机号且**不加 +86/86 区号**；
- **响应信封**：上游业务端点的 payload 一律嵌在 `data` 键下（`{"status":200,"msg":"请求成功","data":{…},"is_aff":"1"}`），**唯一例外是登录的 `jwt` 在顶层**；客户端在 envelope 校验后统一解包 `data`（`data:null`/缺失时容错解整包，兼容顶层形状）；
- 后续请求头 `Authorization: Bearer <jwt>`；jwt 由后端缓存复用（并发安全），凭据变更自动作废；
- jwt 失效时上游回 `{"status":405,"msg":"请登陆后再试"}` → 后端自动重登并重试**一次**（防死循环）；
- 单次请求超时 `[upstream] timeout_seconds`（默认 15s）；
- 凭据运行时真源为 settings（9.1 界面维护），config.toml `[upstream]` 仅安装/启动种子。

### 9.1 上游配置

- `GET /api/admin/upstream/config` → data：

```json
{
  "hostname": "https://lyew.com",
  "username": "16650492239",
  "api_key_set": true,
  "downstream_token": "<64位hex>",
  "sync_path": "/api/upstream/host/sync"
}
```

  - `api_key` **不回显明文**（只回 `api_key_set`，范式同 notify password_set）；
  - `downstream_token` 为空时**首次 GET 自动生成并落库**（懒初始化）。
- `PUT /api/admin/upstream/config`，body `{"hostname","username","api_key"}`；`api_key` 留空 = 不修改；hostname 须为 http/https 完整 URL（存储时去尾部 `/`）；保存后缓存的上游会话立即失效重建。响应 `data: {"saved": true}`。错误码：`invalid_param`。

### 9.2 重置下游密钥

- `POST /api/admin/upstream/downstream-token/reset` → `data: {"downstream_token":"<新64位hex>"}`。旧 token 立即失效（上游需重新登记）。

### 9.3 测试连接

- `POST /api/admin/upstream/test`（无 body）：真实调上游登录 + `/cart/credit`。
- **失败也返回 HTTP 200**（测试结果即数据）：

```json
{ "status":200, "msg":"ok", "data": { "connected": true, "credit": "57664.50",
  "currency": {"code":"CNY","prefix":"¥","suffix":"元"}, "msg": "连接成功" } }
```

  失败时 `connected:false`、`msg` 为可读原因（含上游 msg / 网络错误 / WAF 拦截提示）。

### 9.4 上游余额

- `GET /api/admin/upstream/credit` → `data: {"credit":"57664.50","currency":{...}}`。错误码：`upstream_not_configured` / `upstream_auth_failed` / `upstream_request_failed`。

### 9.5 上游商品树（附本地导入状态）

- `GET /api/admin/upstream/products` → data：

```json
{ "groups": [ { "id": 1, "name": "云服务器", "products": [
  { "id": 101, "type": "dcimcloud", "name": "入门型云主机",
    "description": "<li>1核1G</li>",
    "pay_mode": "recurring",
    "prices": {"monthly":"20.00","quarterly":"60.00","semiannually":"120.00","annually":"200.00"},
    "imported": true, "local_product_id": 1, "local_name": "入门型云主机（本站）",
    "local_sale_prices": {"monthly":"30.00"}, "local_status": 1 }
] } ] }
```

  - 分组/商品 id/name/description/type/pay_mode 来自 `/cart/all`（description 已 HTML 实体解码；pay_mode 从 pay_type 标记解析）；
  - **`prices`（成本价）来自 `/api/product/prodetail`**（后端分批 ≤50 拉取合并，`-1.00` 停售周期已过滤）——不是 pay_type（其中无价格，见 6.5）；
  - `imported/local_*` = 本地映射状态（未导入时 `imported:false`、`local_sale_prices:null`）。

### 9.5a 上游端点 `/api/product/prodetail`（成本价真源，客户端内部调用）

- `GET {上游}/api/product/prodetail?pids[]=1&pids[]=2` → `data.detail` 为 `{pid字符串: 商品对象}` map；
- 商品对象的 `product_pricings[0]` 字段表（lyew.com 实测）：

| 字段组 | 键 | 说明 |
|---|---|---|
| 周期价格（17 键） | `onetime` `hour` `day` `ontrial` `monthly` `quarterly` `semiannually` `annually` `biennially` `triennially` `fourly` `fively` `sixly` `sevenly` `eightly` `ninely` `tenly` | 字符串金额；**`-1.00`/`-1` = 停售（过滤）**；`0.00` = 免费（保留） |
| 安装费（17 键，本批不导入、结构预留） | `osetupfee` `hsetupfee` `dsetupfee` `ontrialfee` `msetupfee` `qsetupfee` `ssetupfee` `asetupfee` `bsetupfee` `tsetupfee` `foursetupfee` `fivesetupfee` `sixsetupfee` `sevensetupfee` `eightsetupfee` `ninesetupfee` `tensetupfee` | 对应周期的一次性安装费 |
| 元数据 | `code`（货币码 CNY）、`id`、`type`、`currency`、`relid` | 非价格键，白名单提取天然排除 |

- 商品对象另含 `stock_control`/`qty`（导入时与成本价同源回填，不再另调 proinfo）；
- 容错（实测）：不存在的 pid → `detail` 键整个缺失或 `product_pricings: null` → 该商品空价格，不报错。

### 9.6 上游库存批量查询

- `GET /api/admin/upstream/products/stock?pids=101,201`（逗号分隔，单次 ≤200）
- → `data: {"list":[{"upstream_pid":101,"name":"…","stock_control":true,"qty":5}]}`（上游 proinfo，`pids[]` 重复键形式由后端拼装；stock_control/qty 宽松类型转换）。

### 9.7 商品导入（幂等）

- `POST /api/admin/upstream/products/import`，body：

```json
{ "items": [ {
  "upstream_pid": 101,
  "name": "本地名称（留空默认上游名）",
  "group_id": 0,
  "group_name": "新分组名（group_id=0 时必填，按名 FirstOrCreate）",
  "sale_prices": {"monthly":"15.00","annually":"150.00"},
  "status": 1
} ] }
```

- 校验（先全量校验再落库，避免半成功）：items 1–200 条；upstream_pid 正整数且批内不重复、必须存在于上游当前商品树；sale_prices 至少一个周期且每项为 >0 数字；分组二选一必填；status ∈ {0,1}（缺省 1）。
- 行为：描述/类型从 `/cart/all` 实时拉取（不信任前端），**成本价与库存从 `/api/product/prodetail` 同源回填**（失败不阻断导入，成本记空/库存记 0 并记日志）；`upstream_pid` 已存在 → **更新**映射与售价（幂等）；不存在 → 创建。
- → `data: {"created":2,"updated":0}`。错误码：`invalid_param` / `upstream_*`。

### 9.8 上游主机列表（对账只读）

- `GET /api/admin/upstream/hosts?page=1&limit=20`（limit ≤100）
- → data 透传上游结构：`{"page":1,"limit":20,"sum":N,"list":[{…上游字段原样…}],"domainstatus":{"Pending":{"name":"待激活","color":"warning"},…}}`。
- ⚠ **上游分页回显字段存在 int/string 类型漂移**（PHP 对 query 参数原样回显：带 `?page=1&limit=10` 调用时 `data.page="1"` 为字符串，不带参数时为 int；lyew.com 实测）。客户端以宽松数字类型（`upstream.FlexInt/FlexInt64`）统一解析，**本接口对前端的输出恒为 JSON 数字**，前端契约不受漂移影响。同类防御覆盖 cart/all 分组与商品 id、proinfo/prodetail 商品 id。

### 9.9 主机推送接收端 `POST /api/upstream/host/sync`

**安全模型**：免 JWT（上游服务器调用），以魔方标准 MD5 签名验证代替登录态；**不在未安装门禁白名单内**——未安装态直接 409 `not_installed`；验签失败 403。请求体上限 1MB。

- 推送字段（上游 → 下游，开通/变更主机时）：`id`（下游主机 ID，首次推送可为 0/缺省）、`host_id`（上游主机 ID，**upsert 匹配键，必填正整数**）、`domain`、`username`、`password`（明文）、`dedicatedip`、`assignedips`、`port`、`os`、`os_url`、`nextduedate`、`domainstatus`、可选 `type`（create/update 等）、`rand_str`、`signature`。
- **签名算法（逐字实现）**：取 `{id, token, rand_str}` 三字段 → 按键名 ksort（字符串序，即 `id` < `rand_str` < `token`）→ JSON 序列化（紧凑无空格、键序为排序后顺序）→ MD5 → **转大写** → 与 `signature` 全等比对（常数时间比较）。`token` = 本系统 `downstream_token`（9.1/9.2 管理）。示例签名材料：`{"id":"12","rand_str":"r4nd","token":"tk"}`。`id` 取推送体原文（数字 12 与字符串 "12" 等价）。
- 处理：验签 → 按 `host_id` upsert `hosts` 表（更新时保留 product_id/client_id 关联）→ raw 原文备份 → 返回 200。字段缺失容错（宽松转换，记日志）。
- 响应：成功 `{"status":200,"msg":"ok","data":{"received":true}}`；验签失败 `403 {"code":"signature_invalid"}`；host_id 缺失/非法 `400 invalid_param`；token 未配置 `403 signature_invalid`（msg 提示先在后台生成）。
- 「把回调地址/token 自动登记到上游」（上游 /host/setdownstream）留批 3 下单链路实现；本批由管理员在上游后台手工登记（前端展示完整回调 URL 与复制按钮）。

---

## 10. 商品域 `/api/admin/products`（批 2，本地商品管理）

> 全部要求：已安装 + JWT 管理员认证。会员前台商品接口（`/api/products`）批 3 随商城开放。

### 10.1 商品列表

- `GET /api/admin/products?group_id=&page=1&page_size=20`（page_size ≤200）
- → `data: {"list":[ProductView],"total":N}`；ProductView：

```json
{ "id":1, "group_id":1, "group_name":"云服务器套餐", "upstream_pid":101,
  "name":"…", "type":"host",
  "cost_prices":{"monthly":"10.00"}, "sale_prices":{"monthly":"15.00"},
  "stock_qty":5, "stock_control":true, "status":1, "sort":0,
  "updated_at":"2026-10-10 06:06:02" }
```

### 10.2 分组列表

- `GET /api/admin/product-groups` → `data: {"list":[{"id":1,"name":"…","sort":0,"status":1}]}`

### 10.3 编辑商品

- `PUT /api/admin/products/:id`，body 字段均可选（缺省不修改）：`{"name","group_id","sale_prices","sort"}`；name ≤255 字符；sale_prices 每周期 >0；group_id 须存在。→ data 为更新后的 ProductView。错误码：`invalid_param` / `product_not_found`。

### 10.4 上下架

- `PUT /api/admin/products/:id/status`，body `{"status":0|1}` → data 为更新后的 ProductView。

### 10.5 手动同步库存

- `POST /api/admin/products/sync-stock`（无 body）：取全部本地商品的 upstream_pid，分批（≤50/批）调上游 proinfo 更新 stock_qty/stock_control；单批失败跳过记日志（尽力而为）。→ `data: {"synced":2,"total":2}`。

---

## 11. 预留章节（后续批次追加，本批不实现）

| 章节 | 功能域 | 路由前缀（规划） |
|---|---|---|
| 13 | 会员前台商品/下单 | `/api/products`、`/api/orders` |
| 14 | 订单域（后台） | `/api/admin/orders` |
| 15 | 计费域（账单/周期） | `/api/billing`、`/api/admin/billing` |
| 16 | 支付域（网关/回调） | `/api/payment` |
| 17 | 工单域 | `/api/tickets`、`/api/admin/tickets` |
| 18 | 上游写操作（下单/开通/续费/退款转发） | `/api/admin/upstream`（扩展） |
| 19 | 管理后台其余设置 | `/api/admin/settings` |

---

## 12. 会员域 `/api/client/*` 与 `/api/admin/clients`（批 3-α）

> 会员 = `clients` 表（独立于管理员 `admins`）。会员 JWT 与管理员 JWT **双签发方隔离**（claims `iss`：会员 `liltidc-oem-client` / 管理员 `liltidc-oem`），跨用 token 必然 401。
> `/api/client/*` 受未安装门禁保护；`/api/client/auth/*`、`/api/client/captcha` 免会员 JWT，其余需 `Authorization: Bearer <会员token>`。

### 12.0 注册流程与验证码

两步注册：**① 发送验证码 → ② 提交注册**。

- 验证码：6 位数字，有效期 **10 分钟**；存储为 **SHA-256 哈希**（settings 表 `verify_code_<场景>_<标识>` 键，值 `hash|过期unix|发送unix`，明文不落库不入日志）；
- 场景 `purpose`：`register`（注册）/ `reset`（找回密码，本批仅 register，reset 留批 3-β）；
- 标识 `target`：`sms:<手机号>` 或 `email:<邮箱>`（与发送通道一致）；
- **频控（服务端强制）**：同 target+purpose **60 秒 1 条**；同 target+purpose **24 小时 10 条**；同 IP **24 小时 30 条**。超限返回 429 `rate_limited`（msg 含需等待秒数或配额说明）；
- 校验失败不消耗验证码（可重试），但**注册成功即失效**；过期/不匹配统一返回 `invalid_credentials` 级错误（见 12.1，防枚举）。

### 12.1 发送注册验证码

- `POST /api/client/auth/send-code`
- 请求体：

```json
{ "purpose": "register", "channel": "sms", "to": "13800138000",
  "username": "newuser", "email": "", "captcha_id": "<可选>", "captcha_answer": "<可选>" }
```

| 字段 | 校验 |
|---|---|
| purpose | 必填，本批仅 `register` |
| channel | `sms` / `email` |
| to | sms→手机号（宽松：数字/+/−/逗号 5–20 位，单个号码）；email→合法邮箱 |
| username | **channel=sms 时必填**（用户名预占查重：已存在 → 400 `username_exists`，短信场景无法回显错误给用户看注册表单，故发送前拦截）；email 时可空 |
| email | channel=email 时应与 to 一致或留空（注册落库用） |
| captcha_id / captcha_answer | 图形验证码（见 12.5）；**触发条件**：同 IP 当日已发送 ≥5 条后必填 |

- 成功：`data: { "sent": true, "expires_in": 600 }`
- 错误码：`invalid_param` / `username_exists`(400) / `rate_limited`(429) / `captcha_required`(400，需要图形码而未带) / `captcha_invalid`(400) / `notify_not_configured`(400) / `sms_send_failed`、`email_send_failed`(502，msg 如实透传通道错误)
- 模板：scene=`register_verify`，vars：`code`（+自动注入 `site_name`），lang 取请求头 `Accept-Language`（zh 前缀→zh-CN，否则 en-US）。

### 12.2 会员注册

- `POST /api/client/auth/register`
- 请求体：

```json
{ "username": "newuser", "password": "Passw0rd!", "confirm_password": "Passw0rd!",
  "email": "u@example.com", "phone": "13800138000", "verify_channel": "sms", "verify_code": "123456" }
```

| 字段 | 校验 |
|---|---|
| username | `^[A-Za-z0-9_]{3,32}$`，全表唯一（**注册期查重返回 `username_exists`**——用户自己选的名字可以告知占用；与登录接口的防枚举策略不同，此为产品必需） |
| password | 8–72 字符，须含字母与数字（bcrypt 哈希存储，cost 10，同 ADR-003） |
| confirm_password | 与 password 一致（`invalid_param`） |
| email | 合法邮箱，**可空**；非空则唯一（占用 → `email_exists`） |
| phone | 合法手机号（`^1[3-9]\d{9}$`），**可空**；非空则唯一（占用 → `phone_exists`） |
| verify_channel + verify_code | 必填其一组合：`sms` 需 phone 非空、`email` 需 email 非空；验证码错/过期 → 400 `verify_code_invalid` |

- 成功：`data: { "id": 1, "username": "newuser" }`（**注册后不自动登录**，前端引导去登录页）
- 事务性：验证码校验 → 唯一性终检 → 落库同一事务；并发同名注册由唯一索引兜底（转 `username_exists`）。
- 错误码：`invalid_param` / `username_exists` / `email_exists` / `phone_exists` / `verify_code_invalid`（均 400）

### 12.3 会员登录

- `POST /api/client/auth/login`
- 请求体：`{ "username": "…", "password": "…" }`——`username` 字段兼容 **用户名 / 邮箱 / 手机号** 三种主体（依次精确匹配）。
- 成功：

```json
{ "status":200,"msg":"ok","data":{ "token":"<JWT>", "expires_at":"…",
  "client": { "id":1, "username":"newuser", "email":"…", "phone":"…", "real_name":"…" } } }
```

- JWT：HS256，密钥复用 `[jwt] secret`，**issuer=`liltidc-oem-client`**，claims：`client_id`、`username`、`iat/exp`；有效期 `[jwt] client_expire_hours`（默认 24）。
- 防枚举：主体不存在与密码错误统一 401 `invalid_credentials`；禁用账号 403 `account_disabled`；**上游关联状态（status=3）不影响登录**（本地鉴权自洽，上游故障不阻断）。
- 副作用：更新 `last_login_at`。

### 12.4 会员资料

- `GET /api/client/profile` → `data: { id, username, email, phone, real_name, company, address, created_at }`
- `PUT /api/client/profile`，body 可选字段：`real_name`（≤64）/ `company`（≤128）/ `address`（≤255）/ `email`（合法且唯一，占用 → `email_exists`）/ `phone`（合法且唯一，占用 → `phone_exists`）；`username` 不可改。→ data 为更新后完整资料。
- `POST /api/client/change-password`，body `{ "old_password", "new_password", "confirm_password" }`：旧密码错 → 401 `invalid_credentials`；新密码规则同注册；成功后**当前 token 仍有效**（批内不做全端下线，见 ADR-025 遗留）。

### 12.5 图形验证码

- `GET /api/client/captcha`（免鉴权、免安装态白名单外——受门禁保护）→ `data: { "captcha_id": "<uuid>", "image": "data:image/png;base64,…", "expires_in": 300 }`
- 答案存 settings `captcha_<id>`（哈希，5 分钟过期）；**校验即焚**（无论对错，用后删除）；
- 校验位点：`send-code` 带 `captcha_id+captcha_answer` 时先验图形码（错 → `captcha_invalid`）；同 IP 当日发送 ≥5 条后未带 → `captcha_required`。
- 生成：纯 Go（image 标准库）绘制 4 位字符（去 0/O/1/l/I 混淆），干扰线+噪点，120×40 PNG。

### 12.6 后台会员管理 `/api/admin/clients`（管理员 JWT）

- `GET /api/admin/clients?keyword=&status=&page=1&page_size=20`
  - keyword 模糊匹配 username/email/phone；status 精确过滤（枚举见 12.7）。
  - → `data: { "list":[ClientView], "total": N }`；ClientView：`{ id, username, email, phone, real_name, company, status, status_text, upstream_uid, upstream_linked_at, last_login_at, created_at }`（status_text 为后端按 Accept-Language 输出的状态文案，前端直接展示）。
- `PUT /api/admin/clients/:id/status`，body `{ "status": 0|1|3 }`（2=删除仅能经 DELETE 达成）→ data 为更新后 ClientView。
- `POST /api/admin/clients/:id/reset-password`，body `{ "new_password": "…" }`（8–72 位）→ `data: { "reset": true }`；管理员重置**不需要**旧密码，操作记服务端日志（含 admin_id 与目标 client_id）。
- `DELETE /api/admin/clients/:id` → 软删除：`status=2, deleted_at=NOW`；**username/email/phone 追加 `#del<id>` 后缀释放唯一索引**（同名可重新注册）；再次 DELETE 幂等返回成功。→ `data: { "deleted": true }`

### 12.7 clients 状态枚举

| status | 含义 | 可登录 | 说明 |
|---|---|---|---|
| 0 | 禁用 | 否（403 account_disabled） | 后台手动禁用 |
| 1 | 激活 | 是 | 注册默认值 |
| 2 | 已删除 | 否（视为不存在） | 软删除，列表默认**不显示**（status=2 过滤可显式查出） |
| 3 | 上游关联异常 | **是** | 上游开户失败/失联标记，仅列表醒目展示，不影响本地任何功能 |

### 12.8 `clients` 表结构

| 列 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint unsigned | PK, AUTO_INCREMENT | |
| username | varchar(64) | UNIQUE, NOT NULL | 登录名（软删除时追加 `#del<id>` 后缀） |
| password_hash | varchar(100) | NOT NULL | bcrypt |
| email | varchar(128) | UNIQUE NULL | 可空（MySQL 唯一索引允许多 NULL）；软删除加后缀 |
| phone | varchar(20) | UNIQUE NULL | 可空；软删除加后缀 |
| real_name / company / address | varchar(64/128/255) | | 资料 |
| status | tinyint | NOT NULL 默认 1 | 见 12.7 |
| upstream_uid | bigint NULL | | 上游会员 ID（批 3-β 开户回填） |
| upstream_linked_at | datetime(3) NULL | | 上游关联时间 |
| last_login_at | datetime(3) NULL | | |
| deleted_at | datetime(3) NULL | | 软删除时间 |
| created_at / updated_at | datetime(3) | | |

### 12.9 settings 新增键（本域内部状态，不经后台界面）

| 键模式 | 值格式 | 说明 |
|---|---|---|
| `verify_code_<purpose>_<target>` | `<sha256hex>\|<过期unix>\|<发送unix>` | 验证码（哈希存储）；target 形如 `sms:13800138000` |
| `vcode_rl_<purpose>_<target>` | `<窗口起点unix>\|<窗口内计数>` | 同 target 频控（60s/1条 + 24h/10条 滑动窗口） |
| `vcode_ip_<ip>` | `<当日unix>\|<当日计数>` | 同 IP 日计数（≥5 要求图形码；≥30 拒绝） |
| `captcha_<uuid>` | `<sha256hex>\|<过期unix>` | 图形验证码答案（校验即焚） |

> 新增错误码（并入 0.4 总表）：`username_exists`(400)、`email_exists`(400)、`phone_exists`(400)、`verify_code_invalid`(400)、`rate_limited`(429)、`captcha_required`(400)、`captcha_invalid`(400)、`client_not_found`(404)。

> 路由分层约定：`/api/admin/*` = 管理后台（管理员 JWT）；`/api/client/*` = 会员前台（会员 JWT，批 3+）；`/api/public/*` = 免鉴权公共接口；`/api/upstream/*` = 上游回调（签名验证，免 JWT）；`/api/*` 其余为安装/认证等特殊域。
