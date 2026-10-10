# LiltIDC_OEM 架构与技术决策记录（批 1）

> 本文件记录批 1 的关键实现取舍。后续批次如有新决策，按编号追加。

## ADR-001 Go module 名

**决策**：`github.com/LangfordKuo/LiltIDC_OEM/backend`

后端所有内部包以 `github.com/LangfordKuo/LiltIDC_OEM/backend/internal/...` 引用。

## ADR-002 ORM 与迁移方案：GORM + AutoMigrate

**决策**：ORM 用 GORM（mysql driver）；迁移用 **GORM AutoMigrate**（启动时与安装时幂等执行），不采用版本化 SQL 迁移。

**理由**：
- 批 1 表结构极小（3 张表）且后续批次会高频加表/加列，AutoMigrate 以 model 结构体为单一真源，改结构体即改表，开发速度最快；
- AutoMigrate 幂等（只增不删不改类型收窄），重复启动安全；
- MySQL 5.7 无现代迁移工具链优势（如 DDL 事务），版本化 SQL 的收益要等到出现**破坏性变更/数据迁移**需求时才体现；
- 折中约定：从出现第一个破坏性变更的批次起，引入 `schema_version` 表 + 版本化 SQL 做增量数据迁移，AutoMigrate 继续负责加表加列。此约定在批 2+ 执行。

## ADR-003 密码哈希：bcrypt（cost=10）

**决策**：管理员密码用 `golang.org/x/crypto/bcrypt`，cost = 10（默认）。

**理由**：bcrypt 是标准库级成熟方案、纯 Go 实现无 cgo 依赖（Windows 交叉编译友好）；argon2id 理论更强但需要额外调参（内存/并行度）且 PHP 系上游（魔方）生态不通用。密码长度上限 72 字节由参数校验兜底（8–72）。

## ADR-004 安装状态存储：config.toml `[install] installed`

**决策**：不建 `install_state` 表。持久化真源 = `config.toml` 的 `[install] installed`；运行时真源 = 进程内 `atomic.Bool`（安装完成即原子置 true，无需重启进程）。

**理由**：未安装时数据库可能根本不可达（还没配置），从库里读安装状态是鸡生蛋问题；且安装完成本来就要回写数据库配置到 config.toml，顺手写 `installed = true` 零成本。防重装：接口层双检（原子布尔 + 互斥锁）。

## ADR-005 JWT

**决策**：HS256（`github.com/golang-jwt/jwt/v5`）。secret 存于 config.toml，安装时若为空自动生成 32 字节随机数的 64 位十六进制串。有效期 `[jwt] expire_hours`（默认 24h）。Claims 含 `admin_id`、`username`。批 1 不做 token 吊销/刷新（登出 = 前端清除）。

## ADR-006 统一响应体与错误处理

**决策**：见 docs/api-contract.md 第 0 章。错误码字符串常量集中在 `internal/pkg/errcode`；业务错误用 `*errcode.BizError`（HTTP 状态 + code + msg）在服务层构造，handler 统一经 `internal/pkg/response` 输出；panic 由自定义 Recovery 中间件兜底为 500 `internal_error`（记录堆栈日志）。

## ADR-007 i18n 方案（前端）【批 1 返工修订】

**决策**：`i18next` + `react-i18next` + `i18next-browser-languagedetector`。

- 语言包：`src/i18n/locales/{zh-CN,en-US}.json`；
- 检测顺序：localStorage（键 `liltidc_lang`）→ 浏览器语言；回退语言 `zh-CN`；
- **`supportedLngs` 必须同时列地区码与裸码：`["zh-CN", "en-US", "zh", "en"]`**（见下方陷阱说明）；
- 手动切换写入 localStorage 持久化；
- `src/i18n/index.ts` 导出 `i18nReady`（init Promise），bootstrap 与测试均先 await 再渲染/断言，杜绝首帧闪 key；
- 后端错误 `msg` 不做翻译（管理员场景，如实展示底层原因优先）。

**⚠ 陷阱记录（批 1 P0 缺陷根因，i18next v26.4.2 源码确证，勿回退）**：

`nonExplicitSupportedLngs: true` 时，`LanguageUtil.isSupportedCode(code)` 会**先把语言码削成裸码**（`"zh-CN"→"zh"`）再检查是否在 `supportedLngs` 中；而 `supportedLngs` 是 LanguageUtil **构造时的快照**（运行时改 `languageUtils.options.supportedLngs` 无效）。批 1 只列了地区码 `["zh-CN","en-US"]`，削出的 `"zh"` 不在其中 → `isSupportedCode("zh-CN")===false` → `toResolveHierarchy("zh-CN")` 返回**空数组** → translator 无可解析语言 → 全站 `t()` 原样吐 key，且 `tsc`/`build`/`lint` 三道关都无法发现（类型与语法完全合法）。

修复后实测：`isSupportedCode("zh-CN")=true`、`toResolveHierarchy("zh-CN")=["zh-CN","zh"]`、`t()` 返回真实译文。防回归由 `src/i18n/i18n.smoke.test.ts` 硬性把关（ADR-014）。注意 `load` 保持默认（不可设 `languageOnly`，那会走另一条分支）。

## ADR-008 品牌主色应用方式（前端）

**决策**：`GET /api/public/brand` 返回十六进制主色（如 `#2563eb`）。前端在 JS 中计算该颜色的相对亮度，选择黑/白前景色，然后运行时写 CSS 变量到 `document.documentElement`：

```
--primary: <品牌色>;
--primary-foreground: <按亮度自动选 #fff 或 #18181b>;
--ring: <品牌色>;
```

shadcn/ui（Tailwind v4，`@theme inline` 映射 `--color-primary: var(--primary)`）的所有 `bg-primary`/`text-primary` 等工具类随之整体换肤；全站仅亮色模式（暗色模式已按 ADR-016 移除）。写入时机：应用 bootstrap（首屏渲染前，避免闪烁）与安装完成后。

## ADR-009 notify 层设计

**决策**：
- `Sender` 接口：`Send(ctx, *Message) error`，`Message{To, Subject, Content}`；
- 两个 provider：`notify.Smsbao`（短信宝 HTTP，BaseURL 可注入便于 httptest 测试）、`notify.SMTP`（net/smtp，STARTTLS 可选）；
- `notify.Manager`：唯一业务入口 `Send(ctx, scene, channel, lang, receiver, vars)`——加载模板（精确 scene+channel+lang，缺失回退 zh-CN）→ 占位符渲染（`{key}` 字符串替换，`{site_name}` 自动注入）→ 从 `settings` 表实时构造 provider → 发送；另有 `SendRaw(ctx, channel, to, subject, content)` 供后台"发送测试"绕过模板；
- provider 配置（短信宝账号/SMTP 参数）**每次发送实时读库**，后台保存立即生效，无需重启；config.toml 的 `[smsbao]`/`[smtp]` 只是安装时的种子值；
- 短信宝密码统一存 MD5（后台提交 32 位十六进制则原样存，否则后端自动 MD5）；
- 批 1 不接任何业务触发；批 2+ 只需注入 `*notify.Manager` 调 `Send`。

## ADR-010 未安装门禁与安装热切换

**决策**：Gin 全局中间件 `InstallGate`：运行时未安装 → 白名单（install 三接口 + `/api/public/brand` + `/api/healthz` + 所有非 `/api/` 路径）之外一律 409 `not_installed`。安装完成通过 `atomic.Pointer[gorm.DB]` / `atomic.Pointer[notify.Manager]` / `atomic.Bool` 热切换，所有请求路径按请求读取指针，**不要求重启进程**。前端配合：bootstrap 先取 `/api/install/status` 缓存到模块级变量，路由守卫**渲染期同步读缓存**（避免 useEffect 异步守卫闪回）；api 客户端收到 409 `not_installed` 时刷新缓存并跳转 `/install`。

**守卫结构修正（批 1 缺陷修复）**：旧版 `RequireInstalled` 只包在 `/admin` 路由上，`/` 与 `/login` 为裸路由——未安装态访问根路径不会跳安装向导，与「首次访问跳转 install」需求不符。修正为**顶层无 path 布局路由**形态（`<Route element={<RequireInstalled />}>`，无 children 时渲染 `<Outlet />`）：除 `/install`（守卫外唯一路由，其「已安装跳回 /」自检在页面内部）外，`/`、`/login`、`/admin/*`、`*` 兜底全部受保护；**后续批次新增路由默认在门禁内**，杜绝逐条包守卫的遗漏（本缺陷根因）。路由树集中在 `src/routes/app-routes.tsx`（不含 Router 本体，测试用 MemoryRouter 包裹），回归测试见 `src/routes/guards.test.tsx`（未安装/已安装/未知三态 × 多路径）。`/admin` 双重守卫次序不变：未装 → `/install`，已装未登录 → `/login`；未知态（null）仍放行，由 API 层 409 兜底。

## ADR-011 前端工程【批 1 返工修正版本记录】

**决策**：Vite 8（rolldown 内核）+ React 19.3 + TypeScript 6.0（strict，paths 不再依赖已废弃的 baseUrl）+ Tailwind CSS v4（`@tailwindcss/vite` 插件，无 tailwind.config）+ shadcn/ui（new-york 风格、neutral 基色、CSS 变量主题；组件基于 `radix-ui` 统一包与 `cn` 工具包）+ React Router 7 + ESLint 10（flat config，typescript-eslint + react-hooks v7【flat 配置位于 `configs.flat`】+ react-refresh）。路径别名 `@` → `src/`。**仅亮色主题**（暗色模式与 `next-themes` 依赖已按 ADR-016 移除）。测试栈见 ADR-014。

> 批 1 报告曾按规划写 Vite 7 / TS 5.9 / ESLint 9，实际 npm 解析到上述最新版本，此处以实际为准。

## ADR-012 后端目录分层

```
backend/
├── cmd/server/main.go        # 入口：加载配置→连库(已安装时)→组装路由→优雅退出
├── internal/
│   ├── config/               # TOML 配置加载/校验/回写
│   ├── router/               # 路由组装与依赖注入
│   ├── handler/              # HTTP 层：绑定参数、调 service、统一响应
│   ├── service/              # 业务层：安装/认证/品牌/通知配置
│   ├── model/                # GORM 模型 + settings 键常量与读写助手
│   ├── middleware/           # 门禁/JWT认证/CORS/请求日志/Recovery
│   ├── migration/            # 数据库连接、AutoMigrate、种子数据
│   ├── notify/               # Sender 接口、smsbao/smtp provider、Manager、模板渲染
│   ├── upstream/             # 上游魔方财务（批 1 仅预留结构，无调用逻辑）
│   └── pkg/                  # app 容器、errcode、response、jwtutil 等横切包
├── config.example.toml
└── go.mod
```

依赖方向（无环）：`router → handler → service → {model, notify, migration, config}`；`middleware/handler/service → pkg/app`；`pkg/app → {config, notify}`。

## ADR-013 数据库字符集/排序规则

**决策**：库级 `utf8mb4` / `utf8mb4_general_ci`（MySQL 5.7 默认，兼容性最好），InnoDB。DSN 固定 `charset=utf8mb4&parseTime=True&loc=Local`。

## ADR-014 前端测试基础设施（批 1 返工新增）

**决策**：`vitest`（jsdom 环境，`npm run test` = `vitest run` 一次跑完退出）+ `@testing-library/react` + `@testing-library/jest-dom`。配置文件 `vitest.config.ts`（mergeConfig 复用 vite.config.ts 的别名与插件），setup 在 `src/test/setup.ts`（注册 jest-dom 断言、补 jsdom 缺失的 `window.matchMedia`、用例后 cleanup）。

**为什么必须存在（硬性门禁的理由）**：批 1 的 i18n 全站失效缺陷（ADR-007）穿过了 `tsc`/`build`/`lint` 三道关——类型与语法完全合法，只有**运行时断言译文**才能发现。因此约定：

1. **i18n 冒烟测试是硬性门禁**（`src/i18n/i18n.smoke.test.ts`）：直接断言 `languageUtils.isSupportedCode("zh-CN")===true`、`toResolveHierarchy` 非空、代表 key 的 `t()` 返回值等于 locale 文件真实译文且不等于 key、`changeLanguage("en-US")` 后返回英文。任何人改动 i18n 配置（尤其 `supportedLngs`/`nonExplicitSupportedLngs`/`load`）都会被它拦截；
2. **locale 完整性测试**：中英 key 集合必须完全一致、叶子值均为非空字符串（缺 key 同样表现为界面吐 key 原文）；
3. **页面渲染冒烟测试**：首页/登录/安装向导第一步/后台布局渲染后，用正则 `\b[a-z]+\.[a-zA-Z]+\b` 扫描**可见文本**（克隆容器并剔除 script/style 后取 textContent，避免第三方库注入脚本的源码误报），出现任何未翻译 key 即失败；
4. CI/交付前 `npm run test` 必须全绿，与 lint/tsc/build 并列为四道关。

## ADR-015 前端代码分割（批 1 返工新增）

**决策**：`/admin` 路由树全部 `React.lazy` + 动态 import（App.tsx），AdminLayout 内 `<Outlet>` 外包 `Suspense`（切换菜单时侧边栏/顶栏保持挂载，仅内容区显示过渡占位）；公共页（home/login/install/not-found）保持静态导入，保证首屏与未安装态零额外请求。**不引入 manualChunks/advancedChunks 手工分组**——纯路由级分割已让主入口从 609.84 kB 降到 381.21 kB（<500 kB 警告线），更细的 vendor 分组留到出现真实需要时再做。

效果（vite build，gzip 前）：主入口 `index.js` 381.21 kB；admin 侧 `settings` 54.30 kB、共享依赖块 115.12 kB / 42.85 kB / 5.25 kB、`layout` 10.32 kB、`dashboard`/`placeholder`/图标块各 <2 kB；无 500 kB 警告。

## ADR-016 移除暗色模式（2026-10-10 Langford 决定）

**决策**：移除暗色模式，**仅保留亮色 + 品牌主色可配**（主色仍按 ADR-008 运行时注入 CSS 变量）。

清理范围：`theme-toggle.tsx` 组件删除；`next-themes` 依赖卸载（`ThemeProvider` 从 main.tsx 移除，`ui/sonner.tsx` 改为固定 `theme="light"`）；index.html 防闪烁内联脚本删除；index.css 的 `.dark` 变量块删除；四个页面（home/login/install/admin layout）的切换按钮移除。

**保留与例外（勿"顺手清理"）**：
1. `src/components/ui/*.tsx` 中 shadcn 官方模板自带的 `dark:` 变体类**保留原样**——保持与官方模板一致，日后升级组件零冲突；
2. index.css 中保留一行**永假**的 `@custom-variant dark`（钉到不存在的占位 class `liltidc-dark-removed`）。原因：Tailwind v4 删除自定义 dark variant 后会**回退到 `prefers-color-scheme: dark` 媒体查询**，届时系统深色的用户会命中 ui 组件保留的 `dark:` 类，而 `.dark` 变量块已删除，将产生半深半浅的样式错乱。永假定义保证 `dark:` 类在任何系统偏好下都不命中（零副作用），这正是"保留 dark: 类"决策成立的前提。

## ADR-017 `<html lang>` 跟随语言切换（批 1 收尾）

**决策**：`src/i18n/index.ts` 中注册 `i18n.on("languageChanged")` 同步 `document.documentElement.lang`，并在 `i18nReady` 完成后按检测结果立即设置一次（首屏正确）。无障碍（读屏发音）、SEO 与浏览器翻译提示依赖该属性。回归断言在 `i18n.smoke.test.ts`。

## ADR-018 CORS 白名单配置化（批 1 收尾）

**决策**：CORS 允许源从硬编码改为配置项 `[server] cors_allow_origins`（字符串数组），默认 `["http://localhost:5173","http://127.0.0.1:5173"]`；`config.applyDefaults` 与 `middleware.CORS`（空白名单回落默认，防 gin-contrib/cors panic）双层兜底。生产 Nginx 同域反代不依赖 CORS；前后端分域部署时按真实前端源配置。

## ADR-019 上游凭据真源：settings 表（批 2）

**决策**：上游魔方财务的地址/账号/密钥运行时真源是数据库 `settings`（键 `upstream_hostname` / `upstream_username` / `upstream_api_key`），后台「上游对接」页可改、改后**立即生效无需重启**；`config.toml [upstream]` 仅作为安装/启动时的种子（对应 settings 键**不存在时**才写入，永不覆盖后台修改）——与 notify 通道配置同一范式（ADR-009）。**凭据明文存储**（Langford 明确偏好简单方案；settings 表不对外输出 api_key，接口只回 `api_key_set`）。

`upstream.SessionManager`（挂在 `app.App`，与 notify.Manager 同范式）负责 Client 生命周期：每次取用先读 settings 计算**凭据指纹**（hostname+username+api_key），指纹不变复用缓存 Client（jwt 随之存活），变化则重建（旧 jwt 作废）；后台保存配置时额外显式 `Invalidate()`。

## ADR-020 上游 JWT 缓存与重试策略（批 2）

**决策**：`upstream.Client` 内部 `sync.Mutex` 保护 jwt 缓存（登录与读写串行化，并发安全）；请求流程：无缓存 jwt → 先登录；带 jwt 请求收到上游 `{"status":405}`（jwt 失效）→ 清空缓存、重登、**重试一次**——仍 405 则放弃并报错（防死循环，单测覆盖恰好 2 次登录 + 2 次业务请求）。上游无显式过期时间接口，「405 驱动重登」即过期兜底。超时 `[upstream] timeout_seconds`（默认 15s）作用于全流程（连接+读）。上游响应 envelope 宽松判定：`status==405` → 重登；`status` 存在且非 0/200 → 业务错误（携带上游 msg）；无 status 字段 → 视为成功（部分端点成功响应不含 status）。响应体上限 8MB、非 JSON（如 WAF 拦截页）转可读错误。

**数字类型漂移宽松化（批 2 小修补记）**：上游 PHP 对 query 参数**原样回显**——`/host/list` 带 `?page=1&limit=10` 调用时 `data.page="1"`（字符串），不带参数时 `data.page=1`（int），而 sum/max_page 恒为 int（lyew.com 实测）。解析层统一用 `upstream.FlexInt/FlexInt64`（UnmarshalJSON 同时接受数字/数字字符串/null/空串，Marshal 输出仍为 JSON 数字，上层契约不变）；防御性同用位点：cart/all 分组与商品 id、proinfo/prodetail 商品 id。回归位点 `flexdrift_test.go`（int/string/混合三形态 + 三端点 string id）。

**data 信封解包（批 2 P0 修复补记）**：魔方业务端点的 payload 一律嵌在响应 `data` 键下（`{"status":200,"msg":"请求成功","data":{…},"is_aff":"1"}`），**唯一例外是登录 `jwt` 在顶层**（走 `loginLocked` 独立路径）。`doOnce` 用单次 `map[string]json.RawMessage` 解析同时取 status/msg 与 data：有 `data`（非 null）解包 data，否则解整包（兼容顶层形状）；`data:null` 容错。初版直接把整包解进 out struct（字段全声明在顶层）导致对真实上游全部解析为空值、且 `TestConnection` 出现「connected:true 但 credit 空」的误导性成功——教训：**httptest fixture 必须逐字对齐真实响应形状（含信封与噪声字段）**，回归测试位点 `internal/upstream/envelope_test.go`（fixture 取自与 lyew.com 形状一致的 mock 实测原文，修复前红/修复后绿）；`TestConnection` 同步修正为 credit 空时 msg 如实标注部分成功。

## ADR-021 host/sync 签名验证与安全模型（批 2）

**决策**：接收端 `POST /api/upstream/host/sync` **免 JWT**（上游服务器调用），以魔方标准签名验证代替登录态：`{id, token, rand_str}` 按键名 ksort（字符串序 `id`<`rand_str`<`token`）→ 紧凑 JSON → MD5 → 大写 → 与 `signature` 全等。实现要点：
- Go 的 `json.Marshal(map[string]string)` 天然按键名字典序、紧凑无空格输出，与 PHP `ksort + json_encode` 逐字节等价（单测以**手工拼接的 PHP 等价实现**交叉自证）；
- 推送体用 `json.Decoder.UseNumber()` 解析，`id` 保留 JSON 原文（数字 `12` → `"12"`），与 PHP 端字符串化一致；
- 比对用 `crypto/subtle.ConstantTimeCompare`（防时序侧信道），signature 大小写归一；
- 安全边界：**不在未安装门禁白名单**（未安装 409）、`downstream_token` 未配置一律 403（fail-closed）、请求体 1MB 上限、原文备份进 `hosts.raw`（60KB 截断）供溯源；
- upsert 按 `upstream_host_id` 匹配，更新时保留 `product_id/client_id`（批 3 回填字段）。⚠ 实现坑（已修，端到端实测抓出）：GORM map 形式 `Updates` 的键**直接当列名**（不走命名策略），必须用蛇形列名而非 JSON 字段名。

## ADR-022 商品映射模型（批 2）

**决策**：本地 `products.upstream_pid` **唯一索引**是上下游映射锚点——重复导入同 pid = 更新映射与售价（幂等，天然防重复商品）；导入时成本价/描述/类型**从上游实时拉取**（不信任前端提交，防篡改），售价由管理员逐周期自定义（每周期必填 >0，界面展示成本价与利润率）；价格存 **JSON 文本**（`map[周期]字符串金额`）而非 MySQL JSON 类型（5.7 支持有限）且金额用字符串（防浮点误差）；分组按名称唯一（导入 `FirstOrCreate`，`group_id` 显式指定优先）。周期枚举与上游 `product_pricings` 键**完全一致**（lyew.com 实测 17 键，本地规范键统一采用上游键名——含 `semiannually` 无下划线形态，免映射最不易错；展示排序 onetime→tenly）。

**⚠ 成本价真源 = `/api/product/prodetail` 的 `product_pricings[0]`（批 2 第二个 P0 教训）**：真实上游 `/cart/all` 的商品对象**不含任何价格字段**；其 `pay_type`（字符串化 JSON）只有付费模式标记（`pay_type:"recurring"/"onetime"`）与**周期时长定义**（`pay_hour_cycle:"720"` 是小时数、`pay_day_cycle:"30"` 是天数——初版解析器把这些当价格产出 `map[pay_hour_cycle:720 …]` 垃圾成本）。修复后：`pay_type` 仅经 `ParsePayMode` 取付费模式；价格/库存/安装费全部走 `GetProductDetail`（白名单 17 周期键 + 17 setupfee 键分离提取，`-1.00` 停售过滤、`0.00` 免费保留、`detail` 键缺失与 `product_pricings:null` 双容错——均实测形态）；setupfee 本批解析不导入（结构预留批 3）。`ImportProducts` 的成本与库存改为 prodetail 同源（不再调 proinfo）。回归位点：`prodetail_test.go`（真实 fixture 逐字取自 lyew.com 响应）+ `products_test.go` 的 `TestGetProductsNoPriceInPayType`（行为级：真实 pay_type 必须解析出空价格）。

**教训二则**：① 中间人 mock 的响应形状不能替代真实上游样本（第一轮 P0 修复用的 mock 形状与真上游不同，导致「已复核」结论错误）；② 对上游字段语义不确定时（pay_hour_cycle 到底是时长还是价格），必须先拿真实样本再写解析器。

## ADR-023 会员表设计与软删除唯一性释放（批 3-α）

**决策**：`clients` 独立于 `admins`（双体系互不相干）；`email`/`phone` 用 **可空指针列 + 唯一索引**（MySQL 唯一索引允许多行 NULL，与「可空但占用查重」语义吻合，Go 侧 `*string`）。密码 bcrypt cost=10（同 ADR-003），强度 8–72 位且必须同时含字母与数字。

**软删除**：`status=2 + deleted_at`，同时 username/email/phone **追加 `#del<id>` 后缀**（超长截断）释放唯一索引——已删除账号不阻塞同名重新注册（端到端实测：软删 member01 后同名注册成功、两行并存）。不用 `gorm.DeletedAt` 全局软删（避免隐式 scope 改变批 1/2 既有查询语义），`deleted_at` 为普通 `*time.Time` 列。重复 DELETE 幂等返回成功；软删账号登录按「不存在」处理（`invalid_credentials`，防枚举）。`upstream_uid`/`upstream_linked_at` 本批建列不写入（批 3-β 开户回填）；`status=3`（上游关联异常）**只作列表醒目提示，不阻断登录**（本地鉴权自洽，上游故障不殃及会员）。

## ADR-024 会员 JWT 双签发方隔离（批 3-α）

**决策**：会员 token 与管理员 token 同用 `[jwt] secret`（HS256）但 **issuer 不同**：管理员 `liltidc-oem`（批 1）/ 会员 `liltidc-oem-client`。`jwtutil.SignClient/ParseClient` 强制 `WithIssuer(IssuerClient)`——管理员 token 访问 `/api/client/*` 或反向越权**必然 401**（单测双向覆盖 + 端到端实测）。claims：`client_id`、`username`；有效期 `[jwt] client_expire_hours`（默认 24h，独立于管理员）。`ClientAuthRequired` 中间件每请求查一次 `clients` 复核状态（禁用 403 / 删除 401）——会员端 QPS 低，状态即时生效优先于省一次查询；改密**不吊销**存量 token（无 token 版本号机制，遗留项：批 4 视需要引入）。前端 token 分键存储：`liltidc_token`（管理员）/ `liltidc_client_token`（会员），api 客户端按 `scope` 选用并区分 401 清理路径。

## ADR-025 验证码体系：存储、频控与图形码（批 3-α）

**决策**：
- **存储**：settings 表 KV（零建表、与安装态天然共存）；验证码存 `SHA-256` 哈希（值格式 `hash|过期unix|发送unix|试错次数`），**明文只存在于发送内容中**（模板 `{code}` 渲染），不落库不入日志（日志仅记 purpose/channel/lang）；
- **频控**（服务端强制，全部 settings 计数、发送成功才落计数——失败不消耗配额）：同 target+purpose 60s/1 条、24h/10 条（429 `rate_limited`，msg 含等待秒数）；同 IP 当日 ≥5 条强制图形码（`captcha_required`）、≥30 条拒绝；同一验证码试错 ≥5 次作废（防 10 分钟窗口在线爆破 10^6）；校验失败不消耗、注册成功显式 Consume（防重放）；
- **图形码**：纯 Go 标准库生成（`internal/pkg/captcha`，5×7 点阵字体放大 + 干扰线噪点，字符集去 0/O/1/I/L），答案哈希存 settings（5 分钟），**校验即焚**（无论对错先删）；比对统一小写归一；
- **通道**：复用批 1 notify.Manager（scene=register_verify，模板可后台改，`{site_name}` 自动注入）；lang 按请求 `Accept-Language`（zh→zh-CN，en→en-US）选模板；通道未配置 → 400 `notify_not_configured`（fail-fast，不误报发送成功）；
- **注册流程次序**：验证码校验（不消耗）→ 唯一性预检 → 落库（唯一索引兜底并发，冲突翻译为 `*_exists`）→ Consume 验证码——无显式事务，靠「索引兜底 + 消耗在最后」保证一致性；
- sms 通道 send-code 前置**用户名预占查重**（短信场景无法回显表单错误，发送前拦截 `username_exists`）。
- 端到端验证（mock SMTP 收信提取真实验证码走完全流程）：发送/注册/重放拒绝/频控 429/爆破作废/图形码生成，全部通过，见批 3-α 交付报告。
