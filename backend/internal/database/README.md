# MySQL 初始化

`OpenMySQL(ctx, cfg)` 接收 `config.Load()` 得到的 MySQL 配置，返回拥有 GORM 句柄 `DB` 的实例。调用方负责在退出时调用 `Close()`。

流程：创建 sql.DB → 设置连接池 → 带 5 秒期限的 PingContext → 将连接池交给 GORM。驱动连接/读取/写入也设置了 5 秒超时；不执行迁移，不读写文章。GORM 自动 Ping 和版本探测关闭，避免绕过显式的启动检查；适用于本项目规划的 MySQL 8+。GORM 内部日志暂时关闭，初始化错误使用不含凭据的提示。

初始化失败会关闭连接池。main.go 已接入，并在 HTTP 关闭流程结束后释放数据库连接。文章 Handler 尚未使用数据库，列表仍为空。

## Windows 本机开发

### 1. 开启 SSH 隧道

在 Windows PowerShell 中执行，将 `YOUR_SSH_USER` 替换为你登录 ECS 使用的 SSH 用户名（不是 MySQL 账号）：

```powershell
ssh -N -o ExitOnForwardFailure=yes -o ServerAliveInterval=30 -o ServerAliveCountMax=3 -L 127.0.0.1:13306:127.0.0.1:3306 YOUR_SSH_USER@8.135.60.136
```

该命令将 Windows 的 `127.0.0.1:13306` 转发到 ECS 的 `127.0.0.1:3306`。`-N` 表示只建立转发，不打开远程 Shell；连接后终端一直等待、没有新提示符是正常现象。保持此窗口开启，使用结束后按 `Ctrl+C` 关闭隧道。若使用私钥登录，在命令中加入 `-i "C:\路径\私钥文件"`。

无需将 ECS 的 MySQL 3306 端口开放到公网；SSH 登录端口须可访问。若 SSH 使用非默认端口，在命令中加入 `-p 实际SSH端口`。

### 2. 通过 MySQL Shell 连接开发数据库

另开一个 PowerShell 窗口执行：

```powershell
mysqlsh --sql --mysql -h 127.0.0.1 -P 13306 -u personal_site_app -p -D personal_site_dev
```

出现密码提示时输入 **MySQL 账号** `personal_site_app` 的密码。不要把密码直接写入命令或提交到仓库。这里使用已安装的 `mysqlsh`；`--sql --mysql` 指定 SQL 模式和 MySQL Classic 协议。

连接后可以执行：

```sql
SELECT DATABASE(), CURRENT_USER(), VERSION();
SHOW GRANTS;
SHOW TABLES;
```

`DATABASE()` 应为 `personal_site_dev`；`CURRENT_USER()` 显示实际匹配的 MySQL 授权账号，当前预期为 `personal_site_app@localhost`。输入 `\quit` 退出 MySQL Shell，不会关闭另一个窗口中的 SSH 隧道。

若提示无法连接 `127.0.0.1:13306`，先检查 SSH 窗口是否仍在运行，再在 PowerShell 中检查本地端口：

```powershell
Test-NetConnection 127.0.0.1 -Port 13306
```

`TcpTestSucceeded: True` 只表示本地 TCP 端口可连接，数据库认证及远端 MySQL 是否可用仍需通过 `mysqlsh` 验证。

### 3. 启动本地 Go 服务

保持 SSH 隧道开启。在 `backend` 目录运行服务，使 godotenv 能读取该目录的 `.env`。它不会覆盖已存在的进程环境变量。

```powershell
Set-Location D:\Projects\personal-site\backend
go run ./cmd/server
```

本地 `.env` 的连接字段应为：

```dotenv
APP_ENV=development
HTTP_ADDR=127.0.0.1:8080
MYSQL_HOST=127.0.0.1
MYSQL_PORT=13306
MYSQL_DATABASE=personal_site_dev
MYSQL_USER=personal_site_app
MYSQL_PASSWORD=替换为真实密码
MYSQL_CHARSET=utf8mb4
MYSQL_TIMEZONE=UTC
```

真实文件不要提交。密码如果包含特殊字符，按 godotenv 格式正确引用；不要输出 DSN。库中 DATETIME 的 UTC 约定由应用保证，DSN 的 loc 不修改 MySQL 会话时区。

成功日志为 `MySQL 连接检查通过`，之后才启动 HTTP。连接成功只证明连接和 Ping 正常，文章表 SELECT 权限将在查询阶段验证。自动化测试使用虚构数据及内存中的测试驱动，没有访问 ECS。
