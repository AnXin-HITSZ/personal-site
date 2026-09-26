# Anxin · 个人网站

Vue 3 + Vite 前端，Gin + GORM + MySQL + Redis 后端的个人网站，主域名 `anxin-hitsz.com`，QA-Agent 入口 <https://qa.anxin-hitsz.com>。

## 启动

需要 Node.js 22.12+（推荐使用你现有的 Node.js 24）。在仓库根目录执行：

```sh
npm ci
npm run dev
```

打开 <http://localhost:5173>。默认使用真实 HTTP API，需要你启动 Go + MySQL。先复制根目录 `.env.example` 为 `.env.local`；ECS 联调见 [部署文档](docs/deployment.md)。仅调试界面时可设置 `VITE_DATA_SOURCE=mock` 并重启 Vite。

```sh
npm test         # 前端数据适配器契约测试
npm run check   # JS 语法检查
npm run build   # Vue 编译与生产构建，输出 dist/
npm run preview # 查看构建结果：http://localhost:4173
```

## 当前交付

- Vue 单文件组件：首页、文章列表、文章卡片。
- 关键词搜索、分类筛选、分页、加载中、空列表、错误重试、移动端布局与键盘焦点。
- mock / HTTP 共用数据服务，请求取消与超时，响应结构校验。
- QA-Agent 外链、站点域名与基础 metadata。
- [接口契约](docs/openapi.yaml)、[第一阶段文章列表练习](docs/exercise-01.md)、[后端分阶段路线](docs/backend-roadmap.md)。

文章均为示例，不代表真实经历或已发布内容。当前只展示文章摘要，正文与管理后台留待后续阶段。

## 项目结构

```text
frontend/
  index.html                Vue 挂载页与 metadata
  public/favicon.svg        站点图标
  src/App.vue               页面布局
  src/components/           ArticleList / ArticleCard
  src/api/articles.js       数据接口与适配器
  src/mocks/articles.js     示例数据
  src/config.js             域名、API、数据源配置
  src/styles.css            响应式样式
backend/README.md           后端练习边界，无实现
docs/                      OpenAPI 与练习文档
tests/                     前端契约测试
vite.config.js             构建与本地 API 代理
```

## 接入 Go

Windows 本机连接 ECS MySQL 的完整命令见 [数据库说明：SSH 隧道与 MySQL Shell](backend/internal/database/README.md#windows-本机开发)。

1. 你完成 `GET /api/v1/articles`，监听 `127.0.0.1:8080`。
2. 数据源默认 http；根目录 `.env.local` 使用 `VITE_API_BASE_URL=/api/v1` 和 `API_PROXY_TARGET=http://127.0.0.1:8080`。Go 在 ECS 时用 SSH 隧道连接 `8.135.60.136`。
3. Vite 开发服务器自动代理 `/api`，无需本地 CORS。若改用完整跨域 API URL，由 Go 服务配置准确的允许来源。
4. 接口失败显示错误，不会自动回退到模拟数据。

`npm run preview` 只预览构建产物，不提供 Go API 代理；真实 API 联调用 `npm run dev`。

## 生产部署约定

目标 ECS 为 `8.135.60.136`，直接使用已有 MySQL，Redis 缓存分阶段启用。根目录复制 `.env.production.example` 为 `.env.production` 后执行 `npm run build`；构建会拒绝 mock 模式。静态目录为 `dist/`，通过部署网关将 `/api/v1` 转发到 Go 服务；主域名开启 HTTPS。前端配置不能存放密钥。

当前仅初始化本地项目，未部署、未修改 DNS，没有代写 Go 后端。

配置模板：根目录 .env.example、.env.production.example、[后端环境变量](backend/.env.example)、[Nginx 模板](deploy/nginx/anxin-hitsz.com.conf.example)。[部署与 SSH 联调说明](docs/deployment.md) 包含域名解析、数据库隔离和 Redis 约定。未连接或修改 ECS。


后端框架与当前落地差距见 [架构审查](docs/architecture-review.md)。Gin/GORM 由你实现；现有 Vue 和 OpenAPI 无需因选用框架而更换。
