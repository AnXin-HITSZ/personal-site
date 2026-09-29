# 数据库迁移

## 约定

- 文件名 `<四位版本>_<名称>.up.sql` / `.down.sql`，版本只增不复用。
- 已应用到任何环境的迁移文件不再修改，后续变更追加新版本。
- 迁移由人手动执行，应用启动不建表、不跑 AutoMigrate。
- 迁移账号与运行账号分离：迁移用有 DDL 权限的账号手动执行；应用账号只有 DML（`SELECT, INSERT, UPDATE, DELETE`），不持有 DDL。这条对应用连得上的**每个**库都适用，没有哪个库可以只给 `SELECT`，见下面「运行账号授权」。
- 库有两个：开发库 `personal_site_dev`，生产库 `personal_site_prod`。下面每条规则对这两个库一视同仁。
- 所有时间列存 UTC，不用 `TIMESTAMP`，不设 `DEFAULT CURRENT_TIMESTAMP`。
- 种子与清理只针对开发库，生产库不执行。

## 运行账号授权

应用账号 `personal_site_app` 的 DML 授权要覆盖到每一个它连得上的库：

```sql
GRANT SELECT, INSERT, UPDATE, DELETE ON `personal_site_dev`.*  TO `personal_site_app`@`localhost`;
GRANT SELECT, INSERT, UPDATE, DELETE ON `personal_site_prod`.* TO `personal_site_app`@`localhost`;
```

`GRANT` 立即生效，不需要 `FLUSH PRIVILEGES`。给完用 `SHOW GRANTS FOR CURRENT_USER();` 核对。

只给 `SELECT` 的库上，写入会报 `1142 ... command denied`。注意这个错会把下面的问题盖住：同一张表上 `SELECT` 报 1146、`INSERT` 报 1142，因为 MySQL 在 `INSERT` 路径上先查权限、后查表存不存在。所以授权补齐之后才会露出 `1146 ... doesn't exist`，两个都修完才能写入。

## 执行

```sh
mysql personal_site_dev < 0001_create_articles.up.sql
mysql personal_site_dev < 0001_create_articles.down.sql   # 回滚
```

## 部署记录

| 版本 | 文件 | 开发库 | 生产库 |
| --- | --- | --- | --- |
| 0001 | `0001_create_articles` | 已应用 | 已应用 2026-09-27 |
| 0002 | `0002_add_article_body` | 已应用 2026-09-27 | 已应用 2026-09-27 |
| 0003 | `0003_create_accounts` | 已应用 2026-09-27 | 已应用 2026-09-27 |
| 0004 | `0004_add_article_body_runes` | 待应用 | 待应用 |

`0002` 给文章表加了 `body`，`NOT NULL` 无默认值，存量行会填成空串。要在写入第一篇文章之前应用它：缺这一列不会报任何错，正文会静默为空——列表查询用 `Omit("Body")` 展开成不含它的显式列名，详情是 `Take(&model.Article)`，目标就是模型本身，GORM 发 `SELECT *`，扫不到就留零值。

`0003` 建 `users`、`sessions`、`auth_tokens` 三张表。账号相关的接口全部依赖它，没应用之前注册、登录、找回口令都回 500，前端上只表现为「服务暂时不可用，请稍后再试」。

`0004` 给文章表加了 `body_runes`（正文的 rune 数，与 `reading_minutes` 同源，一起算、一起落库）。**先应用它，再让带这一列的代码上线**：写入会带上这一列，缺列时 `INSERT`／`UPDATE` 直接报 `1054 Unknown column`；读的那一侧更早爆——公开列表和后台列表都是 `Omit("Body")` 展开成的显式列名，列名里已经有 `body_runes`，缺列时两个列表接口一起 500。详情走 `SELECT *`，缺列只是扫成零值，不报错。

存量行由 `DEFAULT 0` 填成 0，那几篇要重新保存一次才会有这个数；生产库当前没有文章，不需要回填。
