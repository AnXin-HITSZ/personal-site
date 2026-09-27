# Anxin · 个人网站

Vue 3 + Vite 前端，Gin + GORM + MySQL 后端的个人网站，主域名 `anxin-hitsz.com`，QA-Agent 入口 <https://qa.anxin-hitsz.com>。Redis 缓存分阶段启用，当前未接入。

## 启动

需要 Node.js 22.12+（推荐使用你现有的 Node.js 24）。

### 一条命令起全套

```sh
bash dev.sh
```

按 SSH 隧道 → Go 后端 → Vite 前端的顺序起，每一环都等到**真正就绪**才继续下一环——后端要等到它自己打印 `MySQL 连接检查通过`，因此「起来了」就等于数据库链路确实通了，而不是端口恰好被占上。已经在跑的环节直接复用、不会重复拉起，退出时也只关它自己起的进程。按 Ctrl+C 或关掉终端窗口都会收尾；被强杀留下的进程用 `bash dev.sh --stop` 收回。

开关：`--no-tunnel`（隧道已在跑）、`--no-frontend`（只调接口）、`--no-install`（依赖缺失时不自动安装）、`--help`。超时上限用 `READY_TIMEOUT`（秒）覆盖，隧道目标用 `SSH_ALIAS` / `TUNNEL_PORT` / `DB_PORT` 覆盖。

前置：`ssh`、`go`、`node` 在 PATH 上；`backend/.env` 已按 [backend/.env.example](backend/.env.example) 填好，且 `MYSQL_PORT` 与隧道端口一致（不一致时脚本会在连库之前就拦下）；本地有能免密登录 ECS 的私钥。

### 手动起

前端工程自成一个目录，在 `frontend/` 执行：

```sh
npm ci
npm run dev
```

打开 <http://localhost:5173>。默认读取真实 HTTP API，需要同时启动 Go 与 MySQL；只想看界面时改用一个 mock 配置即可。

其余前端命令同样在 `frontend/` 执行：

```sh
npm test         # 前端数据适配器契约测试
npm run check   # JS 语法检查
npm run build   # Vue 编译与生产构建，输出 frontend/dist/
npm run preview # 查看构建结果：http://localhost:4173
```

### 只调界面

在 `frontend/` 建 `.env.local` 写入 `VITE_DATA_SOURCE=mock`，重启 Vite。页面改用 `src/mocks/articles.js` 的示例数据，不需要 Go 和 MySQL。

### 联调真实接口

`bash dev.sh` 做的就是下面这三件事。要分开起、单独看每一环的输出时按这个顺序来——三个进程缺任何一个，页面都会落到错误态：

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

## 当前交付

- Vue 单文件组件：首页、文章列表、文章条目、文章详情、404 页。
- vue-router 承载真实路由：`/`、`/articles/:slug`，其余路径落到 404，刷新任意路径都由网关回退到 `index.html`。
- 关键词搜索、分类筛选、分页、加载中、空列表、错误重试、移动端布局与键盘焦点。
- 正文以 Markdown 原文存库，详情页在前端用 markdown-it 渲染。markdown-it 默认 `html: false`，正文里的 HTML 转义后原样显示。
- 路由切换回填 `<title>`、`meta description` 与 `link[rel="canonical"]`。
- mock / HTTP 共用数据服务，请求取消与超时，响应结构校验。
- Go 服务：`GET /api/v1/articles`、`GET /api/v1/articles/:slug`，参数校验、分页与关键词/分类过滤；列表不返回正文，详情对未发布文章与不存在的地址一律返回 404。
- QA-Agent 外链、站点域名与基础 metadata。

接口契约以代码为准：[适配器与响应校验](frontend/src/api/articles.js)、[响应结构](backend/internal/dto/article.go)、[契约测试](frontend/tests/articles.test.js)。

mock 中的文章均为示例，不代表真实经历或已发布内容。管理后台留待后续阶段。

## 项目结构

```text
dev.sh                     本地开发一键启动（隧道 + 后端 + 前端）
.gitattributes             钉死 *.sh 为 LF，否则 shebang 会被 \r 破坏
frontend/                  前端工程，命令都在这一层执行
  index.html                Vue 挂载页与 metadata
  package.json              依赖与 npm 脚本
  vite.config.js            构建与本地 API 代理
  .env.example              环境变量模板，复制为同目录的 .env.local
  public/favicon.svg        站点图标
  src/App.vue               页面布局
  src/router.js             路由表
  src/metadata.js           标题、描述与 canonical 的回填
  src/format.js             日期格式化
  src/components/           ArticleList / ArticleEntry
  src/views/                ArticleListView / ArticleDetailView / NotFoundView
  src/api/articles.js       数据接口与适配器
  src/mocks/articles.js     示例数据
  src/config.js             域名、API、数据源配置
  src/styles.css            响应式样式
  tests/                    契约测试
  scripts/check.mjs         JS 语法检查
  dist/                     构建产物（生成，不入库）
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
deploy/nginx/              部署网关模板
deploy/systemd/            后端服务单元
deploy/publish.sh          服务器上一键发布
deploy/start.sh            服务器上启动与状态
```

## 前后端边界

Vite 开发服务器把 `/api` 代理到 `API_PROXY_TARGET`，前端只用相对路径，因此本地没有 CORS 问题。若改用完整的跨域 API URL，由 Go 服务配置准确的允许来源。

接口失败时页面显示错误并提供重试，不会回退到模拟数据——mock 只在 `VITE_DATA_SOURCE=mock` 时启用。

`npm run preview` 只预览构建产物，不代理 `/api`；真实接口联调要用 `npm run dev`。

## 生产部署约定

目标 ECS 为 `8.135.60.136`，直接使用已有 MySQL，Redis 缓存分阶段启用。在 `frontend/` 复制 `.env.production.example` 为同目录的 `.env.production` 后执行 `npm run build`；构建会拒绝 mock 模式。产物落在 `frontend/dist/`，部署时把该目录放到服务器的 `/var/www/anxin-site/dist`，通过部署网关将 `/api/v1` 转发到 Go 服务；主域名开启 HTTPS。前端配置不能存放密钥。

### 服务器现状

主域名与 QA-Agent 共用一台 ECS 和一份 Nginx，改动时不要碰 `sites-available/qa-agent`。

| 项 | 位置 |
| --- | --- |
| 代码 | `/root/personal-site`，`git pull --ff-only` 更新 |
| 后端 | `/root/personal-site/backend/dist/server`，systemd 单元 `personal-site.service`，监听 `127.0.0.1:8080` |
| 后端配置 | `/root/personal-site/backend/.env`，`chmod 600`，不入库 |
| 前端产物 | `/var/www/anxin-site/dist` |
| 数据库 | 生产库 `personal_site_prod`，账号 `personal_site_app@localhost` |
| 网关 | `/etc/nginx/sites-available/anxin-hitsz.com` |
| 证书 | `/etc/nginx/ssl/anxin-hitsz.com/anxin-hitsz.com.{pem,key}` |

ECS 连不上 `proxy.golang.org`，服务器上已执行 `go env -w GOPROXY=https://goproxy.cn,direct`，否则 `go build` 会长时间卡在模块下载。

在服务器上发布，一条命令：

```sh
bash /root/personal-site/deploy/publish.sh
```

依次做：前置检查（root、工具链、`backend/.env`、工作区干净）→ `git pull --ff-only` → 后端编译到 `dist/server.new` 再原子替换 → 前端 `npm ci && npm run build` → `rsync -a --delete` 同步到 `/var/www/anxin-site/dist` → 重启并等接口应答。任何一步失败都当场停下并打印原因，`dist/server` 与线上产物不会停在半成品状态。首次使用前要先 `git pull` 让脚本本身到位——脚本不会自己拉自己。

日常启动与状态：

```sh
bash /root/personal-site/deploy/start.sh          # 启动后端与网关，等到接口真的应答
bash /root/personal-site/deploy/start.sh restart  # 换过二进制后重启后端
bash /root/personal-site/deploy/start.sh status   # 只报告状态，不改动任何东西
```

进程不归脚本管：开机自启与崩溃拉起由 systemd 负责，脚本只把「起服务 → 等就绪 → 报告」串成一条命令，退出后进程继续跑。nginx 同时服务 QA-Agent，脚本只会在它没跑的时候拉起来，任何情况下都不停它。

迁移仍由人手动执行，两个脚本都不碰数据库。发布带新列或改列的版本时，先按[迁移约定](backend/migrations/README.md)在库上执行对应迁移，再发布。列表查询用 `Omit("Body")`，GORM 会据此展开成不含正文的显式列名；详情是 `Take(&model.Article)`，目标就是模型本身，GORM 发的是 `SELECT *`——因此迁移滞后**不会报错**，缺的那列会被扫成零值，正文静默为空。

配置模板：[前端环境变量](frontend/.env.example)、[前端生产环境变量](frontend/.env.production.example)、[后端环境变量](backend/.env.example)、[Nginx 模板](deploy/nginx/anxin-hitsz.com.conf.example)、[systemd 单元](deploy/systemd/personal-site.service)。迁移与运行时使用分离的最小权限账号，迁移由人手动执行，见 [迁移约定](backend/migrations/README.md)。

生产库不执行种子与清理；`ARTICLE_CACHE_ENABLED` 保持 `false`。
