-- 0002 文章表增加正文列，存 Markdown 原文。

ALTER TABLE articles
  ADD COLUMN body MEDIUMTEXT NOT NULL AFTER summary;
