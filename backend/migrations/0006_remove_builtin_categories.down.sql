-- 0006 的回退：照 0005 那四行把内置分类放回去。
--
-- 这期间如果已经自建了同名的分类，名字上有唯一索引，这条会报 1062 并整条回滚——
-- 那就把撞上的那几行从语句里去掉再跑：内建的那一行本来也没必要回来了。
INSERT INTO categories (id, name, sort_order, created_at, updated_at) VALUES
  ('backend',  '后端开发', 0, UTC_TIMESTAMP(), UTC_TIMESTAMP()),
  ('frontend', '前端实践', 1, UTC_TIMESTAMP(), UTC_TIMESTAMP()),
  ('ai',       'AI 探索',  2, UTC_TIMESTAMP(), UTC_TIMESTAMP()),
  ('notes',    '学习随笔', 3, UTC_TIMESTAMP(), UTC_TIMESTAMP());
