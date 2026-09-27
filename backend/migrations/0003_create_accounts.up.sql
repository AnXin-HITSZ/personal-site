-- 0003 创建账号体系的表：注册用户、登录会话、一次性令牌。

CREATE TABLE users (
  id                VARCHAR(32)  COLLATE utf8mb4_0900_bin    NOT NULL,
  email             VARCHAR(255) COLLATE utf8mb4_0900_as_ci  NOT NULL,
  password_hash     VARCHAR(255) NOT NULL,
  role              VARCHAR(16)  NOT NULL,
  status            VARCHAR(16)  NOT NULL,
  email_verified_at DATETIME     NULL,
  created_at        DATETIME     NOT NULL,
  updated_at        DATETIME     NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_users_email (email),
  CONSTRAINT ck_users_role
    CHECK (role IN ('admin', 'member')),
  CONSTRAINT ck_users_status
    CHECK (status IN ('active', 'disabled'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_ci;

CREATE TABLE sessions (
  id           VARCHAR(32)  COLLATE utf8mb4_0900_bin NOT NULL,
  user_id      VARCHAR(32)  COLLATE utf8mb4_0900_bin NOT NULL,
  token_hash   CHAR(64)     COLLATE utf8mb4_0900_bin NOT NULL,
  user_agent   VARCHAR(255) NOT NULL,
  ip           VARCHAR(45)  NOT NULL,
  expires_at   DATETIME     NOT NULL,
  created_at   DATETIME     NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_sessions_token_hash (token_hash),
  KEY idx_sessions_user (user_id),
  KEY idx_sessions_expires (expires_at),
  CONSTRAINT fk_sessions_user
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_ci;

CREATE TABLE auth_tokens (
  id         VARCHAR(32) COLLATE utf8mb4_0900_bin NOT NULL,
  user_id    VARCHAR(32) COLLATE utf8mb4_0900_bin NOT NULL,
  kind       VARCHAR(16) NOT NULL,
  token_hash CHAR(64)    COLLATE utf8mb4_0900_bin NOT NULL,
  expires_at DATETIME    NOT NULL,
  used_at    DATETIME    NULL,
  created_at DATETIME    NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_auth_tokens_token_hash (token_hash),
  KEY idx_auth_tokens_user_kind (user_id, kind),
  CONSTRAINT fk_auth_tokens_user
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
  CONSTRAINT ck_auth_tokens_kind
    CHECK (kind IN ('email_verify', 'password_reset'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_ci;
