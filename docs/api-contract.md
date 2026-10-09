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

### 0.4 错误码总表（批 1）

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

### 6.4 安装状态存储

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
secret = ""            # HS256 密钥；安装时若为空自动生成 64 位十六进制随机串
expire_hours = 24      # token 有效期（小时）

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

[upstream]             # 上游魔方财务（批 2 实现调用逻辑，本批仅预留）
hostname = ""
username = ""
api_key = ""

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

## 9. 预留章节（后续批次追加，本批不实现）

| 章节 | 功能域 | 路由前缀（规划） |
|---|---|---|
| 10 | 会员前台认证/资料 | `/api/client/*` |
| 11 | 商品域 | `/api/products`、`/api/admin/products` |
| 12 | 订单域 | `/api/orders`、`/api/admin/orders` |
| 13 | 计费域（账单/周期） | `/api/billing`、`/api/admin/billing` |
| 14 | 支付域（网关/回调） | `/api/payment` |
| 15 | 工单域 | `/api/tickets`、`/api/admin/tickets` |
| 16 | 上游魔方财务对接（开通/运维动作转发） | `/api/admin/upstream` |
| 17 | 管理后台其余设置 | `/api/admin/settings` |

> 路由分层约定：`/api/admin/*` = 管理后台（管理员 JWT）；`/api/client/*` = 会员前台（会员 JWT，批 3+）；`/api/public/*` = 免鉴权公共接口；`/api/*` 其余为安装/认证等特殊域。
