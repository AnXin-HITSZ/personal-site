package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"anxin-hitsz.com/backend/internal/config"
	"anxin-hitsz.com/backend/internal/database"
	"anxin-hitsz.com/backend/internal/handler"
	"anxin-hitsz.com/backend/internal/mail"
	"anxin-hitsz.com/backend/internal/middleware"
	"anxin-hitsz.com/backend/internal/ratelimit"
	"anxin-hitsz.com/backend/internal/repository"
	"anxin-hitsz.com/backend/internal/service"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := database.OpenMySQL(ctx, cfg.MySQL)
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Print("关闭 MySQL 连接池失败")
		}
	}()

	log.Print("MySQL 连接检查通过")

	router, err := newRouter(cfg, db.DB)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		log.Printf("listening on http://%s", cfg.HTTPAddr)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		log.Println("shutdown signal received, draining…")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		if closeErr := srv.Close(); closeErr != nil {
			log.Printf("force close failed: %v", closeErr)
		}
		return err
	}

	log.Println("server stopped")
	return nil
}

// 开发环境用 LogMailer：它把验证链接打进日志，本地才捞得到令牌。
// 生产用占位实现，写信失败——第 5 步接上 SMTP 之前，生产环境的注册和
// 找回口令会明确失败。失败好过安静地把令牌写进生产日志。
func newMailer(appEnv string) mail.Mailer {
	if appEnv == "production" {
		return mail.UnconfiguredMailer{}
	}
	return mail.LogMailer{}
}

// 限速策略集中在这里，一眼能看完每条是几分钟几次。
// 具体数字的取舍：按 IP 的额度比按邮箱的宽，因为一个办公网出口后面可能坐着好几个人；
// 按邮箱的额度收紧，因为那才是爆破的靶子。
func newAccountLimits() handler.AccountLimits {
	return handler.AccountLimits{
		RegisterPerIP:    ratelimit.New(5, time.Hour),
		LoginPerIP:       ratelimit.New(20, 15*time.Minute),
		LoginPerEmail:    ratelimit.New(5, 15*time.Minute),
		PasswordPerIP:    ratelimit.New(10, time.Hour),
		PasswordPerEmail: ratelimit.New(3, time.Hour),
		ResendPerEmail:   ratelimit.New(3, time.Hour),
	}
}

func newRouter(cfg config.Config, db *gorm.DB) (*gin.Engine, error) {
	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	// 顺序不能倒：LogServerErrors 在外面，它的 defer 才看得到 Recovery 写下的 500。
	router.Use(middleware.LogServerErrors(), middleware.Recovery())

	if err := router.SetTrustedProxies([]string{"127.0.0.1"}); err != nil {
		return nil, err
	}

	articleService := service.NewArticle(repository.NewArticle(db))
	articleList := handler.NewArticlesList(articleService)
	articleGet := handler.NewArticleGet(articleService)

	accountRepo := repository.NewAccount(db)
	sessionService := service.NewSession(accountRepo)
	accountService := service.NewAccount(accountRepo, sessionService, newMailer(cfg.AppEnv), cfg.SiteBaseURL)

	cookies := middleware.NewSessionCookie(cfg.Session.CookieSecure)
	account := handler.NewAccount(accountService, cookies, newAccountLimits())

	api := router.Group("/api/v1")
	api.Use(middleware.NewOriginPolicy(cfg.Session.AllowedOrigins).SameOrigin())

	api.GET("/articles", articleList.List)
	api.GET("/articles/:slug", articleGet.Get)

	// 没有登录态的接口。它们的响应体都被刻意做成不区分邮箱是否存在。
	auth := api.Group("/auth")
	auth.POST("/register", account.Register)
	auth.POST("/verify-email", account.VerifyEmail)
	auth.POST("/resend-verification", account.ResendVerification)
	auth.POST("/login", account.Login)
	auth.POST("/logout", account.Logout)
	auth.POST("/forgot-password", account.ForgotPassword)
	auth.POST("/reset-password", account.ResetPassword)

	// 要登录的接口。RequireAuth 也在这里被用到真正的业务路由上。
	me := api.Group("/account", middleware.RequireAuth(cookies, sessionService))
	me.GET("/session", account.Session)
	me.POST("/password", account.ChangePassword)
	me.GET("/sessions", account.ListSessions)
	me.DELETE("/sessions/:id", account.RevokeSession)

	return router, nil
}
