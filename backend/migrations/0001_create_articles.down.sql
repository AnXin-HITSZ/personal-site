-- 0001 回滚：删除文章表。表内数据一并丢失，仅用于未上线环境。

DROP TABLE IF EXISTS articles;
