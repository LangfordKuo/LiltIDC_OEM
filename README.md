# LiltIDC_OEM

LiltIDC_OEM，专为岭云互联代理商提供的 IDC 财务系统。对标魔方财务/WHMCS：自带会员/商品/订单/计费/支付/工单/管理后台；不对接 PVE 等虚拟化层，开通与运维动作转发上游魔方财务执行（批 2 起实现）。

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go 1.27 + Gin + GORM，REST JSON API |
| 前端 | React 19 + TypeScript + Vite 7 + Tailwind CSS v4 + shadcn/ui（new-york） |
| 数据库 | MySQL 5.7（utf8mb4） |
| 生产部署 | Nginx 托管前端静态产物 + 反代 `/api` 到后端 |

## 目录结构

```
LiltIDC_OEM/
├── backend/    Go + Gin 后端（module: github.com/LangfordKuo/LiltIDC_OEM/backend）
├── frontend/   React + Vite 前端
└── docs/       api-contract.md（接口契约）、architecture.md（技术决策）
```

## 开发环境要求

- Go >= 1.27（Windows 下纯 Go 依赖，无需 cgo/gcc）
- Node.js >= 20（开发基线 v24.x，npm >= 10）
- MySQL 5.7（本机 127.0.0.1:3306）

## 快速开始（本机开发）

### 1. 后端

```bash
cd backend
cp config.example.toml config.toml   # Windows: copy config.example.toml config.toml
go run ./cmd/server                  # 默认监听 127.0.0.1:8080
```

- 配置路径可用环境变量 `LILTIDC_CONFIG` 覆盖（默认 `./config.toml`）。
- `config.toml` 不入库；`[install] installed = false` 时仅安装向导与公共接口可用（其余返回 409 `not_installed`）。
- 首次使用：启动前端后访问 `http://localhost:5173/install` 走安装向导（测库 → 建库建表 → 创建管理员 → 自动写入配置并切换为已安装态，无需重启）。

### 2. 前端

```bash
cd frontend
npm install
npm run dev        # http://localhost:5173，/api 已代理到 127.0.0.1:8080
```

页面：`/`（前台占位）、`/install`（安装向导）、`/login`（管理员登录）、`/admin`（后台壳：仪表盘/商品/订单/会员/工单/财务/系统设置）。系统设置页含短信宝、SMTP 配置（带发送测试/余额查询）与消息模板管理。

## 测试与质量

```bash
# 后端
cd backend
gofmt -l .            # 应为空
go vet ./...
go build ./...
go test ./...

# 前端
cd frontend
npm run test          # vitest（jsdom）：i18n 冒烟/locale 完整性/品牌/页面渲染，一次跑完退出
npm run lint          # ESLint（flat config）
npx tsc --noEmit -p tsconfig.app.json
npm run build         # tsc -b + vite build（/admin 路由已做代码分割）
```

## 生产构建

```bash
# 后端：产出 backend/liltidc-server(.exe)
cd backend && go build -o liltidc-server ./cmd/server

# 前端：产出 frontend/dist/
cd frontend && npm run build
```

Nginx 参考：`root` 指向 `frontend/dist`，`location /api` 反代 `http://127.0.0.1:8080`；后端 config.toml 置 `mode = "release"`、`addr = "127.0.0.1"`。（也可在后端 `[server] static_dir` 直接指定 `frontend/dist` 由 Go 托管静态资源。）

## 文档

- [docs/api-contract.md](docs/api-contract.md)：接口契约（路径/请求/响应/错误码/表结构/配置项/通知枚举），后续批次持续追加。
- [docs/architecture.md](docs/architecture.md)：技术决策记录（模块名、迁移方案、密码哈希、i18n、品牌换肤、notify 设计等）。
