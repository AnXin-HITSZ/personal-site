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

// MySQL 把 GORM 的句柄和底下的连接池一起拿着：DB 交给仓储用，pool 留在这里只为能关掉它
// ——GORM 手里是同一个池，但关闭只从这一处走。
type MySQL struct {
	DB   *gorm.DB
	pool *sql.DB
}

// sql.Open 不建连接，只是把 DSN 记下来——所以紧接着必须真的 ping 一次，配置写错了才知道。
func OpenMySQL(ctx context.Context, cfg config.MySQLConfig) (*MySQL, error) {
	pool, err := sql.Open("mysql", cfg.DSN())
	if err != nil {
		return nil, errors.New("MySQL 连接配置无效，请检查配置")
	}
	return initialize(ctx, cfg, pool)
}

func initialize(ctx context.Context, cfg config.MySQLConfig, pool *sql.DB) (*MySQL, error) {
	// 只有没走到最后才关这个池：成功了它就是调用方的东西，这里再关一次等于把人家手上
	// 的连接掐断。
	success := false
	defer func() {
		if !success {
			_ = pool.Close()
		}
	}()

	pool.SetMaxOpenConns(cfg.MaxOpenConns)
	pool.SetMaxIdleConns(cfg.MaxIdleConns)
	pool.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	// 启动时连不上要快点说，不能让进程挂在这儿等。两种情况分开：超时多半是隧道没起来，
	// 连上了被拒才是账号、密码或权限的事——回原样的错误，运维一眼看得出是哪一种。
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.PingContext(pingCtx); err != nil {
		if pingCtx.Err() != nil {
			return nil, pingCtx.Err()
		}
		return nil, errors.New("MySQL 连接检查失败，请检查 SSH 隧道、地址、账号密码和数据库权限")
	}

	// 两个开关各有各的来由。SkipInitializeWithVersion 省掉 GORM 启动时那次 SELECT VERSION()：
	// 它只为按版本挑写法，而这里连的固定是 MySQL 8（迁移用的 utf8mb4_0900_ai_ci 就是 8.0 的
	// 排序规则），没有要分支的地方。日志只留错误，不打每条 SQL——出错时带上去的上下文比
	// SQL 原文有用。
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
