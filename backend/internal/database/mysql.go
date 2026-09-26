package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"anxin-hitsz.com/backend/internal/config"
)

type MySQL struct {
	DB   *gorm.DB
	pool *sql.DB
}

func OpenMySQL(ctx context.Context, cfg config.MySQLConfig) (*MySQL, error) {
	pool, err := sql.Open("mysql", cfg.DSN())
	if err != nil {
		return nil, errors.New("MySQL 连接配置无效，请检查配置")
	}
	return initialize(ctx, cfg, pool)
}

func initialize(ctx context.Context, cfg config.MySQLConfig, pool *sql.DB) (*MySQL, error) {
	success := false
	defer func() {
		if !success {
			_ = pool.Close()
		}
	}()

	pool.SetMaxOpenConns(cfg.MaxOpenConns)
	pool.SetMaxIdleConns(cfg.MaxIdleConns)
	pool.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.PingContext(pingCtx); err != nil {
		if pingCtx.Err() != nil {
			return nil, pingCtx.Err()
		}
		return nil, errors.New("MySQL 连接检查失败，请检查 SSH 隧道、地址、账号密码和数据库权限")
	}

	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      pool,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		DisableAutomaticPing: true, // Already checked with a bounded context above.
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, errors.New("GORM 初始化失败，请检查数据库配置")
	}
	success = true
	return &MySQL{DB: db, pool: pool}, nil
}

func (m *MySQL) Close() error {
	return m.pool.Close()
}
