# 数据库迁移

## 约定

- 文件名 `<四位版本>_<名称>.up.sql` / `.down.sql`，版本只增不复用。
- 已应用到任何环境的迁移文件不再修改，后续变更追加新版本。
- 迁移由人手动执行，应用启动不建表、不跑 AutoMigrate。
- 迁移账号与运行账号分离：迁移用有 DDL 权限的账号，应用账号只有 `SELECT`。
- 所有时间列存 UTC，不用 `TIMESTAMP`，不设 `DEFAULT CURRENT_TIMESTAMP`。
- 种子与清理只针对开发库和测试库，生产库不执行。

## 执行

```sh
mysql personal_site_dev < 0001_create_articles.up.sql
mysql personal_site_dev < 0001_create_articles.down.sql   # 回滚
```

## 部署记录

| 版本 | 文件 | 开发库 | 测试库 | 生产库 |
| --- | --- | --- | --- | --- |
| 0001 | `0001_create_articles` | 待应用 | 待应用 | 待应用 |
