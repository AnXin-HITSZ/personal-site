-- 回滚之前先看清楚：只要有一篇文章归在这四个之外，下面的 CHECK 就加不回去，
-- 整条语句会失败（不是静默放过）。那种情况下要先把这些文章改回内置的四个之一，
-- 或者接受「回不去」——分类表一旦开始用，它就已经是数据的一部分了。

ALTER TABLE articles
  DROP FOREIGN KEY fk_articles_category,
  DROP KEY idx_articles_category,
  ADD CONSTRAINT ck_articles_category
    CHECK (category IN ('backend', 'frontend', 'ai', 'notes'));

DROP TABLE categories;
