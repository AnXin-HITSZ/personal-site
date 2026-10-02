-- 0006 清掉 0005 播下的四个内置分类：分类从此全部由人在写作页自己建，不沿用那份名单。
--
-- 0005 里那四行 INSERT 不是种子，而是外键的前提——它们就是被换掉的那个 CHECK 里原来
-- 的四个词，父行不存在，ADD CONSTRAINT 当场报 1452。现在外键建好了，前提也就算用完
-- 了，可以把没有文章指着的行清掉。
--
-- NOT EXISTS 那一圈不是保险，是必须：外键是 RESTRICT，只要有一篇文章还归在某一行
-- 底下，直接 DELETE 会报 1451、整条语句原地回滚。有文章指着的那一行就留着
-- ——分类要删，该走写作页那条「先把文章挪走」的流程，由人来决定，不是迁移顺手替他决定。
DELETE FROM categories
 WHERE id IN ('backend', 'frontend', 'ai', 'notes')
   AND NOT EXISTS (SELECT 1 FROM articles WHERE articles.category = categories.id);
