-- 0005 分类从写死的四个词变成一张表。
--
-- 四个内置分类的 id 沿用文章里原来那几个代号（backend、frontend、ai、notes），
-- 所以 articles.category 里的值一个字节都不用改，外键当场建得起来。新分类的 id 由
-- auth.NewShortID 生成，和文章 id 同一个来路——只是不进任何地址，看不见。
--
-- name 用表默认的 utf8mb4_0900_as_ci：「AI 探索」和「ai 探索」由库判成同一个名字，
-- 不靠代码记得去比。
--
-- id 是 VARCHAR(16)，和 articles.category 等宽，这是外键要求的。四条内置 id 最长 8 个
-- 字符、新生成的固定 8 个字符，都装得下。

CREATE TABLE categories (
  id         VARCHAR(16) NOT NULL,
  name       VARCHAR(16) NOT NULL,
  sort_order INT         NOT NULL,
  created_at DATETIME    NOT NULL,
  updated_at DATETIME    NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_categories_name (name),
  KEY idx_categories_sort (sort_order)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_ci;

INSERT INTO categories (id, name, sort_order, created_at, updated_at) VALUES
  ('backend',  '后端开发', 0, UTC_TIMESTAMP(), UTC_TIMESTAMP()),
  ('frontend', '前端实践', 1, UTC_TIMESTAMP(), UTC_TIMESTAMP()),
  ('ai',       'AI 探索',  2, UTC_TIMESTAMP(), UTC_TIMESTAMP()),
  ('notes',    '学习随笔', 3, UTC_TIMESTAMP(), UTC_TIMESTAMP());

-- 分类由 CHECK 约束换成外键。RESTRICT 是有意的：有文章的分类删不掉，由库拦下来，
-- 不是代码里判一下——判一下总有判漏的那一次。
--
-- idx_articles_category 是外键要求的（外键要求引用列上有一条以它为最左前缀的索引），
-- 现有的 idx_status_category_published 以 status 打头，顶不了这个位置。顺带它也
-- 正好是后台按分类筛选要用的那条。
ALTER TABLE articles
  DROP CHECK ck_articles_category,
  ADD KEY idx_articles_category (category),
  ADD CONSTRAINT fk_articles_category
    FOREIGN KEY (category) REFERENCES categories (id)
    ON DELETE RESTRICT ON UPDATE RESTRICT;
