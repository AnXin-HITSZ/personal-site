-- 0003 回滚：删除账号体系的表。表内数据一并丢失，仅用于未上线环境。
-- 顺序与外键相反，先删子表再删 users。

DROP TABLE IF EXISTS auth_tokens;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
