# Anxin · 个人网站

Vue 3 + Vite 前端，Gin + GORM + MySQL 后端的个人网站，主域名 `anxin-hitsz.com`，QA-Agent 入口 <https://qa.anxin-hitsz.com>。Redis 缓存分阶段启用，当前未接入。

## 启动

需要 Node.js 22.12+（推荐使用你现有的 Node.js 24）。在仓库根目录执行：

```sh
npm ci
npm run dev
```

打开 <http://localhost:5173>。默认读取真实 HTTP API，需要同时启动 Go 与 MySQL；只想看界面时改用一个 mock 配置即可。

### 只调界面

在根目录建 `.env.local` 写入 `VITE_DATA_SOURCE=mock`，重启 Vite。页面改用 `src/mocks/articles.js` 的示例数据，不需要 Go 和 MySQL。

### 联调真实接口

三个进程都要起，缺任何一个页面都会落到错误态：

1. **SSH 隧道**（PowerShell，保持窗口开启）

   ```powershell
   ssh -N -o ExitOnForwardFailure=yes -o ServerAliveInterval=30 -o ServerAliveCountMax=3 -L 127.0.0.1:13306:127.0.0.1:3306 你的SSH用户名@8.135.60.136
   ```

2. **Go 服务**——必须在 `backend` 目录执行：`config.Load()` 只读当前工作目录的 `.env`，从根目录启动会报 `缺少必填配置 MYSQL_HOST`。

   ```powershell
   Set-Location D:\Projects\personal-site\backend
   go run ./cmd/server
   ```

   先打印 `MySQL 连接检查通过`，再打印 `listening on http://127.0.0.1:8080`。

3. **Vite**——上一条 `npm run dev`。

**本地联调不需要 `.env.local`**：默认值 `VITE_DATA_SOURCE=http`、`VITE_API_BASE_URL=/api/v1`、`API_PROXY_TARGET=http://127.0.0.1:8080` 已经指向本机 Go 服务，`/api` 由 Vite 代理，因此没有跨域问题。数据库连接字段见 [backend/.env.example](backend/.env.example)，本机把 `MYSQL_PORT` 指向隧道端口 `13306`。隧道、`mysqlsh` 验证与账号约定见[数据库说明](backend/internal/database/README.md#windows-本机开发)。

```sh
npm test         # 前端数据适配器契约测试
npm run check   # JS 语法检查
npm run build   # Vue 编译与生产构建，输出 dist/
npm run preview # 查看构建结果：http://localhost:4173
```

## 当前交付

- Vue 单文件组件：首页、文章列表、文章条目。
- 关键词搜索、分类筛选、分页、加载中、空列表、错误重试、移动端布局与键盘焦点。
- mock / HTTP 共用数据服务，请求取消与超时，响应结构校验。
- Go 服务：`GET /api/v1/articles`，参数校验、分页与关键词/分类过滤。
- QA-Agent 外链、站点域名与基础 metadata。

接口契约以代码为准：[适配器与响应校验](frontend/src/api/articles.js)、[响应结构](backend/internal/dto/article.go)、[契约测试](tests/articles.test.js)。

mock 中的文章均为示例，不代表真实经历或已发布内容。当前只展示文章摘要，正文与管理后台留待后续阶段。

## 项目结构

```text
frontend/
  index.html                Vue 挂载页与 metadata
  public/favicon.svg        站点图标
  src/App.vue               页面布局
  src/components/           ArticleList / ArticleEntry
  src/api/articles.js       数据接口与适配器
  src/mocks/articles.js     示例数据
  src/config.js             域名、API、数据源配置
  src/styles.css            响应式样式
backend/
  cmd/server/               程序入口与路由装配
  internal/config/          环境变量加载与校验
  internal/database/        MySQL 连接池
  internal/model/           表结构映射
  internal/repository/      查询
  internal/service/         业务规则
  internal/handler/         HTTP 参数校验与响应
  internal/dto/             响应结构
  internal/middleware/      恢复中间件
  migrations/               手动执行的 SQL 迁移，启动不建表
tests/                     前端契约测试
scripts/check.mjs          JS 语法检查
deploy/nginx/              部署网关模板
vite.config.js             构建与本地 API 代理
```

## 前后端边界

Vite 开发服务器把 `/api` 代理到 `API_PROXY_TARGET`，前端只用相对路径，因此本地没有 CORS 问题。若改用完整的跨域 API URL，由 Go 服务配置准确的允许来源。

接口失败时页面显示错误并提供重试，不会回退到模拟数据——mock 只在 `VITE_DATA_SOURCE=mock` 时启用。

`npm run preview` 只预览构建产物，不代理 `/api`；真实接口联调要用 `npm run dev`。

## 生产部署约定

目标 ECS 为 `8.135.60.136`，直接使用已有 MySQL，Redis 缓存分阶段启用。根目录复制 `.env.production.example` 为 `.env.production` 后执行 `npm run build`；构建会拒绝 mock 模式。静态目录为 `dist/`，通过部署网关将 `/api/v1` 转发到 Go 服务；主域名开启 HTTPS。前端配置不能存放密钥。

当前只用到 ECS 上的开发库（经 SSH 隧道，见「启动」），尚未部署服务、未修改 DNS。

配置模板：根目录 .env.example、.env.production.example、[后端环境变量](backend/.env.example)、[Nginx 模板](deploy/nginx/anxin-hitsz.com.conf.example)。迁移与运行时使用分离的最小权限账号，迁移由人手动执行，见 [迁移约定](backend/migrations/README.md)。

生产库不执行种子与清理；`ARTICLE_CACHE_ENABLED` 保持 `false`。
