package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"

	"anxin-hitsz.com/backend/internal/auth"
	"anxin-hitsz.com/backend/internal/mail"
	"anxin-hitsz.com/backend/internal/storage"
)

type Config struct {
	AppEnv   string
	HTTPAddr string
	MySQL    MySQLConfig
	Session  SessionConfig
	// 站点对外的地址，用来拼验证邮件和重置口令邮件里的链接。没有末尾斜杠。
	SiteBaseURL string
	// 邮件通道。nil 表示这台机器不发信，由 main.go 按环境挑一个占位实现顶上。
	Mail *mail.SMTPConfig
	// 图片存储。nil 表示这台机器不存图片，同样由 main.go 挑占位实现。
	OSS *storage.OSSConfig
}

type SessionConfig struct {
	CookieSecure bool
	// 额外的可信来源。同源请求不靠它判断（那是拿 Origin 的主机和本次请求的
	// Host 直接比），只有反向代理改写了 Host 时才需要在这里补一条。
	AllowedOrigins []string
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

	mailConfig, err := loadMailConfig()
	if err != nil {
		return Config{}, err
	}

	ossConfig, err := loadOSSConfig()
	if err != nil {
		return Config{}, err
	}

	appEnv := optionalString("APP_ENV", "development")

	return Config{
		AppEnv:   appEnv,
		HTTPAddr: optionalString("HTTP_ADDR", "127.0.0.1:8080"),
		MySQL:    mysqlConfig,
		Session: SessionConfig{
			// 生产只走 HTTPS。开发是 http://localhost:5173，带上 Secure 的
			// cookie 浏览器根本不会存，更不会发回来。
			CookieSecure:   appEnv == "production",
			AllowedOrigins: optionalList("ALLOWED_ORIGINS"),
		},
		// 去掉末尾斜杠，拼链接时才不用到处判断中间该有几个斜杠。
		SiteBaseURL: strings.TrimRight(optionalString("SITE_BASE_URL", ""), "/"),
		Mail:        mailConfig,
		OSS:         ossConfig,
	}, nil
}

// 「一个都没填」是合法的——那表示这台机器不存图片，由调用方挑占位实现顶上。
// 但只要填了 bucket，其余四项就都是必填：半套配置不能拖到作者传第一张图时
// 才暴露，那时他看到的会是一句和配置无关的 500。
func loadOSSConfig() (*storage.OSSConfig, error) {
	bucket := optionalString("OSS_BUCKET", "")
	if bucket == "" {
		return nil, nil
	}

	region, err := requiredString("OSS_REGION")
	if err != nil {
		return nil, err
	}
	accessKeyID, err := requiredString("OSS_ACCESS_KEY_ID")
	if err != nil {
		return nil, err
	}
	accessKeySecret, err := requiredString("OSS_ACCESS_KEY_SECRET")
	if err != nil {
		return nil, err
	}
	publicBaseURL, err := requiredString("OSS_PUBLIC_BASE_URL")
	if err != nil {
		return nil, err
	}

	return &storage.OSSConfig{
		Region:          region,
		Bucket:          bucket,
		Endpoint:        optionalString("OSS_ENDPOINT", ""),
		AccessKeyID:     accessKeyID,
		AccessKeySecret: accessKeySecret,
		// 去掉末尾斜杠，拼 URL 时才不用到处判断中间该有几个斜杠。
		PublicBaseURL: strings.TrimRight(publicBaseURL, "/"),
	}, nil
}

// 「一个都没填」是合法的——那表示这台机器不发信，由调用方挑占位实现顶上。
// 但只要填了主机，其余四项就都是必填：半套配置不能退化成安静地不发信，
// 那会让注册看起来成功，而用户永远收不到那封信。
func loadMailConfig() (*mail.SMTPConfig, error) {
	host := optionalString("SMTP_HOST", "")
	if host == "" {
		return nil, nil
	}

	port, err := optionalInt("SMTP_PORT", 465, 1, 65535)
	if err != nil {
		return nil, err
	}
	username, err := requiredString("SMTP_USERNAME")
	if err != nil {
		return nil, err
	}
	password, err := requiredString("SMTP_PASSWORD")
	if err != nil {
		return nil, err
	}
	from, err := requiredString("SMTP_FROM")
	if err != nil {
		return nil, err
	}

	// 复用注册那条校验，而不是在这里另写一遍：发件地址会原样进 SMTP 命令
	// 和邮件头，格式不对或者夹了换行都得在启动时就挡住，不能等到第一封
	// 注册信发不出去才发现。
	normalizedFrom, err := auth.NormalizeEmail(from)
	if err != nil {
		return nil, fmt.Errorf("SMTP_FROM %w", err)
	}

	return &mail.SMTPConfig{
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
		From:     normalizedFrom,
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

func optionalList(key string) []string {
	value := os.Getenv(key)
	if value == "" {
		return nil
	}
	list := make([]string, 0, strings.Count(value, ",")+1)
	for _, part := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			list = append(list, trimmed)
		}
	}
	return list
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
