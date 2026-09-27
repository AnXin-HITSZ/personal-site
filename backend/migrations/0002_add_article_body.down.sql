-- 0002 回滚：删除正文列。列内正文一并丢失，仅用于未上线环境。

ALTER TABLE articles DROP COLUMN body;
