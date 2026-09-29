-- 0004 文章表增加正文字数列，与 reading_minutes 同源：都由服务端按同一份正文数出来，
-- 一起落库。分开算迟早会有一个忘了跟着改。

ALTER TABLE articles
  ADD COLUMN body_runes INT NOT NULL DEFAULT 0 AFTER reading_minutes,
  ADD CONSTRAINT ck_articles_body_runes CHECK (body_runes >= 0);
