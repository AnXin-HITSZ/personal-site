-- 0001 创建文章表。

CREATE TABLE articles (
  id              VARCHAR(32)  COLLATE utf8mb4_0900_bin NOT NULL,
  slug            VARCHAR(120) COLLATE utf8mb4_0900_bin NOT NULL,
  title           VARCHAR(255) NOT NULL,
  summary         VARCHAR(500) NOT NULL,
  category        VARCHAR(16)  NOT NULL,
  tags            JSON         NULL,
  status          VARCHAR(16)  NOT NULL,
  published_at    DATETIME     NULL,
  reading_minutes INT          NOT NULL,
  created_at      DATETIME     NOT NULL,
  updated_at      DATETIME     NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_articles_slug (slug),
  KEY idx_status_published (status, published_at DESC, id ASC),
  KEY idx_status_category_published (status, category, published_at DESC, id ASC),
  CONSTRAINT ck_articles_category
    CHECK (category IN ('backend', 'frontend', 'ai', 'notes')),
  CONSTRAINT ck_articles_status
    CHECK (status IN ('draft', 'published')),
  CONSTRAINT ck_articles_published_needs_time
    CHECK (status <> 'published' OR published_at IS NOT NULL),
  CONSTRAINT ck_articles_reading_minutes
    CHECK (reading_minutes >= 1)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_ci;
