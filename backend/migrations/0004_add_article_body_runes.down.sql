-- 0004 回滚：删除正文字数列。这个数下次保存时会重新算出来，中间那段时间页面上
-- 少一个「N 字」，不影响读。
--
-- 约束先单独删掉，不指望 MySQL 在删列时替我们顺带处理。

ALTER TABLE articles
  DROP CHECK ck_articles_body_runes,
  DROP COLUMN body_runes;
