package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv   string
	HTTPAddr string
	MySQL    MySQLConfig
}

type MySQLConfig struct {
	Host            string
	Port            int
	Database        string
	User            string
	Password        string
	Charset         string
	Location        *time.Location
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

func Load() (Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		// godotenv 的解析错误可能包含文件原文，不能透传到日志。
		return Config{}, errors.New("加载 .env 失败，请检查文件权限和格式")
	}

	mysqlConfig, err := loadMySQLConfig()
	if err != nil {
		return Config{}, err
	}

	return Config{
		AppEnv:   optionalString("APP_ENV", "development"),
		HTTPAddr: optionalString("HTTP_ADDR", "127.0.0.1:8080"),
		MySQL:    mysqlConfig,
	}, nil
}

func loadMySQLConfig() (MySQLConfig, error) {
	host, err := requiredString("MYSQL_HOST")
	if err != nil {
		return MySQLConfig{}, err
	}
	database, err := requiredString("MYSQL_DATABASE")
	if err != nil {
		return MySQLConfig{}, err
	}
	user, err := requiredString("MYSQL_USER")
	if err != nil {
		return MySQLConfig{}, err
	}
	password, err := requiredString("MYSQL_PASSWORD")
	if err != nil {
		return MySQLConfig{}, err
	}

	port, err := optionalInt("MYSQL_PORT", 3306, 1, 65535)
	if err != nil {
		return MySQLConfig{}, err
	}
	maxOpenConns, err := optionalInt("MYSQL_MAX_OPEN_CONNS", 10, 1, 1000)
	if err != nil {
		return MySQLConfig{}, err
	}
	maxIdleConns, err := optionalInt("MYSQL_MAX_IDLE_CONNS", 5, 0, 1000)
	if err != nil {
		return MySQLConfig{}, err
	}
	lifetimeSeconds, err := optionalInt("MYSQL_CONN_MAX_LIFETIME_SECONDS", 300, 1, 86400)
	if err != nil {
		return MySQLConfig{}, err
	}

	if maxIdleConns > maxOpenConns {
		return MySQLConfig{}, fmt.Errorf(
			"MYSQL_MAX_IDLE_CONNS(%d) 不能大于 MYSQL_MAX_OPEN_CONNS(%d)", maxIdleConns, maxOpenConns)
	}

	timezone := optionalString("MYSQL_TIMEZONE", "UTC")
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return MySQLConfig{}, fmt.Errorf("MYSQL_TIMEZONE 不是合法时区 %q: %w", timezone, err)
	}

	return MySQLConfig{
		Host:            host,
		Port:            port,
		Database:        database,
		User:            user,
		Password:        password,
		Charset:         optionalString("MYSQL_CHARSET", "utf8mb4"),
		Location:        location,
		MaxOpenConns:    maxOpenConns,
		MaxIdleConns:    maxIdleConns,
		ConnMaxLifetime: time.Duration(lifetimeSeconds) * time.Second,
	}, nil
}

func (c MySQLConfig) DSN() string {
	driverConfig := mysql.NewConfig()
	driverConfig.User = c.User
	driverConfig.Passwd = c.Password
	driverConfig.Net = "tcp"
	driverConfig.Addr = net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	driverConfig.DBName = c.Database
	driverConfig.ParseTime = true
	driverConfig.Timeout = 5 * time.Second
	driverConfig.ReadTimeout = 5 * time.Second
	driverConfig.WriteTimeout = 5 * time.Second
	driverConfig.Loc = c.Location
	driverConfig.Params = map[string]string{"charset": c.Charset}
	return driverConfig.FormatDSN()
}

func (c MySQLConfig) String() string {
	return fmt.Sprintf("MySQLConfig{Host:%s Port:%d Database:%s User:%s Password:*** Charset:%s Timezone:%s}",
		c.Host, c.Port, c.Database, c.User, c.Charset, c.Location)
}

func requiredString(key string) (string, error) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return "", fmt.Errorf("缺少必填配置 %s", key)
	}
	if value == "" {
		return "", fmt.Errorf("配置 %s 不能为空", key)
	}
	return value, nil
}

func optionalString(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

func optionalInt(key string, fallback, min, max int) (int, error) {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s 必须是整数，当前为 %q", key, value)
	}
	if parsed < min || parsed > max {
		return 0, fmt.Errorf("%s 必须在 %d 到 %d 之间，当前为 %d", key, min, max, parsed)
	}
	return parsed, nil
}
