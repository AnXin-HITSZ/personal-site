# Anxin · 个人网站

Vue 3 + Vite 前端，Gin + GORM + MySQL 后端的个人网站，主域名 `anxin-hitsz.com`，QA-Agent 入口 <https://qa.anxin-hitsz.com>。Redis 缓存分阶段启用，当前未接入。

## 启动

需要 Node.js 22.12+（推荐使用你现有的 Node.js 24）。

### 一条命令起全套

```sh
bash dev.sh
```

按 SSH 隧道 → Go 后端 → Vite 前端的顺序起，每一环都等到**真正就绪**才继续下一环——后端要等到它自己打印 `MySQL 连接检查通过`，因此「起来了」就等于数据库链路确实通了，而不是端口恰好被占上。已经在跑的环节直接复用、不会重复拉起，退出时也只关它自己起的进程。按 Ctrl+C 或关掉终端窗口都会收尾；被强杀留下的进程用 `bash dev.sh --stop` 收回。

开关：`--only-db`（只起隧道并保持前台，要用 `mysqlsh` 连库、跑迁移或 `-create-admin` 时用）、`--no-tunnel`（隧道已在跑）、`--no-frontend`（只调接口）、`--only-frontend`（只起前端，不碰数据库）、`--no-install`（依赖缺失时不自动安装）、`--help`。超时上限用 `READY_TIMEOUT`（秒）覆盖，隧道目标用 `SSH_ALIAS` / `TUNNEL_PORT` / `DB_PORT` 覆盖。

前置：本次要用到的命令在 PATH 上（只检查用得上的那些，`--only-db` 就只要 `ssh`，`--only-frontend` 就只要 `node`）；要用到数据库的那几种模式还要求 `backend/.env` 已按 [backend/.env.example](backend/.env.example) 填好，且 `MYSQL_PORT` 与隧道端口一致（不一致时脚本会在连库之前就拦下），以及本地有能免密登录 ECS 的私钥。

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

```sh
bash dev.sh --only-frontend
```

再在 `frontend/` 建 `.env.local` 写入 `VITE_DATA_SOURCE=mock`，重启 Vite。页面改用 `src/mocks/articles.js` 的示例数据，不需要 Go 和 MySQL。mock 只覆盖公开的读页面——账号与写作没有 mock 数据源，在 mock 下会明确报错。不配 mock 而直接 `--only-frontend` 也可以，只是要读真实数据的页面会显示各自的失败态，脚本在结束时会把这句话再说一遍。

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

## 邮件通道

注册验证、重发验证和找回口令都要发信，走阿里云邮件推送的 SMTP。代码在 `backend/internal/mail/smtp.go`，配置项见[后端环境变量](backend/.env.example)里的 `SMTP_*`。

**发信是这个服务唯一的对外网络调用。** 它有 10 秒的硬期限，覆盖 TCP 连接、TLS 握手和整段 SMTP 对话；账号接口是同步发信的，所以这也是注册那一屏最慢的响应时间。期限必须小于 Nginx 的 `proxy_read_timeout`（模板里是 15 秒），否则网关会先掐断，而服务还在等 SMTP。

发信失败**不会**让接口报错。这几个接口的响应体刻意做成不区分邮箱是否存在，一旦把「信没发出去」变成 500，就等于说出「这个邮箱确实存在，只是我们邮件出问题了」。失败只记一行日志，带收件人和主题，由人来看。

### 三种情况

| 情况 | 行为 |
| --- | --- |
| 配了 `SMTP_HOST` | 用真实 SMTP。开发机上也可以配，不必等到生产才第一次试 |
| 没配，`APP_ENV=development` | `LogMailer`，把验证链接整段打进日志——本地就是靠它捞令牌的 |
| 没配，`APP_ENV=production` | 明确失败。注册照常返回 202，但信不会发出，启动时有一行警告 |

生产没配 SMTP 时接口仍然返回「验证邮件已经发出」——这是上面那条不区分原则的代价。启动日志里的那行警告是唯一能提前发现它的地方。

### 端口与加密

`SMTP_PORT=465` 走隐式 TLS，连上就是密的。填其它端口则先明文连接、再要求 `STARTTLS` 升级，**服务器不支持就拒绝发送，不会降级成明文**——降级的下一步就是把口令递出去。凭据在任何情况下都不会走明文连接。

### 一次性准备

这些是人工的活，代码不碰：

1. 在邮件推送控制台添加发信域名，按提示在 DNS 加几条记录。控制台会给确切值，逐条照抄即可，不要自己想当然：
   - **域名所有权验证**：一条 TXT 记录，证明域名是你的。
   - **SPF**：一条 TXT 记录，列出允许代表这个域名发信的服务器。**根域上只能有一条 SPF 记录**——已经有的话要合并成一条，两条并存的后果是 SPF 整体失效，而不是取其一。
   - **DKIM**：控制台给的公钥记录，收件方拿它验证签名。这条最容易被跳过，而跳过之后邮件会大量进垃圾箱。
   - **DMARC**（可选但建议）：`_dmarc` 下的 TXT，先用 `v=DMARC1; p=none` 只做监测，确认没问题再收紧。
2. 在同一个控制台建发信地址，它必须落在已验证的域名下。这个地址填进 `SMTP_FROM`，**只填裸地址**，不能带显示名——带了会在启动时报错。
3. 把 `SMTP_HOST` / `SMTP_USERNAME` / `SMTP_PASSWORD` / `SMTP_FROM` 写进 `/root/personal-site/backend/.env`。口令是邮件推送控制台里单独设的，和阿里云账号密码不是一回事。

DNS 生效要时间，配完先发一封试试再往下走。数据库账号不需要任何新权限。

### 换了端口或服务器之后

`SMTP_HOST` 一旦填了，其余四项就是必填；半套配置会在启动时直接报错，不会退化成「安静地不发信」。发件地址会过一遍和注册邮箱相同的校验，格式不对或者夹了换行都在启动时挡住——发件地址会原样进 SMTP 命令和邮件头。
